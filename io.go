package main

import "io"

const (
	// SectorSize is the size of one disk sector in bytes.
	SectorSize = 512

	// IRQ numbers of the devices connected to the I/O APIC.
	irqTimer = 0
	irqKbd   = 1
	irqCom1  = 4
	irqIDE   = 14

	// timerPeriod is the number of instructions between two timer
	// interrupts. It replaces the (wall clock) local APIC timer: with the
	// emulator running at a few tens of millions of instructions per second
	// this is roughly the 100Hz tick a real xv6 sees.
	timerPeriod = 200000
)

// Disk is a block device backing an emulated IDE drive.
type Disk interface {
	ReadSector(lba uint32, buf []byte) error
	WriteSector(lba uint32, buf []byte) error
}

// ReaderWriterAtDisk adapts a file (or any ReaderAt/WriterAt) to Disk.
type ReaderWriterAtDisk struct {
	r io.ReaderAt
	w io.WriterAt
}

// NewDisk creates a Disk from something which supports random access.
func NewDisk(r io.ReaderAt) *ReaderWriterAtDisk {
	d := &ReaderWriterAtDisk{r: r}
	if w, ok := r.(io.WriterAt); ok {
		d.w = w
	}
	return d
}

// ReadSector reads one sector from the disk.
func (d *ReaderWriterAtDisk) ReadSector(lba uint32, buf []byte) error {
	_, err := d.r.ReadAt(buf, int64(lba)*SectorSize)
	return err
}

// WriteSector writes one sector to the disk.
func (d *ReaderWriterAtDisk) WriteSector(lba uint32, buf []byte) error {
	if d.w == nil {
		return nil
	}
	_, err := d.w.WriteAt(buf, int64(lba)*SectorSize)
	return err
}

// MemDisk is a disk which lives in memory. It is used by the wasm build.
type MemDisk struct {
	data []byte
}

// NewMemDisk creates a disk backed by the given image.
func NewMemDisk(data []byte) *MemDisk { return &MemDisk{data: data} }

// ReadSector reads one sector from the image.
func (d *MemDisk) ReadSector(lba uint32, buf []byte) error {
	off := int(lba) * SectorSize
	for i := range buf {
		if off+i < len(d.data) {
			buf[i] = d.data[off+i]
		} else {
			buf[i] = 0
		}
	}
	return nil
}

// WriteSector writes one sector to the image, growing it if necessary.
func (d *MemDisk) WriteSector(lba uint32, buf []byte) error {
	off := int(lba) * SectorSize
	if off+len(buf) > len(d.data) {
		grown := make([]byte, off+len(buf))
		copy(grown, d.data)
		d.data = grown
	}
	copy(d.data[off:], buf)
	return nil
}

// IO emulates the I/O address space and the devices attached to it.
type IO struct {
	e     *Emulator
	hdds  [2]Disk
	ide   ideState
	uart  uartState
	lapic lapicState
	ioapi ioapicState
	crt   crtState

	// putc receives every byte the guest writes to the serial port.
	putc func(byte)
}

// NewIO creates the device model of the machine.
func NewIO(e *Emulator, out func(byte)) *IO {
	io := &IO{e: e, putc: out}
	io.ide.status = ideDRDY
	io.lapic.regs[lapicVER] = 0x00050014
	io.ioapi.regs[0] = 0
	for i := range io.ioapi.redir {
		io.ioapi.redir[i] = 0x00010000 // masked
	}
	return io
}

// PushInput queues one byte of console input for the guest.
func (io *IO) PushInput(b byte) {
	io.uart.rx = append(io.uart.rx, b)
	if io.uart.ier&0x01 != 0 {
		io.raiseIRQ(irqCom1)
	}
}

// HasInput reports whether console input is still waiting to be read.
func (io *IO) HasInput() bool { return len(io.uart.rx) > 0 }

// raiseIRQ routes a device interrupt through the I/O APIC.
func (io *IO) raiseIRQ(irq int) {
	entry := io.ioapi.redir[irq]
	if entry&0x00010000 != 0 {
		return // masked
	}
	io.e.raiseIRQ(int(entry & 0xFF))
}

