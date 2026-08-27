package main

import (
	"fmt"
	"io"
)

// 32bit registers
const (
	EAX = 0
	ECX = 1
	EDX = 2
	EBX = 3
	ESP = 4
	EBP = 5
	ESI = 6
	EDI = 7
)

// 16bit register
const (
	AX = EAX
	CX = ECX
	DX = EDX
	BX = EBX
	SP = ESP
	BP = EBP
	SI = ESI
	DI = EDI
)

// 8bit register
const (
	AL = EAX
	CL = ECX
	DL = EDX
	BL = EBX
	AH = AL + 4
	CH = CL + 4
	DH = DL + 4
	BH = BL + 4
)

// segment register
const (
	ES = 0
	CS = 1
	SS = 2
	DS = 3
	FS = 4
	GS = 5
)

// Control register flags
const (
	CR0ProtectedModeEnable = uint32(1) << 0
	CR0WriteProtect        = uint32(1) << 16
	CR0PagingFlag          = uint32(1) << 31
	CR4PageSizeExtension   = uint32(1) << 4
)

// Page table entry flags
const (
	PtePresent  = uint32(1) << 0
	PteWritable = uint32(1) << 1
	PteUser     = uint32(1) << 2
	PteAccessed = uint32(1) << 5
	PteDirty    = uint32(1) << 6
	PtePageSize = uint32(1) << 7
)

// Exception vectors
const (
	ExDivideError    = 0
	ExDebug          = 1
	ExBreakpoint     = 3
	ExOverflow       = 4
	ExBoundRange     = 5
	ExInvalidOpcode  = 6
	ExDeviceNotAvail = 7
	ExDoubleFault    = 8
	ExInvalidTSS     = 10
	ExSegmentNotPres = 11
	ExStackFault     = 12
	ExGeneralProtect = 13
	ExPageFault      = 14
)

// Segment is a segment register with its cached descriptor.
type Segment struct {
	selector uint16
	base     uint32
	limit    uint32 // in bytes, already scaled by the granularity bit
	typ      uint8  // descriptor type field
	dpl      uint8
	present  bool
	system   bool // true if this is a system descriptor (S bit is 0)
	db       bool // default operand/address size is 32bit
}

// DescriptorTable is the content of GDTR/IDTR.
type DescriptorTable struct {
	base  uint32
	limit uint16
}

// TaskRegister contains the selector and the cached descriptor of the TSS.
type TaskRegister struct {
	selector uint16
	base     uint32
	limit    uint32
}

// cpuException is thrown (as a panic) when the guest triggers a fault. It is
// recovered at the instruction boundary and turned into an IDT dispatch.
type cpuException struct {
	vector  int
	errCode uint32
	hasErr  bool
}

// cpuAbort is thrown when the emulator itself cannot continue, e.g. because an
// instruction is not implemented.
type cpuAbort struct {
	msg string
}

// Emulator is an i386 virtual machine.
type Emulator struct {
	registers [8]uint32  // general purpose registers
	eip       uint32     // program counter
	eflags    Eflags     // status register
	sreg      [6]Segment // segment registers
	cr        [8]uint32  // control registers
	gdtr      DescriptorTable
	idtr      DescriptorTable
	tr        TaskRegister
	cpl       uint8   // current privilege level
	memory    []uint8 // physical memory
	io        *IO     // i/o ports and devices
	halted    bool    // waiting for an interrupt (hlt)
	shutdown  bool    // the guest asked to power off

	// interrupt state
	irr      [256]bool // pending interrupt requests, indexed by vector
	numIRR   int
	intAfter int // deliver a pending interrupt only after N instructions (sti shadow)

	// decoding state of the instruction which is being executed
	instEIP   uint32 // eip of the instruction being executed
	opSize    int    // operand size in bits (16 or 32)
	addrSize  int    // address size in bits (16 or 32)
	segPrefix int    // segment override, or -1
	repPrefix uint8  // 0, 0xf2 or 0xf3

	// caches
	tlb      [tlbSize]tlbEntry
	codeTag  uint32 // page number of the cached instruction page + 1 (0 = invalid)
	codePhys uint32 // physical base address of the cached instruction page

	instCount uint64    // number of executed instructions
	writer    io.Writer // the console of the guest
	trace     bool
	disasm    map[uint64]string
}

const tlbSize = 256

type tlbEntry struct {
	valid    bool
	tag      uint32 // linear page number
	phys     uint32 // physical page base
	writable bool
	user     bool
}

func (e *Emulator) pagingEnabled() bool {
	return e.cr[0]&CR0PagingFlag != 0
}

func (e *Emulator) protectedMode() bool {
	return e.cr[0]&CR0ProtectedModeEnable != 0
}

