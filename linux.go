package main

import (
	"encoding/binary"
	"fmt"
)

// The 32bit boot protocol of Linux (see Documentation/arch/x86/boot.rst in the
// kernel tree) is a lot easier to implement than a BIOS: the boot loader (that
// is, this file) puts the protected mode kernel at 0x100000, fills in the boot
// parameters ("zero page") and jumps to the entry point with the cpu already
// in 32bit protected mode.
const (
	// BootParamsBase is where the zero page is built.
	BootParamsBase = uint32(0x00010000)

	// CmdlineBase is where the kernel command line is copied.
	CmdlineBase = uint32(0x00020000)

	// BootGDTBase is the GDT the kernel starts with. It only has to be
	// valid until the kernel loads its own one.
	BootGDTBase = uint32(0x00030000)

	// KernelBase is the load address of the protected mode kernel. It has
	// to match code32_start of the image (it always is 0x100000 for a
	// bzImage which is not relocatable).
	KernelBase = uint32(0x00100000)

	// BootCS and BootDS are the selectors the kernel expects.
	BootCS = uint16(0x10)
	BootDS = uint16(0x18)
)

// offsets inside the setup header (the part of the zero page which comes from
// the boot image itself)
const (
	hdrSetupSects    = 0x1F1
	hdrJump          = 0x201
	hdrMagic         = 0x202
	hdrVersion       = 0x206
	hdrTypeOfLoader  = 0x210
	hdrLoadflags     = 0x211
	hdrCode32Start   = 0x214
	hdrRamdiskImage  = 0x218
	hdrRamdiskSize   = 0x21C
	hdrHeapEndPtr    = 0x224
	hdrCmdlinePtr    = 0x228
	hdrInitrdAddrMax = 0x22C
	hdrCmdlineSize   = 0x238
	hdrInitSize      = 0x260

	// zero page fields which do not come from the image
	bpE820Entries = 0x1E8
	bpE820Table   = 0x2D0

	loadedHigh = 0x01
	canUseHeap = 0x80
)

// DefaultCmdline is the command line the emulator boots Linux with: the
// console is the serial port, and the userland comes from the initial ram
// disk.
const DefaultCmdline = "console=ttyS0 earlyprintk=serial,ttyS0 tsc=reliable"

// e820 memory types
const (
	e820RAM      = 1
	e820Reserved = 2
)

// LoadLinux prepares the machine to run a bzImage: the kernel and the initial
// ram disk are copied into the guest memory, the zero page is filled in and
// the cpu is put into the state the 32bit boot protocol asks for.
func LoadLinux(e *Emulator, bzImage, initrd []byte, cmdline string) error {
	if len(bzImage) < 0x300 {
		return fmt.Errorf("bzImage: too short (%d bytes)", len(bzImage))
	}
	if string(bzImage[hdrMagic:hdrMagic+4]) != "HdrS" {
		return fmt.Errorf("bzImage: no HdrS magic, this is not a Linux kernel image")
	}
	version := binary.LittleEndian.Uint16(bzImage[hdrVersion:])
	if version < 0x0206 {
		return fmt.Errorf("bzImage: boot protocol %d.%02d is too old",
			version>>8, version&0xFF)
	}

	setupSects := uint32(bzImage[hdrSetupSects])
	if setupSects == 0 {
		setupSects = 4
	}
	payload := (setupSects + 1) * SectorSize
	if uint32(len(bzImage)) <= payload {
		return fmt.Errorf("bzImage: truncated (setup is %d bytes)", payload)
	}
	kernel := bzImage[payload:]

	code32Start := binary.LittleEndian.Uint32(bzImage[hdrCode32Start:])
	if code32Start != KernelBase {
		return fmt.Errorf("bzImage: code32_start is 0x%x, only 0x%x is supported",
			code32Start, KernelBase)
	}
	initSize := binary.LittleEndian.Uint32(bzImage[hdrInitSize:])
	if KernelBase+initSize > PHYSTOP {
		return fmt.Errorf("bzImage: needs %d MB of memory, the machine has %d MB",
			(KernelBase+initSize)>>20, PHYSTOP>>20)
	}
	if err := writeMemory(e, KernelBase, kernel); err != nil {
		return err
	}

	// the zero page starts as a copy of the setup header of the image
	zeroPage := make([]byte, 4096)
	hdrEnd := uint32(0x202) + uint32(bzImage[hdrJump])
	if hdrEnd > 0x1000 {
		hdrEnd = 0x1000
	}
	copy(zeroPage[hdrSetupSects:hdrEnd], bzImage[hdrSetupSects:hdrEnd])

	zeroPage[hdrTypeOfLoader] = 0xFF // "undefined" boot loader
	zeroPage[hdrLoadflags] = zeroPage[hdrLoadflags]&^canUseHeap | loadedHigh
	binary.LittleEndian.PutUint32(zeroPage[hdrHeapEndPtr:], 0)
	binary.LittleEndian.PutUint32(zeroPage[hdrCmdlinePtr:], CmdlineBase)

	// the initial ram disk goes as high as it fits
	if len(initrd) > 0 {
		addrMax := binary.LittleEndian.Uint32(bzImage[hdrInitrdAddrMax:])
		if addrMax == 0 || addrMax > PHYSTOP-1 {
			addrMax = PHYSTOP - 1
		}
		if uint32(len(initrd)) > addrMax {
			return fmt.Errorf("initrd: %d bytes do not fit into memory", len(initrd))
		}
		addr := (addrMax - uint32(len(initrd)) + 1) &^ 0xFFF
		if addr < KernelBase+initSize {
			return fmt.Errorf("initrd: %d bytes do not fit above the kernel", len(initrd))
		}
		if err := writeMemory(e, addr, initrd); err != nil {
			return err
		}
		binary.LittleEndian.PutUint32(zeroPage[hdrRamdiskImage:], addr)
		binary.LittleEndian.PutUint32(zeroPage[hdrRamdiskSize:], uint32(len(initrd)))
	}

	// the memory map: the kernel only trusts what the boot loader tells it
	e820 := []struct {
		addr uint64
		size uint64
		typ  uint32
	}{
		{0x00000000, 0x0009FC00, e820RAM},
		{0x0009FC00, 0x00000400, e820Reserved}, // EBDA
		{0x000F0000, 0x00010000, e820Reserved}, // BIOS
		{0x00100000, uint64(PHYSTOP) - 0x100000, e820RAM},
	}
	zeroPage[bpE820Entries] = byte(len(e820))
	for i, ent := range e820 {
		off := bpE820Table + i*20
		binary.LittleEndian.PutUint64(zeroPage[off:], ent.addr)
		binary.LittleEndian.PutUint64(zeroPage[off+8:], ent.size)
		binary.LittleEndian.PutUint32(zeroPage[off+16:], ent.typ)
	}

	if err := writeMemory(e, BootParamsBase, zeroPage); err != nil {
		return err
	}

	maxCmdline := binary.LittleEndian.Uint32(bzImage[hdrCmdlineSize:])
	if maxCmdline != 0 && uint32(len(cmdline)) > maxCmdline {
		return fmt.Errorf("command line is longer than the %d bytes the kernel accepts",
			maxCmdline)
	}
	if err := writeMemory(e, CmdlineBase, append([]byte(cmdline), 0)); err != nil {
		return err
	}

	setupBootCPU(e, BootParamsBase, code32Start)
	return nil
}

