package main

import "io"

const (
	// EBDABase is the address of the (fake) BIOS extended data area which
	// holds the MP floating pointer structure.
	EBDABase = uint32(0x600)

	// MpConfigTableBase is the address of the MP configuration table.
	MpConfigTableBase = uint32(0x700)

	// LocalAPICBase is the physical address of the local APIC registers.
	LocalAPICBase = uint32(0xFEE00000)

	// IOAPICBase is the physical address of the I/O APIC registers.
	IOAPICBase = uint32(0xFEC00000)

	// PHYSTOP is the size of the physical memory of the machine. It has to
	// match the PHYSTOP of the guest (see xv6-public/memlayout.h).
	PHYSTOP = uint32(0xE000000)

	// DEVSPACE is the start of the memory mapped devices.
	DEVSPACE = uint32(0xFE000000)
)

func getMpConf() [72]byte {
	var mpconf [72]byte

	// configuration table header (struct mpconf)
	mpconf[0] = 'P'                                  // signature
	mpconf[1] = 'C'                                  // signature
	mpconf[2] = 'M'                                  // signature
	mpconf[3] = 'P'                                  // signature
	mpconf[4] = 72                                   // length
	mpconf[5] = 0                                    // length
	mpconf[6] = 1                                    // version
	mpconf[7] = 0                                    // checksum, filled in below
	mpconf[8] = 0                                    // product id (uchar [20])
	mpconf[28] = 0                                   // OEM table pointer
	mpconf[32] = 0                                   // OEM table length
	mpconf[34] = 2                                   // entry count
	mpconf[36] = uint8((LocalAPICBase >> 0) & 0xFF)  // address of local APIC
	mpconf[37] = uint8((LocalAPICBase >> 8) & 0xFF)  // address of local APIC
	mpconf[38] = uint8((LocalAPICBase >> 16) & 0xFF) // address of local APIC
	mpconf[39] = uint8((LocalAPICBase >> 24) & 0xFF) // address of local APIC
	mpconf[40] = 0                                   // extended table length
	mpconf[42] = 0                                   // extended table checksum
	mpconf[43] = 0                                   // reserved

	// processor table entry (struct mpproc)
	mpconf[44] = 0    // entry type(0)
	mpconf[45] = 0    // local APIC id
	mpconf[46] = 0x14 // local APIC version
	mpconf[47] = 0x03 // cpu flags: enabled, bootstrap processor
	mpconf[48] = 0    // cpu signature
	mpconf[52] = 0    // feature flags from CPUID instruction

	// I/O APIC table entry (struct mpioapic)
	mpconf[64] = 2                                // entry type(2)
	mpconf[65] = 0                                // I/O APIC id
	mpconf[66] = 0x11                             // I/O APIC version
	mpconf[67] = 0x01                             // I/O APIC flags: enabled
	mpconf[68] = uint8((IOAPICBase >> 0) & 0xFF)  // I/O APIC address
	mpconf[69] = uint8((IOAPICBase >> 8) & 0xFF)  // I/O APIC address
	mpconf[70] = uint8((IOAPICBase >> 16) & 0xFF) // I/O APIC address
	mpconf[71] = uint8((IOAPICBase >> 24) & 0xFF) // I/O APIC address

	// setup checksum for (struct mpconf)
	s := uint8(0)
	for i := uint32(0); i < uint32(mpconf[4]); i++ {
		s = s + mpconf[i]
	}
	mpconf[7] = uint8(0 - s)
	return mpconf
}

func getEBDA() [16]byte {
	ebda := [16]byte{
		'_', 'M', 'P', '_', // signature _MP_
		uint8((MpConfigTableBase >> 0) & 0xFF), // phys addr of MP config table
		uint8((MpConfigTableBase >> 8) & 0xFF),
		uint8((MpConfigTableBase >> 16) & 0xFF),
		uint8((MpConfigTableBase >> 24) & 0xFF),
		1,       // length 1
		1,       // specrev [14]
		0,       // checksum
		0,       // type
		0,       // imcrp
		0, 0, 0, // reserved
	}

	// setup checksum for (struct mp)
	s := uint8(0)
	for i := uint32(0); i < 16; i++ {
		s = s + ebda[i]
	}
	ebda[10] = uint8(0 - s)
	return ebda
}