func (e *Emulator) flushTLB() {
	for i := range e.tlb {
		e.tlb[i].valid = false
	}
	e.codeTag = 0
}

func throwException(vector int, errCode uint32, hasErr bool) {
	panic(cpuException{vector: vector, errCode: errCode, hasErr: hasErr})
}

func abort(format string, a ...interface{}) {
	panic(cpuAbort{msg: fmt.Sprintf(format, a...)})
}

// ---------------------------------------------------------------------------
// registers
// ---------------------------------------------------------------------------

func (e *Emulator) getRegister8(index uint8) uint8 {
	if index < 4 {
		return uint8(e.registers[index])
	}
	return uint8(e.registers[index-4] >> 8)
}

func (e *Emulator) setRegister8(index, value uint8) {
	if index < 4 {
		e.registers[index] = (e.registers[index] & 0xFFFFFF00) | uint32(value)
	} else {
		e.registers[index-4] = (e.registers[index-4] & 0xFFFF00FF) | (uint32(value) << 8)
	}
}

func (e *Emulator) getRegister16(index uint8) uint16 {
	return uint16(e.registers[index])
}

func (e *Emulator) setRegister16(index uint8, value uint16) {
	e.registers[index] = (e.registers[index] & 0xFFFF0000) | uint32(value)
}

func (e *Emulator) getRegister32(index uint8) uint32 {
	return e.registers[index]
}

func (e *Emulator) setRegister32(index uint8, value uint32) {
	e.registers[index] = value
}

func (e *Emulator) getReg(index uint8, size int) uint32 {
	switch size {
	case 8:
		return uint32(e.getRegister8(index))
	case 16:
		return uint32(e.getRegister16(index))
	default:
		return e.registers[index]
	}
}

func (e *Emulator) setReg(index uint8, size int, value uint32) {
	switch size {
	case 8:
		e.setRegister8(index, uint8(value))
	case 16:
		e.setRegister16(index, uint16(value))
	default:
		e.registers[index] = value
	}
}

func szMask(size int) uint32 {
	switch size {
	case 8:
		return 0xFF
	case 16:
		return 0xFFFF
	}
	return 0xFFFFFFFF
}

func szSign(size int) uint32 {
	switch size {
	case 8:
		return 0x80
	case 16:
		return 0x8000
	}
	return 0x80000000
}

func signExtend(value uint32, size int) uint32 {
	switch size {
	case 8:
		if value&0x80 != 0 {
			return value | 0xFFFFFF00
		}
		return value & 0xFF
	case 16:
		if value&0x8000 != 0 {
			return value | 0xFFFF0000
		}
		return value & 0xFFFF
	}
	return value
}

// ---------------------------------------------------------------------------
// segmentation
// ---------------------------------------------------------------------------

func (e *Emulator) descriptorAt(selector uint16) (uint64, bool) {
	if selector&0xFFF8 == 0 {
		return 0, false
	}
	offset := uint32(selector & 0xFFF8)
	if offset+7 > uint32(e.gdtr.limit) {
		return 0, false
	}
	return e.readPhys64(e.translate(e.gdtr.base+offset, false, false)), true
}

func decodeDescriptor(desc uint64) Segment {
	var s Segment
	s.base = uint32(((desc >> 16) & 0xFFFFFF) | (((desc >> 56) & 0xFF) << 24))
	s.limit = uint32(desc&0xFFFF) | uint32((desc>>32)&0xF0000)
	s.typ = uint8((desc >> 40) & 0xF)
	s.system = (desc>>44)&0x1 == 0
	s.dpl = uint8((desc >> 45) & 0x3)
	s.present = (desc>>47)&0x1 == 1
	s.db = (desc>>54)&0x1 == 1
	if (desc>>55)&0x1 == 1 {
		s.limit = (s.limit << 12) | 0xFFF
	}
	return s
}

// loadSegment loads a segment register and caches its descriptor.
func (e *Emulator) loadSegment(index int, selector uint16) {
	if !e.protectedMode() {
		e.sreg[index] = Segment{
			selector: selector,
			base:     uint32(selector) << 4,
			limit:    0xFFFF,
			present:  true,
			db:       false,
		}
		if index == CS {
			e.codeTag = 0
		}
		return
	}

	if selector&0xFFF8 == 0 {
		if index == CS || index == SS {
			throwException(ExGeneralProtect, 0, true)
		}
		e.sreg[index] = Segment{selector: selector}
		return
	}

	desc, ok := e.descriptorAt(selector)
	if !ok {
		throwException(ExGeneralProtect, uint32(selector&0xFFFC), true)
	}
	s := decodeDescriptor(desc)
	if !s.present {
		throwException(ExSegmentNotPres, uint32(selector&0xFFFC), true)
	}
	s.selector = selector
	e.sreg[index] = s
	if index == CS {
		e.cpl = uint8(selector & 0x3)
		e.codeTag = 0
	}
}