// setupBootCPU brings the cpu into the state the 32bit boot protocol requires:
// protected mode with flat segments, paging and interrupts off, esi pointing
// at the zero page.
func setupBootCPU(e *Emulator, bootParams, entry uint32) {
	// a GDT with the two flat descriptors the kernel starts with
	gdt := []uint64{
		0,
		0,
		descriptor(0, 0xFFFFFFFF, 0xA), // __BOOT_CS: execute/read
		descriptor(0, 0xFFFFFFFF, 0x2), // __BOOT_DS: read/write
	}
	for i, desc := range gdt {
		e.writePhys32(BootGDTBase+uint32(i)*8, uint32(desc))
		e.writePhys32(BootGDTBase+uint32(i)*8+4, uint32(desc>>32))
	}
	e.gdtr = DescriptorTable{base: BootGDTBase, limit: uint16(len(gdt)*8 - 1)}
	e.idtr = DescriptorTable{}

	e.cr[0] = CR0ProtectedModeEnable | 0x10 // PE, ET; paging is off
	e.cr[2] = 0
	e.cr[3] = 0
	e.cr[4] = 0
	e.cpl = 0
	e.flushTLB()

	e.loadSegment(CS, BootCS)
	for _, seg := range []int{DS, ES, SS, FS, GS} {
		e.loadSegment(seg, BootDS)
	}

	for i := range e.registers {
		e.registers[i] = 0
	}
	e.registers[ESI] = bootParams
	e.registers[ESP] = BootParamsBase - 16
	e.eflags = 2 // interrupts disabled
	e.eip = entry
}

// descriptor builds a 32bit segment descriptor with a page granular limit.
func descriptor(base, limit uint32, typ uint8) uint64 {
	limit >>= 12
	return uint64(limit&0xFFFF) |
		uint64(base&0xFFFFFF)<<16 |
		uint64(typ)<<40 |
		1<<44 | // code/data segment
		1<<47 | // present
		uint64((limit>>16)&0xF)<<48 |
		1<<54 | // 32bit
		1<<55 | // 4KB granularity
		uint64(base>>24)<<56
}

func writeMemory(e *Emulator, addr uint32, data []byte) error {
	if uint64(addr)+uint64(len(data)) > uint64(len(e.memory)) {
		return fmt.Errorf("cannot load %d bytes at 0x%08x: out of memory",
			len(data), addr)
	}
	copy(e.memory[addr:], data)
	return nil
}