// step advances the timers and the devices. It is called regularly from the
// instruction loop.
func (io *IO) step(instCount uint64) {
	if io.lapic.timerEnabled() && instCount-io.lapic.lastTick >= timerPeriod {
		io.lapic.lastTick = instCount
		io.e.raiseIRQ(int(io.lapic.regs[lapicTIMER] & 0xFF))
	}
	// the uart interrupt is level triggered: as long as unread data sits in
	// the receive buffer the line stays asserted
	if len(io.uart.rx) > 0 && io.uart.ier&0x01 != 0 {
		io.raiseIRQ(irqCom1)
	}
	if io.ide.irqCountdown > 0 {
		io.ide.irqCountdown--
		if io.ide.irqCountdown == 0 {
			io.raiseIRQ(irqIDE)
		}
	}
}

// idle fast forwards to the next timer deadline. It is called while the cpu
// is halted waiting for an interrupt.
func (io *IO) idle() {
	if io.lapic.timerEnabled() {
		next := io.lapic.lastTick + timerPeriod
		if next > io.e.instCount {
			io.e.instCount = next
		}
	} else {
		io.e.instCount += 256
	}
	io.step(io.e.instCount)
}

// ---------------------------------------------------------------------------
// port I/O
// ---------------------------------------------------------------------------

func (io *IO) in(port uint16, size int) uint32 {
	switch size {
	case 8:
		return uint32(io.in8(port))
	case 16:
		return uint32(io.in8(port)) | uint32(io.in8(port))<<8
	}
	if port == 0x01F0 {
		// the data register returns a whole 32bit chunk of the sector
		return uint32(io.in8(port)) | uint32(io.in8(port))<<8 |
			uint32(io.in8(port))<<16 | uint32(io.in8(port))<<24
	}
	return uint32(io.in8(port))
}

func (io *IO) out(port uint16, size int, value uint32) {
	switch size {
	case 8:
		io.out8(port, uint8(value))
	case 16:
		io.out8(port, uint8(value))
		io.out8(port+1, uint8(value>>8))
	default:
		if port == 0x01F0 {
			io.out8(port, uint8(value))
			io.out8(port, uint8(value>>8))
			io.out8(port, uint8(value>>16))
			io.out8(port, uint8(value>>24))
			return
		}
		io.out8(port, uint8(value))
	}
}

func (io *IO) in8(port uint16) uint8 {
	switch {
	case port >= 0x01F0 && port <= 0x01F7, port == 0x03F6:
		return io.ideIn(port)
	case port >= 0x03F8 && port <= 0x03FF:
		return io.uartIn(port)
	case port == 0x0060: // keyboard data
		return 0
	case port == 0x0064: // keyboard status: input buffer empty
		return 0x14
	case port == 0x03D4 || port == 0x03D5:
		return io.crtIn(port)
	case port == 0x0071: // CMOS data
		return 0
	case port == 0x0020 || port == 0x00A0: // 8259 PIC
		return 0
	}
	return 0xFF
}

func (io *IO) out8(port uint16, value uint8) {
	switch {
	case port >= 0x01F0 && port <= 0x01F7, port == 0x03F6:
		io.ideOut(port, value)
	case port >= 0x03F8 && port <= 0x03FF:
		io.uartOut(port, value)
	case port == 0x03D4 || port == 0x03D5:
		io.crtOut(port, value)
	}
	// everything else (PIC, PIT, CMOS, keyboard controller, ...) is ignored
}

// ---------------------------------------------------------------------------
// IDE disk
// ---------------------------------------------------------------------------

const (
	ideERR  = 0x01
	ideDRQ  = 0x08
	ideDRDY = 0x40
	ideBSY  = 0x80
)

type ideState struct {
	drive         int
	lba           uint32
	sectorCount   uint8
	status        uint8
	buf           [SectorSize]byte
	pos           int
	writing       bool
	sectorsLeft   uint8
	irqCountdown  int
	lastCmdIsRead bool
}

func (io *IO) ideIn(port uint16) uint8 {
	s := &io.ide
	switch port {
	case 0x01F0:
		if s.status&ideDRQ == 0 {
			return 0
		}
		b := s.buf[s.pos]
		s.pos++
		if s.pos == SectorSize {
			s.pos = 0
			s.sectorsLeft--
			if s.sectorsLeft > 0 {
				s.lba++
				io.ideReadSector()
			} else {
				s.status = ideDRDY
			}
		}
		return b
	case 0x01F1:
		return 0
	case 0x01F7, 0x03F6:
		if io.hdds[s.drive] == nil {
			return 0
		}
		return s.status
	}
	return 0
}