func (e *Emulator) segmentBase(index int) uint32 {
	return e.sreg[index].base
}

// stackAddressSize returns the size of ESP/SP used for stack operations.
func (e *Emulator) stackAddressSize() int {
	if e.sreg[SS].db {
		return 32
	}
	if !e.protectedMode() {
		return 16
	}
	return 32
}

// ---------------------------------------------------------------------------
// memory access
// ---------------------------------------------------------------------------

func (e *Emulator) pageFault(linear uint32, present, write, user bool) {
	var code uint32
	if present {
		code |= 0x1
	}
	if write {
		code |= 0x2
	}
	if user {
		code |= 0x4
	}
	e.cr[2] = linear
	throwException(ExPageFault, code, true)
}

// translate converts a linear address into a physical address.
func (e *Emulator) translate(linear uint32, write, user bool) uint32 {
	if !e.pagingEnabled() {
		return linear
	}

	tag := linear >> 12
	ent := &e.tlb[tag&(tlbSize-1)]
	if ent.valid && ent.tag == tag && (!write || ent.writable) && (!user || ent.user) {
		return ent.phys | (linear & 0xFFF)
	}

	pdeAddr := (e.cr[3] & 0xFFFFF000) + ((linear>>22)&0x3FF)*4
	pde := e.readPhys32(pdeAddr)
	if pde&PtePresent == 0 {
		e.pageFault(linear, false, write, user)
	}

	var phys, flags uint32
	if pde&PtePageSize != 0 && e.cr[4]&CR4PageSizeExtension != 0 {
		flags = pde
		phys = (pde & 0xFFC00000) | (linear & 0x3FF000)
		e.writePhys32(pdeAddr, pde|PteAccessed)
	} else {
		pteAddr := (pde & 0xFFFFF000) + ((linear>>12)&0x3FF)*4
		pte := e.readPhys32(pteAddr)
		if pte&PtePresent == 0 {
			e.pageFault(linear, false, write, user)
		}
		flags = pte & pde
		phys = pte & 0xFFFFF000
		e.writePhys32(pteAddr, pte|PteAccessed)
		e.writePhys32(pdeAddr, pde|PteAccessed)
	}

	if user && flags&PteUser == 0 {
		e.pageFault(linear, true, write, user)
	}
	if write && flags&PteWritable == 0 && (user || e.cr[0]&CR0WriteProtect != 0) {
		e.pageFault(linear, true, write, user)
	}

	ent.valid = true
	ent.tag = tag
	ent.phys = phys
	ent.writable = flags&PteWritable != 0
	ent.user = flags&PteUser != 0
	return phys | (linear & 0xFFF)
}

func (e *Emulator) readPhys8(addr uint32) uint8 {
	if addr >= DEVSPACE {
		return uint8(e.io.mmioRead(addr, 8))
	}
	if int(addr) >= len(e.memory) {
		return 0xFF
	}
	return e.memory[addr]
}

func (e *Emulator) readPhys16(addr uint32) uint16 {
	if addr >= DEVSPACE {
		return uint16(e.io.mmioRead(addr, 16))
	}
	if int(addr)+1 >= len(e.memory) {
		return 0xFFFF
	}
	return uint16(e.memory[addr]) | uint16(e.memory[addr+1])<<8
}

func (e *Emulator) readPhys32(addr uint32) uint32 {
	if addr >= DEVSPACE {
		return e.io.mmioRead(addr, 32)
	}
	if int(addr)+3 >= len(e.memory) {
		return 0xFFFFFFFF
	}
	m := e.memory[addr : addr+4 : addr+4]
	return uint32(m[0]) | uint32(m[1])<<8 | uint32(m[2])<<16 | uint32(m[3])<<24
}

func (e *Emulator) readPhys64(addr uint32) uint64 {
	return uint64(e.readPhys32(addr)) | uint64(e.readPhys32(addr+4))<<32
}

func (e *Emulator) writePhys8(addr uint32, value uint8) {
	if addr >= DEVSPACE {
		e.io.mmioWrite(addr, uint32(value), 8)
		return
	}
	if int(addr) >= len(e.memory) {
		return
	}
	e.memory[addr] = value
}

func (e *Emulator) writePhys16(addr uint32, value uint16) {
	if addr >= DEVSPACE {
		e.io.mmioWrite(addr, uint32(value), 16)
		return
	}
	if int(addr)+1 >= len(e.memory) {
		return
	}
	e.memory[addr] = uint8(value)
	e.memory[addr+1] = uint8(value >> 8)
}