// NewEmulator creates a new machine with PHYSTOP bytes of memory. The cpu
// starts in real mode unless protectedMode is set, in which case it starts
// with flat 32bit segments. Everything the guest writes to its console ends
// up in writer.
func NewEmulator(eip, esp uint32, protectedMode bool,
	writer io.Writer, disasm map[uint64]string) *Emulator {
	e := &Emulator{
		memory: make([]uint8, PHYSTOP),
		eip:    eip,
		writer: writer,
		disasm: disasm,
	}
	e.registers[EAX] = 0xaa55
	e.registers[EDX] = 0x80 // the boot drive number
	e.registers[ESP] = esp
	e.eflags = 2
	e.cr[0] = 0x10

	for i := range e.sreg {
		e.sreg[i] = Segment{limit: 0xFFFF, present: true}
	}
	if protectedMode {
		e.cr[0] |= CR0ProtectedModeEnable
		for i := range e.sreg {
			e.sreg[i] = Segment{limit: 0xFFFFFFFF, present: true, db: true, typ: 0x3}
		}
		e.sreg[CS].typ = 0xB
	}

	e.io = NewIO(e, func(b byte) {
		if e.writer != nil {
			e.writer.Write([]byte{b})
		}
	})

	// setup BDA (BIOS Data Area)
	e.memory[0x040E] = uint8(EBDABase >> 4)
	e.memory[0x040F] = uint8(EBDABase >> 12)

	// setup EBDA (struct mp) at EBDABase
	for i, val := range getEBDA() {
		e.memory[EBDABase+uint32(i)] = val
	}

	// setup (struct mpconf) at MpConfigTableBase
	for i, val := range getMpConf() {
		e.memory[MpConfigTableBase+uint32(i)] = val
	}

	return e
}

// SetConsoleOutput redirects the bytes written to the serial port.
func (e *Emulator) SetConsoleOutput(f func(byte)) {
	e.io.putc = f
}

// InstructionCount returns the number of executed instructions.
func (e *Emulator) InstructionCount() uint64 { return e.instCount }

// Halted reports whether the cpu is waiting for an interrupt.
func (e *Emulator) Halted() bool { return e.halted }

// Shutdown reports whether the machine stopped for good.
func (e *Emulator) Shutdown() bool { return e.shutdown }

func (e *Emulator) traceInst() {
	name := e.disasm[uint64(e.eip)]
	printf("%08d eip=0x%08x cs=0x%02x %-30s eax=%08x ebx=%08x ecx=%08x edx=%08x esi=%08x edi=%08x esp=%08x ebp=%08x\n",
		e.instCount, e.eip, e.sreg[CS].selector, name,
		e.registers[EAX], e.registers[EBX], e.registers[ECX], e.registers[EDX],
		e.registers[ESI], e.registers[EDI], e.registers[ESP], e.registers[EBP])
}

func (e *Emulator) dump(index int) {
	printf("%10d ----------------------------------------------\n", index)
	printf("EIP=0x%08x  EFLAGS=0x%08x  CPL=%d  CR0=0x%08x CR3=0x%08x\n",
		e.eip, uint32(e.eflags), e.cpl, e.cr[0], e.cr[3])
	names := []string{"EAX", "ECX", "EDX", "EBX", "ESP", "EBP", "ESI", "EDI"}
	for i, n := range names {
		printf("%s=0x%08x%s", n, e.registers[i], map[bool]string{true: "\n", false: " "}[i%4 == 3])
	}
	printf("CS=0x%04x(base=0x%08x) DS=0x%04x SS=0x%04x ES=0x%04x FS=0x%04x GS=0x%04x(base=0x%08x)\n",
		e.sreg[CS].selector, e.sreg[CS].base, e.sreg[DS].selector, e.sreg[SS].selector,
		e.sreg[ES].selector, e.sreg[FS].selector, e.sreg[GS].selector, e.sreg[GS].base)
	e.eflags.dump()
}

func (e *Emulator) dumpGDTEntry(physAddr uint32) {
	desc := e.readPhys64(physAddr)
	s := decodeDescriptor(desc)
	printf("GDTEntry{addr=0x%x base=0x%08x limit=0x%08x type=0x%x dpl=%d present=%v db=%v}\n",
		physAddr, s.base, s.limit, s.typ, s.dpl, s.present, s.db)
}