func (io *IO) ideOut(port uint16, value uint8) {
	s := &io.ide
	switch port {
	case 0x01F0:
		if !s.writing {
			return
		}
		s.buf[s.pos] = value
		s.pos++
		if s.pos == SectorSize {
			s.pos = 0
			if d := io.hdds[s.drive]; d != nil {
				d.WriteSector(s.lba, s.buf[:])
			}
			s.sectorsLeft--
			if s.sectorsLeft > 0 {
				s.lba++
			} else {
				s.writing = false
				s.status = ideDRDY
			}
			s.irqCountdown = 1
		}
	case 0x01F2:
		s.sectorCount = value
	case 0x01F3:
		s.lba = (s.lba &^ 0x000000FF) | uint32(value)
	case 0x01F4:
		s.lba = (s.lba &^ 0x0000FF00) | uint32(value)<<8
	case 0x01F5:
		s.lba = (s.lba &^ 0x00FF0000) | uint32(value)<<16
	case 0x01F6:
		s.lba = (s.lba &^ 0x0F000000) | (uint32(value)&0x0F)<<24
		s.drive = int((value >> 4) & 0x1)
	case 0x01F7:
		io.ideCommand(value)
	}
}

func (io *IO) ideCommand(cmd uint8) {
	s := &io.ide
	s.sectorsLeft = s.sectorCount
	if s.sectorsLeft == 0 {
		s.sectorsLeft = 1
	}
	s.pos = 0
	switch cmd {
	case 0x20, 0x21, 0xC4: // read sectors
		s.writing = false
		io.ideReadSector()
		s.irqCountdown = 1
	case 0x30, 0x31, 0xC5: // write sectors
		s.writing = true
		s.status = ideDRDY | ideDRQ
	case 0xE7, 0xEA: // flush cache
		s.status = ideDRDY
		s.irqCountdown = 1
	default:
		s.status = ideDRDY | ideERR
	}
}

func (io *IO) ideReadSector() {
	s := &io.ide
	d := io.hdds[s.drive]
	if d == nil {
		s.status = ideDRDY | ideERR
		return
	}
	if err := d.ReadSector(s.lba, s.buf[:]); err != nil {
		s.status = ideDRDY | ideERR
		return
	}
	s.status = ideDRDY | ideDRQ
}

// ---------------------------------------------------------------------------
// 16550 UART
// ---------------------------------------------------------------------------

type uartState struct {
	rx   []byte
	ier  uint8
	lcr  uint8
	mcr  uint8
	dll  uint8
	dlm  uint8
	fifo uint8
}

func (io *IO) uartIn(port uint16) uint8 {
	s := &io.uart
	dlab := s.lcr&0x80 != 0
	switch port - 0x03F8 {
	case 0:
		if dlab {
			return s.dll
		}
		if len(s.rx) == 0 {
			return 0
		}
		b := s.rx[0]
		s.rx = s.rx[1:]
		if len(s.rx) > 0 && s.ier&0x01 != 0 {
			io.raiseIRQ(irqCom1)
		}
		return b
	case 1:
		if dlab {
			return s.dlm
		}
		return s.ier
	case 2: // interrupt identification
		if len(s.rx) > 0 {
			return 0x04 // received data available
		}
		return 0x01 // no interrupt pending
	case 3:
		return s.lcr
	case 4:
		return s.mcr
	case 5: // line status: transmitter always ready
		lsr := uint8(0x60)
		if len(s.rx) > 0 {
			lsr |= 0x01
		}
		return lsr
	case 6: // modem status
		return 0xB0
	}
	return 0
}

func (io *IO) uartOut(port uint16, value uint8) {
	s := &io.uart
	dlab := s.lcr&0x80 != 0
	switch port - 0x03F8 {
	case 0:
		if dlab {
			s.dll = value
			return
		}
		if io.putc != nil {
			io.putc(value)
		}
	case 1:
		if dlab {
			s.dlm = value
		} else {
			s.ier = value
			if s.ier&0x01 != 0 && len(s.rx) > 0 {
				io.raiseIRQ(irqCom1)
			}
		}
	case 2:
		s.fifo = value
	case 3:
		s.lcr = value
	case 4:
		s.mcr = value
	}
}

// ---------------------------------------------------------------------------
// CGA CRT controller (only the cursor position registers are needed)
// ---------------------------------------------------------------------------

type crtState struct {
	index uint8
	regs  [32]uint8
}