func (e *Emulator) writePhys32(addr, value uint32) {
	if addr >= DEVSPACE {
		e.io.mmioWrite(addr, value, 32)
		return
	}
	if int(addr)+3 >= len(e.memory) {
		return
	}
	m := e.memory[addr : addr+4 : addr+4]
	m[0] = uint8(value)
	m[1] = uint8(value >> 8)
	m[2] = uint8(value >> 16)
	m[3] = uint8(value >> 24)
}

// readLinear reads size bits from a linear address.
func (e *Emulator) readLinear(linear uint32, size int) uint32 {
	user := e.cpl == 3
	if size == 8 {
		return uint32(e.readPhys8(e.translate(linear, false, user)))
	}
	if linear&0xFFF <= uint32(0x1000-size/8) {
		phys := e.translate(linear, false, user)
		if size == 16 {
			return uint32(e.readPhys16(phys))
		}
		return e.readPhys32(phys)
	}
	// the access crosses a page boundary
	var ret uint32
	for i := 0; i < size/8; i++ {
		ret |= uint32(e.readPhys8(e.translate(linear+uint32(i), false, user))) << uint(8*i)
	}
	return ret
}

// writeLinear writes size bits to a linear address.
func (e *Emulator) writeLinear(linear uint32, size int, value uint32) {
	user := e.cpl == 3
	if size == 8 {
		e.writePhys8(e.translate(linear, true, user), uint8(value))
		return
	}
	if linear&0xFFF <= uint32(0x1000-size/8) {
		phys := e.translate(linear, true, user)
		if size == 16 {
			e.writePhys16(phys, uint16(value))
		} else {
			e.writePhys32(phys, value)
		}
		return
	}
	for i := 0; i < size/8; i++ {
		e.writePhys8(e.translate(linear+uint32(i), true, user), uint8(value>>uint(8*i)))
	}
}

func (e *Emulator) readSeg(seg int, offset uint32, size int) uint32 {
	return e.readLinear(e.sreg[seg].base+offset, size)
}

func (e *Emulator) writeSeg(seg int, offset uint32, size int, value uint32) {
	e.writeLinear(e.sreg[seg].base+offset, size, value)
}

// getMemory8 and friends are kept for tests and debug helpers. They interpret
// the address as a linear (kernel) address.
func (e *Emulator) getMemory8(address uint32) uint8 {
	return uint8(e.readLinear(address, 8))
}

func (e *Emulator) getMemory16(address uint32) uint16 {
	return uint16(e.readLinear(address, 16))
}

func (e *Emulator) getMemory32(address uint32) uint32 {
	return e.readLinear(address, 32)
}

func (e *Emulator) setMemory8(address uint32, value uint8) {
	e.writeLinear(address, 8, uint32(value))
}

func (e *Emulator) setMemory32(address, value uint32) {
	e.writeLinear(address, 32, value)
}

// ---------------------------------------------------------------------------
// instruction fetch
// ---------------------------------------------------------------------------

func (e *Emulator) fetch8() uint8 {
	linear := e.sreg[CS].base + e.eip
	e.eip++
	tag := (linear >> 12) + 1
	if tag != e.codeTag {
		e.codePhys = e.translate(linear, false, e.cpl == 3) &^ 0xFFF
		e.codeTag = tag
	}
	return e.readPhys8(e.codePhys | (linear & 0xFFF))
}

func (e *Emulator) fetch16() uint16 {
	return uint16(e.fetch8()) | uint16(e.fetch8())<<8
}

func (e *Emulator) fetch32() uint32 {
	return uint32(e.fetch16()) | uint32(e.fetch16())<<16
}

func (e *Emulator) fetchImm(size int) uint32 {
	switch size {
	case 8:
		return uint32(e.fetch8())
	case 16:
		return uint32(e.fetch16())
	}
	return e.fetch32()
}

// ---------------------------------------------------------------------------
// stack
// ---------------------------------------------------------------------------

func (e *Emulator) push(value uint32, size int) {
	if e.stackAddressSize() == 16 {
		sp := (e.getRegister16(SP) - uint16(size/8))
		e.setRegister16(SP, sp)
		e.writeSeg(SS, uint32(sp), size, value)
		return
	}
	esp := e.registers[ESP] - uint32(size/8)
	e.registers[ESP] = esp
	e.writeSeg(SS, esp, size, value)
}

func (e *Emulator) pop(size int) uint32 {
	if e.stackAddressSize() == 16 {
		sp := e.getRegister16(SP)
		v := e.readSeg(SS, uint32(sp), size)
		e.setRegister16(SP, sp+uint16(size/8))
		return v
	}
	esp := e.registers[ESP]
	v := e.readSeg(SS, esp, size)
	e.registers[ESP] = esp + uint32(size/8)
	return v
}

func (e *Emulator) push32(value uint32) { e.push(value, 32) }
func (e *Emulator) pop32() uint32       { return e.pop(32) }