func (io *IO) crtIn(port uint16) uint8 {
	if port == 0x03D4 {
		return io.crt.index
	}
	return io.crt.regs[io.crt.index&0x1F]
}

func (io *IO) crtOut(port uint16, value uint8) {
	if port == 0x03D4 {
		io.crt.index = value
		return
	}
	io.crt.regs[io.crt.index&0x1F] = value
}

// ---------------------------------------------------------------------------
// memory mapped devices: local APIC and I/O APIC
// ---------------------------------------------------------------------------

const (
	lapicID    = 0x0020 / 4
	lapicVER   = 0x0030 / 4
	lapicTPR   = 0x0080 / 4
	lapicEOI   = 0x00B0 / 4
	lapicSVR   = 0x00F0 / 4
	lapicESR   = 0x0280 / 4
	lapicICRLO = 0x0300 / 4
	lapicICRHI = 0x0310 / 4
	lapicTIMER = 0x0320 / 4
	lapicPCINT = 0x0340 / 4
	lapicLINT0 = 0x0350 / 4
	lapicLINT1 = 0x0360 / 4
	lapicERROR = 0x0370 / 4
	lapicTICR  = 0x0380 / 4
	lapicTCCR  = 0x0390 / 4
	lapicTDCR  = 0x03E0 / 4

	lapicMasked   = 0x00010000
	lapicEnabled  = 0x00000100
	lapicDelivs   = 0x00001000
	lapicPeriodic = 0x00020000
)

type lapicState struct {
	regs     [1024]uint32
	lastTick uint64
}

func (l *lapicState) timerEnabled() bool {
	return l.regs[lapicSVR]&lapicEnabled != 0 &&
		l.regs[lapicTICR] != 0 &&
		l.regs[lapicTIMER]&lapicMasked == 0
}

type ioapicState struct {
	index uint32
	regs  [4]uint32
	redir [24]uint64
}

func (io *IO) mmioRead(addr uint32, size int) uint32 {
	switch {
	case addr >= LocalAPICBase && addr < LocalAPICBase+0x1000:
		reg := (addr - LocalAPICBase) / 4
		switch reg {
		case lapicID:
			return 0 // this cpu is apic id 0
		case lapicICRLO:
			return io.lapic.regs[reg] &^ lapicDelivs
		case lapicTCCR:
			return 0
		}
		return io.lapic.regs[reg]

	case addr >= IOAPICBase && addr < IOAPICBase+0x1000:
		switch addr - IOAPICBase {
		case 0x00:
			return io.ioapi.index
		case 0x10:
			return io.ioapicRead(io.ioapi.index)
		}
	}
	return 0
}

func (io *IO) mmioWrite(addr, value uint32, size int) {
	switch {
	case addr >= LocalAPICBase && addr < LocalAPICBase+0x1000:
		reg := (addr - LocalAPICBase) / 4
		io.lapic.regs[reg] = value
		switch reg {
		case lapicTICR:
			io.lapic.lastTick = io.e.instCount
		case lapicICRLO:
			io.lapic.regs[reg] &^= lapicDelivs
		}

	case addr >= IOAPICBase && addr < IOAPICBase+0x1000:
		switch addr - IOAPICBase {
		case 0x00:
			io.ioapi.index = value
		case 0x10:
			io.ioapicWrite(io.ioapi.index, value)
		}
	}
}

func (io *IO) ioapicRead(index uint32) uint32 {
	switch {
	case index == 0x00: // id
		return io.ioapi.regs[0]
	case index == 0x01: // version, max redirection entry
		return 0x00170011
	case index >= 0x10 && index < 0x10+2*24:
		entry := io.ioapi.redir[(index-0x10)/2]
		if index&1 == 0 {
			return uint32(entry)
		}
		return uint32(entry >> 32)
	}
	return 0
}

func (io *IO) ioapicWrite(index, value uint32) {
	switch {
	case index == 0x00:
		io.ioapi.regs[0] = value
	case index >= 0x10 && index < 0x10+2*24:
		i := (index - 0x10) / 2
		if index&1 == 0 {
			io.ioapi.redir[i] = (io.ioapi.redir[i] &^ 0xFFFFFFFF) | uint64(value)
		} else {
			io.ioapi.redir[i] = (io.ioapi.redir[i] & 0xFFFFFFFF) | uint64(value)<<32
		}
	}
}
