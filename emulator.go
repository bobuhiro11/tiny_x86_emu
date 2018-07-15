package main

import (
	"errors"
	"fmt"
	"github.com/fatih/color"
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
	GS = 5
)

// Controll Register
const (
	CR4PageSizeExtension = 0x10
	CR0PagingFlag        = 0x1 << 31
)

const (
	EBDABase          = uint32(0x600) // address of struct mp
	MpConfigTableBase = uint32(0x700) // TODO: address of struct mpconf,need check
	LocalAPICBase     = uint32(0xFEC80000)
	IOAPICBase        = uint32(0xFEC00000)
)

// Emulator is an i386 Virtual Machine
type Emulator struct {
	registers              [8]uint32        // general registers
	cr                     [16]uint32       // controll registers
	sreg                   [6]uint32        // segment registers
	eflags                 Eflags           // eflags
	gdtrSize               uint16           // global table descriptor table's size
	gdtrBase               uint32           // global table descriptor table's base phys address
	memory                 map[uint32]uint8 // physical memory
	eip                    uint32           // program counter
	isSilent               bool             // silent mode
	reader                 io.Reader
	writer                 io.Writer
	io                     IO
	operandSizeOverride    bool              // true if operand size override (0x66) is enabled
	genuineProtectedEnable bool              // procted mode is refreshed only when sreg is changed
	disasm                 map[uint64]string // disasmed code (ex. 32255 -> "0000 add [bx+si],al")
}

func getMpConf() [72]byte {
	var mpconf [72]byte

	// configuration table header (struct mpconf)
	mpconf[0] = 'P'                                  // signature
	mpconf[1] = 'C'                                  // signature
	mpconf[2] = 'M'                                  // signature
	mpconf[3] = 'P'                                  // signature
	mpconf[4] = 72                                   // FIXME: length
	mpconf[5] = 0                                    // FIXME: length
	mpconf[6] = 1                                    // version
	mpconf[7] = 0                                    // FIXME: checksum
	mpconf[8] = 0                                    // product id (uchar [20])
	mpconf[28] = 0                                   // OEM table pointer
	mpconf[32] = 0                                   // OEM OEM table length
	mpconf[34] = 0                                   // entry count
	mpconf[36] = uint8((LocalAPICBase >> 0) & 0XFF)  // FIXME: adress of local APIC
	mpconf[37] = uint8((LocalAPICBase >> 8) & 0XFF)  // FIXME: adress of local APIC
	mpconf[38] = uint8((LocalAPICBase >> 16) & 0XFF) // FIXME: adress of local APIC
	mpconf[39] = uint8((LocalAPICBase >> 24) & 0XFF) // FIXME: adress of local APIC
	mpconf[40] = 0                                   // extended table length
	mpconf[42] = 0                                   // extended table checksum
	mpconf[43] = 0                                   // reserved

	// processor table entry (struct mpproc)
	mpconf[44] = 0 // entry type(0)
	mpconf[45] = 1 // FIXME: local APIC id
	mpconf[46] = 1 // local APIC version
	mpconf[47] = 0 // CPU flags
	mpconf[48] = 0 // CPU signature
	mpconf[49] = 0 // CPU signature
	mpconf[50] = 0 // CPU signature
	mpconf[51] = 0 // CPU signature
	mpconf[52] = 0 // feature flags from CPUID instruction
	mpconf[53] = 0 // feature flags from CPUID instruction
	mpconf[54] = 0 // feature flags from CPUID instruction
	mpconf[55] = 0 // feature flags from CPUID instruction
	mpconf[56] = 0 // reserved
	mpconf[57] = 0 // reserved
	mpconf[58] = 0 // reserved
	mpconf[59] = 0 // reserved
	mpconf[60] = 0 // reserved
	mpconf[61] = 0 // reserved
	mpconf[62] = 0 // reserved
	mpconf[63] = 0 // reserved

	// I/O APIC table entry (struct mpioapic)
	mpconf[64] = 2                                // entry type(2)
	mpconf[65] = 1                                // I/O APIC id
	mpconf[66] = 1                                // I/O APIC version
	mpconf[67] = 0                                // I/O APIC flags
	mpconf[68] = uint8((IOAPICBase >> 0) & 0XFF)  // I/O APIC address
	mpconf[69] = uint8((IOAPICBase >> 8) & 0XFF)  // I/O APIC address
	mpconf[70] = uint8((IOAPICBase >> 16) & 0XFF) // I/O APIC address
	mpconf[71] = uint8((IOAPICBase >> 24) & 0XFF) // I/O APIC address

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
		uint8((MpConfigTableBase >> 0) & 0XFF), // phys addr of MP config table
		uint8((MpConfigTableBase >> 8) & 0XFF),
		uint8((MpConfigTableBase >> 16) & 0XFF),
		uint8((MpConfigTableBase >> 24) & 0XFF),
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

// NewEmulator creates New Emulator
func NewEmulator(memorySize, eip, esp uint32, protectedMode, isSilent bool, reader io.Reader, writer io.Writer, disasm map[uint64]string) *Emulator {
	e := &Emulator{
		memory:   map[uint32]uint8{},
		eip:      eip,
		isSilent: isSilent,
		reader:   reader,
		writer:   writer,
		disasm:   disasm,
	}
	e.registers[EAX] = 0xaa55
	e.registers[EDX] = 0x80
	e.registers[ESP] = esp
	e.cr[0] = 0x10
	e.io = NewIO(&reader, &writer)
	e.eflags = 2
	if protectedMode {
		e.cr[0] |= 1
		e.genuineProtectedEnable = true
	}

	// setup BDA (BIOS Data Area)
	e.memory[0x040E] = uint8(EBDABase >> 4)
	e.memory[0x040F] = uint8(EBDABase >> 12)

	// setup EBDA (struct mp) at EBDABase
	fmt.Printf("EBDA address=0x%X\n", ((uint32(e.memory[0x040f])<<8)|uint32(e.memory[0x040e]))<<4)
	for i, val := range getEBDA() {
		e.memory[EBDABase+uint32(i)] = val
	}

	// setup (struct mpconf) at MpConfigTableBase
	for i, val := range getMpConf() {
		e.memory[MpConfigTableBase+uint32(i)] = val
	}

	return e
}

// emulate instruction

func (e *Emulator) execInst() error {
	switch e.getCode8(0) {
	case 0x01:
		e.addRm32R32()
	case 0x03:
		e.addR32Rm32()
	case 0x05:
		e.addEaxImm32()
	case 0x09:
		e.orRm32R32()
	case 0x0B:
		e.orR32Rm32()
	case 0x0D:
		e.orEaxImm32()
	case 0x0F:
		e.code0f()
	case 0x25:
		if (e.genuineProtectedEnable && !e.operandSizeOverride) || (!e.genuineProtectedEnable && e.operandSizeOverride) {
			e.andEaxImm32()
		} else {
			e.andAxImm16()
		}
	case 0x29:
		e.subRm32R32()
	case 0x2D:
		e.subEaxImm32()
	case 0x31:
		if (e.genuineProtectedEnable && !e.operandSizeOverride) || (!e.genuineProtectedEnable && e.operandSizeOverride) {
			e.xorRm32R32()
		} else {
			e.xorRm16R16()
		}
	case 0x38:
		e.cmpRm8R8()
	case 0x39:
		e.cmpRm32R32()
	case 0x3b:
		e.cmpR32Rm32()
	case 0x3c:
		e.cmpAlImm8()
	case 0x3d:
		e.cmpEaxImm32()
	case 0x40, 0x41, 0x42, 0x43, 0x44, 0x45, 0x46, 0x47:
		e.incR32()
	case 0x48, 0x49, 0x4a, 0x4b, 0x4c, 0x4d, 0x4e, 0x4f:
		e.decR32()
	case 0x50, 0x51, 0x52, 0x53, 0x54, 0x55, 0x56, 0x57:
		e.pushR32()
	case 0x58, 0x59, 0x5a, 0x5b, 0x5c, 0x5d, 0x5e, 0x5f:
		e.popR32()
	case 0x66:
		e.operandSizeOverride = true
		e.eip++
		e.execInst()
		e.operandSizeOverride = false
	case 0x68:
		if (e.genuineProtectedEnable && !e.operandSizeOverride) || (!e.genuineProtectedEnable && e.operandSizeOverride) {
			e.pushImm32()
		} else {
			e.pushImm16()
		}
	case 0x69:
		e.imulR32Rm32Imm32()
	case 0x6a:
		e.pushImm8()
	case 0x6d:
		e.insd()
	case 0x71:
		e.jno()
	case 0x72:
		e.jb()
	case 0x73:
		e.jae()
	case 0x74:
		e.jz()
	case 0x75:
		e.jnz()
	case 0x76:
		e.jna()
	case 0x77:
		e.ja()
	case 0x78:
		e.js()
	case 0x79:
		e.jns()
	case 0x84:
		e.testRm8R8()
	case 0x85:
		e.testRm32R32()
	case 0x7A:
		e.jp()
	case 0x7B:
		e.jpo()
	case 0x7C:
		e.jl()
	case 0x7D:
		e.jc()
	case 0x7E:
		e.jng()
	case 0x7F:
		e.jg()
	case 0x80:
		e.code80()
		// e.orRm8Imm8()
	case 0x81:
		e.code81()
	case 0x83:
		e.code83()
	case 0x88:
		e.movRm8R8()
	case 0x89:
		e.movRm32R32()
	case 0x9C:
		e.pushf()
	case 0x8A:
		e.movR8Rm8()
	case 0x8B:
		e.movR32Rm32()
	case 0x8D:
		e.leaR32Rm32()
	case 0xA1:
		e.movEaxMoffs32()
	case 0xA3:
		e.movMoffs32Eax()
	case 0xA8:
		e.testAlImm8()
	case 0xA9:
		if (e.genuineProtectedEnable && !e.operandSizeOverride) || (!e.genuineProtectedEnable && e.operandSizeOverride) {
			e.testEaxImm32()
		} else {
			e.testAxImm16()
		}
	case 0x8E:
		e.movSregRm16() // 16 bit mode
	case 0xB0, 0xB1, 0xB2, 0xB3, 0xB4, 0xB5, 0xB6, 0xB7:
		e.movR8Imm8()
	case 0x90:
		e.nop()
	case 0xAA:
		e.stosb()
	case 0xAB:
		e.stosd()
	case 0xC1:
		e.codeC1()
	case 0xC3:
		e.ret()
	case 0xC6:
		e.movRm8Imm8()
	case 0xC7:
		e.movRm32Imm32()
	case 0xC9:
		e.leave()
	case 0xCD:
		e.intImm8()
	case 0xEB:
		e.shortJmp()
	case 0xE4:
		e.inAlImm8()
	case 0xE5:
		if e.genuineProtectedEnable {
			e.inEaxImm8()
		} else {
			e.inAxImm8()
		}
	case 0xE6:
		e.outAlImm8()
	case 0xB8, 0xB9, 0xBA, 0xBB, 0xBC, 0xBD, 0xBE, 0xBF:
		if (e.genuineProtectedEnable && !e.operandSizeOverride) || (!e.genuineProtectedEnable && e.operandSizeOverride) {
			e.movR32Imm32()
		} else {
			e.movR16Imm16()
		}
	case 0xE8:
		if (e.genuineProtectedEnable && !e.operandSizeOverride) || (!e.genuineProtectedEnable && e.operandSizeOverride) {
			e.callRel32()
		} else {
			e.callRel16()
		}
	case 0xE9:
		e.jmpRel32()
	case 0xEA:
		e.farJmp()
	case 0xEC:
		e.inAlDx()
	case 0xEE:
		e.outAlDx()
	case 0xEF:
		e.outAxDx()
	case 0xF3:
		// rep prefix
		ecx := e.getRegister32(ECX)
		fmt.Printf("repeat %d times. ", ecx)
		if ecx > 1 {
			e.eip++
			fmt.Printf("eip=0x%x code=0x%x\n", e.eip, e.getCode8(0))
			e.execInst()
			fmt.Printf("The exec of %d loop finished.\n", ecx)
			e.decRegister32(ECX, 1)
			e.eip -= 2
			fmt.Printf("Next eip=0x%x(0x%x) paddr of pdtentry=0x%x code=0x%x ecx=0x%x\n",
				e.eip, e.v2p(e.eip),
				(e.cr[3]>>22)+4*(e.eip>>22),
				e.getCode8(0), e.getRegister32(ECX))
		} else if ecx == 1 {
			e.eip++
			fmt.Printf("eip=0x%x code=0x%x\n", e.eip, e.getCode8(0))
			e.execInst()
			e.decRegister32(ECX, 1)
		} else {
			e.eip += 2
		}
	case 0xF4:
		e.halt()
	case 0xF6:
		e.testRm8Imm8()
	case 0xF7:
		e.codeF7()
	case 0xFA:
		e.cli()
	case 0xFC:
		e.cld()
	case 0xFF:
		e.codeFf()
	default:
		return errors.New(fmt.Sprintf("eip=%x opecode = %x is not implemented at execInst().", e.eip, e.getCode8(0)))
	}
	return nil
}

func (e *Emulator) nop() {
	e.eip++
}

func (e *Emulator) stosb() {
	address := e.getRegister32(EDI)
	value := e.getRegister8(AL)
	e.setMemory8(address, value)
	if e.eflags.isEnable(DirectionFlag) {
		e.decRegister32(EDI, 1)
	} else {
		e.incRegister32(EDI, 1)
	}
	e.eip++
}

func (e *Emulator) stosd() {
	address := e.getRegister32(EDI)
	value := e.getRegister32(EAX)
	fmt.Printf("stodsd address=0x%x(0x%x) value=0x%x\n", address, e.v2p(address), value)
	e.setMemory32(address, value)
	if e.eflags.isEnable(DirectionFlag) {
		e.decRegister32(EDI, 4)
	} else {
		e.incRegister32(EDI, 4)
	}
	e.eip++
}

func (e *Emulator) insd() {
	ioAddress := e.getRegister16(DX)
	value := e.io.in32(ioAddress)
	memAddress := e.getRegister32(EDI)
	// fmt.Printf("(insd) input 0x%08x from io[0x%x] to memory[paddr=0x%x vaddr=0x%x]\n",
	// 	value, ioAddress, memAddress, e.v2p(memAddress))
	e.setMemory32(memAddress, value)
	e.incRegister32(EDI, 4)
	e.eip++
}

func (e *Emulator) cli() {
	e.eflags.unset(InterruptFlag)
	e.eip++
}

func (e *Emulator) cld() {
	e.eflags.unset(DirectionFlag)
	e.eip++
}

// eip=801005f6 opecode = 0 is not implemented.
func (e *Emulator) codeF7() {
	testRm32Imm32 := func(e *Emulator, m ModRM) {
		value := e.getCode32(0)
		e.eip += 4
		result := e.getRm32(m) & value
		if result == 0 {
			e.eflags.set(ZeroFlag)
		} else {
			e.eflags.unset(ZeroFlag)
		}
		e.eflags.unset(CarryFlag)
		e.eflags.unset(OverflowFlag)
		e.eflags.updatePF(uint8(result & 0xFF))
	}
	negRm32 := func(e *Emulator, m ModRM) {
		value := e.getRm32(m)
		e.setRm32(m, (^value)+1)
	}
	divRm32 := func(e *Emulator, m ModRM) {
		// x/y = a ... b
		x := (uint64(e.getRegister32(EDX)) << 32) | uint64(e.getRegister32(EAX))
		y := uint64(e.getRm32(m))
		a := uint32(x / y)
		b := uint32(x % y)

		e.setRegister32(EAX, a)
		e.setRegister32(EDX, b)
	}

	e.eip++
	m := e.parseModRM()

	if e.genuineProtectedEnable == false && e.operandSizeOverride == false ||
		e.genuineProtectedEnable == true && e.operandSizeOverride == true {
		panic("16bit mode is not implemented")
	}

	switch m.opecode {
	case 0:
		testRm32Imm32(e, m)
	case 3:
		negRm32(e, m)
	case 6:
		divRm32(e, m)
	default:
		panic(fmt.Sprintf("eip=%x opecode = %d\n", e.eip, m.opecode) + "not implemented")
	}
}

func (e *Emulator) code0f() {
	lgdt := func() {
		m := e.parseModRM()
		address := uint32(e.calcMemoryAddress16(m))
		e.gdtrSize = e.getMemory16(address)
		e.gdtrBase = e.getMemory32(address + 2)
		fmt.Printf("address=0x%x gdtrSize=0x%x gdtrBase=0x%x\n",
			address, e.gdtrSize, e.gdtrBase)
		e.dumpGDTEntry(e.gdtrBase)
		e.dumpGDTEntry(e.gdtrBase + 0x8)
		e.dumpGDTEntry(e.gdtrBase + 0x10)
		e.dumpGDTEntry(e.gdtrBase + 0x18)
		e.dumpGDTEntry(e.gdtrBase + 0x20)
		e.dumpGDTEntry(e.gdtrBase + 0x28)
	}
	movR32Cr := func() {
		m := e.parseModRM()
		// e.setR32(m, e.cr[m.opecode])
		e.setRm32(m, e.cr[m.opecode])
	}
	movCrR32 := func() {
		m := e.parseModRM()
		// e.cr[m.opecode] = e.getR32(m)
		e.cr[m.opecode] = e.getRm32(m)
		// fmt.Printf("%d %d\n", m.opecode, e.cr[m.opecode])
		if m.opecode == 4 && e.cr[m.opecode]&CR4PageSizeExtension != 0 {
			fmt.Printf("CR4 page size sxtension Enabled (Page size is 4MB).\n")
		} else if m.opecode == 3 {
			fmt.Printf("CR3 Page Directory Table is at 0x%08x\n", e.cr[m.opecode]>>12)
			fmt.Printf("Page Directory Table[%d] = 0x%08x\n",
				0, e.getMemory32((e.cr[m.opecode]>>22)+4*0))
			fmt.Printf("Page Directory Table[%d] = 0x%08x\n",
				512, e.getMemory32((e.cr[m.opecode]>>22)+4*512))
		} else if m.opecode == 0 && e.cr[m.opecode]&CR0PagingFlag != 0 {
			fmt.Printf("CR0 paging is Enabled.\n")
		}
	}
	MovzxR32Rm8 := func() {
		m := e.parseModRM()
		// e.setRegister32(m.opecode, uint32(e.getMemory8(e.calcMemoryAddress32(m))))
		e.setRegister32(m.opecode, uint32(e.getRm8(m)))
	}
	MovzxR32Rm16 := func() {
		m := e.parseModRM()
		// fmt.Printf("m.disp32=0x%x\n", m.disp32)
		// e.setRegister32(m.opecode, uint32(e.getMemory16(m.disp32)))
		e.setRegister32(m.opecode, uint32(e.getRm16(m)))
	}
	MovsxR32Rm8 := func() {
		m := e.parseModRM()
		value := uint32(e.getRm8(m))
		if value&0x80 != 0 {
			value |= 0xFFFFFF00
		}
		e.setRegister32(m.opecode, value)
	}
	MovsxR32Rm16 := func() {
		m := e.parseModRM()
		// fmt.Printf("m.disp32=0x%x\n", m.disp32)
		value := uint32(e.getMemory16(m.disp32))
		if value&0x8000 != 0 {
			value |= 0xFFFF0000
		}
		e.setRegister32(m.opecode, value)
	}
	jne := func() {
		rel := e.getCode32(0)
		e.eip += 4
		if !e.eflags.isEnable(ZeroFlag) {
			e.eip += rel
		}
	}
	je := func() {
		rel := e.getCode32(0)
		e.eip += 4
		if e.eflags.isEnable(ZeroFlag) {
			e.eip += rel
		}
	}
	jg := func() {
		rel := e.getCode32(0)
		e.eip += 4
		if !e.eflags.isEnable(ZeroFlag) && e.eflags.isEnable(SignFlag) == e.eflags.isEnable(OverflowFlag) {
			e.eip += rel
		}
	}
	jae := func() {
		rel := e.getCode32(0)
		e.eip += 4
		if !e.eflags.isEnable(CarryFlag) {
			e.eip += rel
		}
	}
	ja := func() {
		rel := e.getCode32(0)
		e.eip += 4
		if !e.eflags.isEnable(ZeroFlag) && !e.eflags.isEnable(CarryFlag) {
			e.eip += rel
		}
	}

	second := e.getCode8(1)
	e.eip += 2
	if second == 0x01 {
		lgdt()
	} else if second == 0x20 {
		movR32Cr()
	} else if second == 0x22 {
		movCrR32()
	} else if second == 0x83 {
		jae()
	} else if second == 0x84 {
		je()
	} else if second == 0x85 {
		jne()
	} else if second == 0x87 {
		ja()
	} else if second == 0x8f {
		jg()
	} else if second == 0xB6 {
		MovzxR32Rm8()
	} else if second == 0xB7 {
		MovzxR32Rm16()
	} else if second == 0xBe {
		MovsxR32Rm8()
	} else if second == 0xBf {
		MovsxR32Rm16()
	} else {
		panic(fmt.Sprintf("EIP=0x%x 0x0F 0x%x is not implemented\n", e.eip-1, second))
	}
}

func (e *Emulator) intImm8() {
	value := e.getCode8(1)
	if value == 0x10 && e.getRegister16(AX) == 0x13 {
		// TODO: change video mode, 16color, 80x25
	} else if value == 0x10 && e.getRegister8(AH) == 0x0e {
		charCode := e.getRegister8(AL)
		fmt.Fprintf(e.writer, "%c", charCode)
	} else {
		panic(fmt.Sprintf("int not implemented"))
	}
	e.eip += 2
}

func (e *Emulator) outAlImm8() {
	address := uint16(e.getCode8(1))
	value := e.getRegister8(AL)
	e.io.out8(address, value)
	e.eip += 2
}

func (e *Emulator) inAlImm8() {
	address := uint16(e.getCode8(1))
	value := e.io.in8(address)
	e.setRegister8(AL, value)
	e.eip += 2
}

func (e *Emulator) inAxImm8() {
	panic(fmt.Sprintf("inAxImm8 not implemented"))
}

func (e *Emulator) inEaxImm8() {
	panic(fmt.Sprintf("inEaxImm8 not implemented"))
}

func (e *Emulator) movR16Imm16() {
	reg := e.getCode8(0) - 0xB8
	value := e.getCode16(1)
	e.setRegister16(reg, value)
	e.eip += 3
}

func (e *Emulator) movR32Imm32() {
	reg := e.getCode8(0) - 0xB8
	value := e.getCode32(1)
	e.registers[reg] = value
	e.eip += 5
}

func (e *Emulator) movEaxMoffs32() {
	value := e.getMemory32(e.getCode32(1))
	// fmt.Printf("value=0x%x\n", value)
	e.setRegister32(EAX, value)
	e.eip += 5
}

func (e *Emulator) movMoffs32Eax() {
	value := e.getRegister32(EAX)
	// fmt.Printf("value=0x%x\n", value)
	// e.setRegister32(EAX, value)
	e.setMemory32(e.getCode32(1), value)
	e.eip += 5
}

func (e *Emulator) movRm8Imm8() {
	e.eip++
	m := e.parseModRM()
	value := e.getCode8(0)
	e.eip += 1
	e.setRm8(m, value)
}

func (e *Emulator) movRm32Imm32() {
	e.eip++
	m := e.parseModRM()
	value := e.getCode32(0)
	e.eip += 4
	e.setRm32(m, value)
}

func (e *Emulator) orEaxImm32() {
	value := e.getCode32(1) | e.getRegister32(EAX)
	e.setRegister32(EAX, value)
	e.eip += 5
}

func (e *Emulator) addEaxImm32() {
	value := e.getCode32(1) + e.getRegister32(EAX)
	e.setRegister32(EAX, value)
	e.eip += 5
}

func (e *Emulator) subEaxImm32() {
	value := e.getCode32(1) - e.getRegister32(EAX)
	e.setRegister32(EAX, value)
	e.eip += 5
}

func (e *Emulator) code81() {
	addRm32Imm32 := func(e *Emulator, m ModRM) {
		rm32 := e.getRm32(m)
		imm32 := e.getCode32(0)
		e.eip += 4
		// fmt.Printf("rm32 value=0x%x imm32 value=0x%x\n", rm32, imm32)
		result := uint64(rm32) + uint64(imm32)
		e.setRm32(m, uint32(result))
	}
	andRm32Imm32 := func(e *Emulator, m ModRM) {
		rm32 := e.getRm32(m)
		imm32 := e.getCode32(0)
		e.eip += 4
		// fmt.Printf("rm32 value=0x%x imm32 value=0x%x\n", rm32, imm32)
		result := uint64(rm32) & uint64(imm32)
		e.setRm32(m, uint32(result))
	}
	orRm32Imm32 := func(e *Emulator, m ModRM) {
		rm32 := e.getRm32(m)
		imm32 := e.getCode32(0)
		e.eip += 4
		// fmt.Printf("rm32 value=0x%x imm32 value=0x%x\n", rm32, imm32)
		result := uint64(rm32) | uint64(imm32)
		e.setRm32(m, uint32(result))
	}
	cmpRm32Imm32 := func(e *Emulator, m ModRM) {
		rm32 := e.getRm32(m)
		imm32 := e.getCode32(0)
		// fmt.Printf("eip=%x rm32 value=0x%x imm32 value=0x%x\n", e.eip, rm32, imm32)
		e.eip += 4
		result := uint64(rm32) - uint64(imm32)
		e.eflags.updateBySub(rm32, imm32, result)
		// e.setRm32(m, uint32(result))
	}

	e.eip++
	m := e.parseModRM()

	if e.genuineProtectedEnable == false && e.operandSizeOverride == false ||
		e.genuineProtectedEnable == true && e.operandSizeOverride == true {
		panic("16bit mode is not implemented")
	}

	switch m.opecode {
	case 0:
		addRm32Imm32(e, m)
	case 1:
		orRm32Imm32(e, m)
	case 4:
		andRm32Imm32(e, m)
	case 7:
		cmpRm32Imm32(e, m)
	default:
		panic(fmt.Sprintf("EIP=0x%x code=81 opecode=%d ", e.eip, m.opecode) + "not implemented")
	}
}

func (e *Emulator) code80() {
	// cmpRm32Imm8 := func(e *Emulator, m ModRM) {
	// 	rm32 := e.getRm32(m)
	// 	imm8 := uint32(e.getSignCode8(0))
	// 	e.eip++
	// 	result := uint64(rm32) - uint64(imm8)
	// 	e.eflags.updateBySub(rm32, imm8, result)
	// }
	orRm8Imm8 := func(e *Emulator, m ModRM) {
		rm8 := e.getRm8(m)
		imm8 := e.getCode8(0)
		e.eip++
		e.setRm8(m, rm8|imm8)
		e.eflags.updateByAndOr8(rm8 | imm8)
	}
	andRm8Imm8 := func(e *Emulator, m ModRM) {
		rm8 := e.getRm8(m)
		imm8 := e.getCode8(0)
		e.eip++
		e.setRm8(m, rm8&imm8)
		e.eflags.updateByAndOr8(rm8 & imm8)
	}
	cmpRm8Imm8 := func(e *Emulator, m ModRM) {
		imm8 := e.getCode8(0)
		rm8 := e.getRm8(m)
		e.eip++
		result := uint16(rm8) - uint16(imm8)
		e.eflags.updateBySub8(rm8, imm8, result)
	}

	e.eip++
	m := e.parseModRM()

	if e.genuineProtectedEnable == false && e.operandSizeOverride == false ||
		e.genuineProtectedEnable == true && e.operandSizeOverride == true {
		panic("16bit mode is not implemented")
	}

	switch m.opecode {
	case 1:
		orRm8Imm8(e, m)
	case 4:
		andRm8Imm8(e, m)
	case 7:
		cmpRm8Imm8(e, m)
	default:
		panic(fmt.Sprintf("EIP=0x%x opecode = %d\n", e.eip, m.opecode) + "not implemented")
	}
}

func (e *Emulator) code83() {
	subRm32Imm8 := func(e *Emulator, m ModRM) {
		rm32 := e.getRm32(m)
		imm8 := uint32(e.getSignCode8(0))
		e.eip++
		result := uint64(rm32) - uint64(imm8)
		e.setRm32(m, uint32(result))
		e.eflags.updateBySub(rm32, imm8, result)
	}
	addRm32Imm8 := func(e *Emulator, m ModRM) {
		rm32 := e.getRm32(m)
		imm8 := uint32(e.getSignCode8(0))
		e.eip++
		e.setRm32(m, rm32+imm8)
	}
	andRm32Imm8 := func(e *Emulator, m ModRM) {
		rm32 := e.getRm32(m)
		imm8 := uint32(e.getSignCode8(0))
		e.eip++
		e.setRm32(m, rm32&uint32(imm8))
	}
	orRm32Imm8 := func(e *Emulator, m ModRM) {
		rm32 := e.getRm32(m)
		imm8 := uint32(e.getSignCode8(0))
		e.eip++
		e.setRm32(m, rm32|uint32(imm8))
	}
	cmpRm32Imm8 := func(e *Emulator, m ModRM) {
		rm32 := e.getRm32(m)
		imm8 := uint32(e.getSignCode8(0))
		e.eip++
		result := uint64(rm32) - uint64(imm8)
		e.eflags.updateBySub(rm32, imm8, result)
	}
	e.eip++
	m := e.parseModRM()

	if e.genuineProtectedEnable == false && e.operandSizeOverride == false ||
		e.genuineProtectedEnable == true && e.operandSizeOverride == true {
		panic("16bit mode is not implemented")
	}

	switch m.opecode {
	case 0:
		addRm32Imm8(e, m)
	case 1:
		orRm32Imm8(e, m)
	case 4:
		andRm32Imm8(e, m)
	case 5:
		subRm32Imm8(e, m)
	case 7:
		cmpRm32Imm8(e, m)
	default:
		panic(fmt.Sprintf("opecode = %d\n", m.opecode) + "not implemented")
	}
}

func (e *Emulator) codeC1() {
	e.eip++
	m := e.parseModRM()

	shrRm32Imm8 := func(e *Emulator, m ModRM) {
		rm32 := e.getRm32(m)
		imm8 := uint32(e.getCode8(0))
		e.eip++
		e.setRm32(m, rm32>>imm8)
		// TODO: change elfags
	}

	sarRm32Imm8 := func(e *Emulator, m ModRM) {
		rm32 := e.getRm32(m)
		sign := rm32 & 0x80000000
		imm8 := uint32(e.getCode8(0))
		e.eip++
		e.setRm32(m, ((rm32>>imm8)&0x7FFFFFFF)|sign)
		// TODO: change elfags
	}

	shlRm32Imm8 := func(e *Emulator, m ModRM) {
		rm32 := e.getRm32(m)
		imm8 := uint32(e.getCode8(0))
		e.eip++
		e.setRm32(m, rm32<<imm8)
		// TODO: change elfags
	}

	switch m.opecode {
	case 4:
		shlRm32Imm8(e, m)
	case 5:
		shrRm32Imm8(e, m)
	case 7:
		sarRm32Imm8(e, m)
	default:
		panic(fmt.Sprintf("EIP=0x%x opecode=%d ", e.eip-2, m.opecode) + "not implemented at codeC1")
	}
}

func (e *Emulator) codeFf() {
	incRm32 := func(e *Emulator, m ModRM) {
		rm32 := e.getRm32(m)
		e.setRm32(m, rm32+1)
	}
	decRm32 := func(e *Emulator, m ModRM) {
		rm32 := e.getRm32(m)
		e.setRm32(m, rm32-1)
	}
	pushRm32 := func(e *Emulator, m ModRM) {
		rm32 := e.getRm32(m)
		e.push32(rm32)
	}
	callRm32 := func(e *Emulator, m ModRM) {
		address := e.calcMemoryAddress32(m)
		jmpAddress := e.getMemory32(address)
		e.push32(e.eip + 6)
		// fmt.Printf("address=0x%x jmpAddress=0x%x\n", address, jmpAddress)
		e.eip = jmpAddress
	}
	jmpRm32 := func(e *Emulator, m ModRM) {
		address := e.getRm32(m)
		// address := e.calcMemoryAddress32(m)
		// fmt.Printf("jmpRm32 address=0x%x address2=0x%x\n", address, e.getMemory32(address))
		e.eip = address
	}
	e.eip++
	m := e.parseModRM()
	switch m.opecode {
	case 0:
		incRm32(e, m)
	case 1:
		decRm32(e, m)
	case 2:
		callRm32(e, m)
	// case 3:
	// 	callM16(e, m)
	case 4:
		jmpRm32(e, m)
	// case 5:
	// 	jmpM16(e, m)
	case 6:
		pushRm32(e, m)
	default:
		panic(fmt.Sprintf("opecode = %d\n", m.opecode) + "not implemented at codeFf")
	}
}

func (e *Emulator) movRm8R8() {
	e.eip++
	m := e.parseModRM()
	r8 := e.getR8(m)
	e.setRm8(m, r8)
}

func (e *Emulator) cmpRm8R8() {
	e.eip++
	m := e.parseModRM()
	r8 := e.getR8(m)
	rm8 := e.getRm8(m)
	result := uint16(rm8) - uint16(r8)
	e.eflags.updateBySub8(rm8, r8, result)
}

func (e *Emulator) xorRm16R16() {
	e.eip++
	m := e.parseModRM()
	e.setRm16(m, e.getRm16(m)^e.getR16(m))
}

func (e *Emulator) xorRm32R32() {
	e.eip++
	m := e.parseModRM()
	e.setRm32(m, e.getRm32(m)^e.getR32(m))
}

func (e *Emulator) movRm32R32() {
	e.eip++
	m := e.parseModRM()
	r32 := e.getR32(m)
	e.setRm32(m, r32)
}

func (e *Emulator) subRm32R32() {
	e.eip++
	m := e.parseModRM()
	rm32 := e.getRm32(m)
	r32 := e.getR32(m)
	// fmt.Printf("rm32=0x%x r32=0x%x\n", rm32, r32)
	e.setRm32(m, rm32-r32)
}

func (e *Emulator) orRm32R32() {
	e.eip++
	m := e.parseModRM()
	r32 := e.getR32(m)
	rm32 := e.getRm32(m)
	e.setRm32(m, r32|rm32)
}

func (e *Emulator) addRm32R32() {
	e.eip++
	m := e.parseModRM()
	r32 := e.getR32(m)
	rm32 := e.getRm32(m)
	e.setRm32(m, r32+rm32)
}

func (e *Emulator) orR32Rm32() {
	e.eip++
	m := e.parseModRM()
	r32 := e.getR32(m)
	rm32 := e.getRm32(m)
	e.setR32(m, r32|rm32)
}

func (e *Emulator) imulR32Rm32Imm32() {
	e.eip++
	m := e.parseModRM()
	rm32 := e.getRm32(m)
	imm32 := e.getCode32(0)
	e.eip += 4
	e.setR32(m, rm32*imm32)
	// e.eflags.updateByImul(rm32, imm32, rm32*imm32) // FIXME
}

func (e *Emulator) addR32Rm32() {
	e.eip++
	m := e.parseModRM()
	r32 := e.getR32(m)
	rm32 := e.getRm32(m)
	e.setR32(m, r32+rm32)
}

func (e *Emulator) leaR32Rm32() {
	e.eip++
	m := e.parseModRM()
	// fmt.Printf("leaR32Rm32 r=%d\n", m.opecode)
	e.setR32(m, e.calcMemoryAddress32(m))
}

func (e *Emulator) movR32Rm32() {
	e.eip++
	m := e.parseModRM()
	rm32 := e.getRm32(m)
	e.setR32(m, rm32)
}

// 16 bit mode
func (e *Emulator) movSregRm16() {
	e.eip++
	m := e.parseModRM()
	rm16 := e.getRm16(m)
	// fmt.Printf("m.opecode=%d\n", m.opecode)
	e.setSreg16(m.opecode, rm16)
}

func (e *Emulator) movR8Rm8() {
	e.eip++
	m := e.parseModRM()
	rm8 := e.getRm8(m)
	e.setR8(m, rm8)
}

func (e *Emulator) movR8Imm8() {
	reg := e.getCode8(0) - 0xB0
	e.setRegister8(reg, e.getCode8(1))
	e.eip += 2
}

func (e *Emulator) cmpR32Rm32() {
	e.eip++
	m := e.parseModRM()
	r32 := e.getR32(m)
	rm32 := e.getRm32(m)
	result := uint64(r32) - uint64(rm32)
	e.eflags.updateBySub(r32, rm32, result)
}

func (e *Emulator) cmpRm32R32() {
	e.eip++
	m := e.parseModRM()
	r32 := e.getR32(m)
	rm32 := e.getRm32(m)
	result := uint64(rm32) - uint64(r32)
	e.eflags.updateBySub(rm32, r32, result)
}

func (e *Emulator) cmpEaxImm32() {
	ax := e.getRegister32(EAX)
	value := e.getCode32(1)
	result := uint64(ax) - uint64(value)
	e.eip += 5
	e.eflags.updateBySub(ax, value, result)
}

func (e *Emulator) testEaxImm32() {
	ax := e.getRegister32(EAX)
	value := e.getCode32(1)
	result := ax & value
	if result == 0 {
		e.eflags.set(ZeroFlag)
	} else {
		e.eflags.unset(ZeroFlag)
	}
	e.eflags.unset(CarryFlag)
	e.eflags.unset(OverflowFlag)
	e.eflags.updatePF(uint8(result & 0xFF))
	e.eflags.setVal(SignFlag, result&0x80000000 != 0)
	e.eip += 5
}

func (e *Emulator) testRm32R32() {
	e.eip++
	m := e.parseModRM()
	result := e.getRm32(m) & e.getR32(m)
	if result == 0 {
		e.eflags.set(ZeroFlag)
	} else {
		e.eflags.unset(ZeroFlag)
	}
	e.eflags.unset(CarryFlag)
	e.eflags.unset(OverflowFlag)
	e.eflags.updatePF(uint8(result & 0xFF))
	e.eflags.setVal(SignFlag, result&0x80000000 != 0)
}

func (e *Emulator) testRm8R8() {
	e.eip++
	m := e.parseModRM()
	result := e.getRm8(m) & e.getR8(m)
	if result == 0 {
		e.eflags.set(ZeroFlag)
	} else {
		e.eflags.unset(ZeroFlag)
	}
	e.eflags.unset(CarryFlag)
	e.eflags.unset(OverflowFlag)
	e.eflags.updatePF(uint8(result & 0xFF))
	e.eflags.setVal(SignFlag, result&0x80 != 0)
}

func (e *Emulator) testRm8Imm8() {
	e.eip++
	m := e.parseModRM()
	rm8 := e.getRm8(m)
	value := e.getCode8(0)
	e.eip++

	result := uint32(rm8 & value)
	if result == 0 {
		e.eflags.set(ZeroFlag)
	} else {
		e.eflags.unset(ZeroFlag)
	}
	e.eflags.unset(CarryFlag)
	e.eflags.unset(OverflowFlag)
	e.eflags.updatePF(uint8(result & 0xFF))
	e.eflags.setVal(SignFlag, result&0x80 != 0)
}

// e.setRegister32(m.opecode, uint32(e.getMemory8(m.disp32)))
func (e *Emulator) testAxImm16() {
	ax := uint32(e.getRegister16(AX))
	value := uint32(e.getCode16(1))
	result := ax & value
	if result == 0 {
		e.eflags.set(ZeroFlag)
	} else {
		e.eflags.unset(ZeroFlag)
	}
	e.eflags.unset(CarryFlag)
	e.eflags.unset(OverflowFlag)
	e.eflags.updatePF(uint8(result & 0xFF))
	e.eflags.setVal(SignFlag, result&0x8000 != 0)
	e.eip += 3
}

func (e *Emulator) andEaxImm32() {
	ax := e.getRegister32(EAX)
	value := e.getCode32(1)
	result := ax & value
	if result == 0 {
		e.eflags.set(ZeroFlag)
	} else {
		e.eflags.unset(ZeroFlag)
	}
	e.eflags.unset(CarryFlag)
	e.eflags.unset(OverflowFlag)
	e.eflags.updatePF(uint8(result & 0xFF))
	e.setRegister32(EAX, result)
	e.eip += 5
}

func (e *Emulator) andAxImm16() {
	ax := uint32(e.getRegister16(AX))
	value := uint32(e.getCode16(1))
	result := ax & value
	if result == 0 {
		e.eflags.set(ZeroFlag)
	} else {
		e.eflags.unset(ZeroFlag)
	}
	e.eflags.unset(CarryFlag)
	e.eflags.unset(OverflowFlag)
	e.eflags.updatePF(uint8(result & 0xFF))
	e.setRegister16(AX, uint16(result))
	e.eip += 3
}

func (e *Emulator) testAlImm8() {
	al := uint32(e.getRegister8(AL))
	value := uint32(e.getCode8(1))
	result := al & value
	if result == 0 {
		e.eflags.set(ZeroFlag)
	} else {
		e.eflags.unset(ZeroFlag)
	}
	e.eflags.unset(CarryFlag)
	e.eflags.unset(OverflowFlag)
	e.eflags.updatePF(uint8(result & 0xFF))
	e.eflags.setVal(SignFlag, result&0x80 != 0)
	e.eip += 2
}

func (e *Emulator) cmpAlImm8() {
	al := uint32(e.getRegister8(AL))
	value := uint32(e.getCode8(1))
	result := uint64(al) - uint64(value)
	e.eflags.updateBySub(al, value, result)
	e.eip += 2
}

func (e *Emulator) shortJmp() {
	diff := int32(e.getSignCode8(1))
	if diff < 0 {
		e.eip = e.eip - uint32(-diff) + uint32(2)
	} else {
		e.eip = e.eip + uint32(diff) + uint32(2)
	}
}

func (e *Emulator) farJmp() {
	offset := e.getCode16(1)
	segmentIndex := e.getCode16(3)
	e.setSreg16(CS, segmentIndex)
	e.eip = uint32(offset)
}

func (e *Emulator) jmpRel32() {
	diff := e.getSignCode32(1)
	if diff < 0 {
		e.eip = e.eip - uint32(-diff) + uint32(5)
	} else {
		e.eip = e.eip + uint32(diff) + uint32(5)
	}
}

func (e *Emulator) pushR32() {
	reg := e.getCode8(0) - 0x50
	e.push32(e.getRegister32(reg))
	e.eip++
}

func (e *Emulator) pushf() {
	e.eip++
	e.push32(uint32(e.eflags))
}

func (e *Emulator) incR32() {
	reg := e.getCode8(0) - 0x40
	e.setRegister32(reg, e.getRegister32(reg)+1)
	e.eip++
}

func (e *Emulator) decR32() {
	reg := e.getCode8(0) - 0x48
	e.setRegister32(reg, e.getRegister32(reg)-1)
	e.eip++
}

func (e *Emulator) popR32() {
	reg := e.getCode8(0) - 0x58
	e.setRegister32(reg, e.pop32())
	e.eip++
}

func (e *Emulator) pushImm8() {
	value := uint32(e.getCode8(1))
	e.push32(value)
	e.eip += 2
}

func (e *Emulator) pushImm16() {
	value := uint32(e.getCode16(1))
	e.push32(value)
	e.eip += 3
}

func (e *Emulator) pushImm32() {
	value := e.getCode32(1)
	e.push32(value)
	e.eip += 5
}

func (e *Emulator) callRel16() {
	diff := e.getSingedCode16(1)
	e.push32(e.eip + 3)
	if diff < 0 {
		e.eip = e.eip - uint32(-diff) + uint32(3)
	} else {
		e.eip = e.eip + uint32(diff) + uint32(3)
	}
}

func (e *Emulator) callRel32() {
	diff := e.getSingedCode32(1)
	e.push32(e.eip + 5)
	if diff < 0 {
		e.eip = e.eip - uint32(-diff) + uint32(5)
	} else {
		e.eip = e.eip + uint32(diff) + uint32(5)
	}
}

func (e *Emulator) ret() {
	e.eip = e.pop32()
}

func (e *Emulator) jnz() {
	if e.eflags.isEnable(ZeroFlag) {
		e.eip += uint32(2)
	} else {
		e.eip += uint32(2) + uint32(e.getSignCode8(1))
	}
}

func (e *Emulator) ja() {
	if e.eflags.isEnable(CarryFlag) || e.eflags.isEnable(ZeroFlag) {
		e.eip += uint32(2)
	} else {
		e.eip += uint32(2) + uint32(e.getSignCode8(1))
	}
}

func (e *Emulator) jb() {
	if e.eflags.isEnable(CarryFlag) {
		e.eip += uint32(2) + uint32(e.getSignCode8(1))
	} else {
		e.eip += uint32(2)
	}
}

func (e *Emulator) jp() {
	if e.eflags.isEnable(ParityFlag) {
		e.eip += uint32(2) + uint32(e.getSignCode8(1))
	} else {
		e.eip += uint32(2)
	}
}

func (e *Emulator) jpo() {
	if e.eflags.isEnable(ParityFlag) {
		e.eip += uint32(2)
	} else {
		e.eip += uint32(2) + uint32(e.getSignCode8(1))
	}
}

func (e *Emulator) jc() {
	if e.eflags.isEnable(CarryFlag) {
		e.eip += uint32(2) + uint32(e.getSignCode8(1))
	} else {
		e.eip += uint32(2)
	}
}

func (e *Emulator) jae() {
	if e.eflags.isEnable(CarryFlag) {
		e.eip += uint32(2)
	} else {
		e.eip += uint32(2) + uint32(e.getSignCode8(1))
	}
}

func (e *Emulator) jno() {
	if e.eflags.isEnable(OverflowFlag) {
		e.eip += uint32(2)
	} else {
		e.eip += uint32(2) + uint32(e.getSignCode8(1))
	}
}

func (e *Emulator) jna() {
	if e.eflags.isEnable(CarryFlag) || e.eflags.isEnable(ZeroFlag) {
		e.eip += uint32(2) + uint32(e.getSignCode8(1))
	} else {
		e.eip += uint32(2)
	}
}

func (e *Emulator) jz() {
	if e.eflags.isEnable(ZeroFlag) {
		e.eip += uint32(2) + uint32(e.getSignCode8(1))
	} else {
		e.eip += uint32(2)
	}
}

func (e *Emulator) js() {
	if e.eflags.isEnable(SignFlag) {
		e.eip += uint32(2) + uint32(e.getSignCode8(1))
	} else {
		e.eip += uint32(2)
	}
}

func (e *Emulator) jns() {
	if e.eflags.isEnable(SignFlag) {
		e.eip += uint32(2)
	} else {
		e.eip += uint32(2) + uint32(e.getSignCode8(1))
	}
}

func (e *Emulator) jg() {
	if !e.eflags.isEnable(ZeroFlag) && e.eflags.isEnable(SignFlag) == e.eflags.isEnable(OverflowFlag) {
		e.eip += uint32(2) + uint32(e.getSignCode8(1))
	} else {
		e.eip += uint32(2)
	}
}

func (e *Emulator) jng() {
	if e.eflags.isEnable(ZeroFlag) || e.eflags.isEnable(SignFlag) != e.eflags.isEnable(OverflowFlag) {
		e.eip += uint32(2) + uint32(e.getSignCode8(1))
	} else {
		e.eip += uint32(2)
	}
}

func (e *Emulator) jl() {
	if e.eflags.isEnable(SignFlag) != e.eflags.isEnable(OverflowFlag) {
		e.eip += uint32(e.getSignCode8(1))
	} else {
		e.eip += 2
	}
}

func (e *Emulator) jle() {
	if e.eflags.isEnable(ZeroFlag) || e.eflags.isEnable(SignFlag) != e.eflags.isEnable(OverflowFlag) {
		e.eip += uint32(e.getSignCode8(1))
	} else {
		e.eip += 2
	}
}

func (e *Emulator) inAlDx() {
	address := e.getRegister16(DX)
	value := e.io.in8(address)
	e.setRegister8(AL, value)
	e.eip++
}

func (e *Emulator) outAlDx() {
	address := e.getRegister16(DX)
	value := e.getRegister8(AL)
	e.io.out8(address, value)
	e.eip++
}

func (e *Emulator) outAxDx() {
	address := e.getRegister16(DX)
	value := e.getRegister16(AX)
	e.io.out16(address, value)
	e.eip++
}

// util

// dump GDT entry
func (e *Emulator) dumpGDTEntry(physAddr uint32) {
	entry := e.getMemory64(physAddr)
	segmentBaseAddr := uint32((((entry >> 56) & 0xFF) << 24) | (((entry >> 32) & 0xFF) << 16) | ((entry >> 16) & 0xFFFF))
	segmentLimit := uint32((((entry >> 48) & 0xF) << 16) | (entry & 0xFFFF))
	isCodeSegment := (entry >> 43) & 1
	fmt.Printf("GDTEntry[%d]={entryPhysAddr=0x%x segmentBaseAddr=0x%x segmentLimit=0x%x isCodeSegment=0x%x}\n",
		(physAddr-e.gdtrBase)/8, physAddr, segmentBaseAddr, segmentLimit, isCodeSegment)
}

func (e *Emulator) setRm32(m ModRM, value uint32) {
	if m.mod == 3 {
		e.setRegister32(m.rm, value)
	} else {
		address := e.calcMemoryAddress32(m)
		e.setMemory32(address, value)
	}
}

func (e *Emulator) getRm32(m ModRM) uint32 {
	if m.mod == 3 {
		return e.getRegister32(m.rm)
	}
	address := e.calcMemoryAddress32(m)
	// fmt.Printf("rm32 address=0x%x\n", address)
	return e.getMemory32(address)
}

func (e *Emulator) setRm16(m ModRM, value uint16) {
	if m.mod == 3 {
		e.setRegister16(m.rm, value)
	} else {
		address := uint32(e.calcMemoryAddress16(m))
		e.setMemory16(address, value)
	}
}

func (e *Emulator) getRm16(m ModRM) uint16 {
	if m.mod == 3 {
		return e.getRegister16(m.rm) // TODO check OK?
	}
	address := e.calcMemoryAddress32(m)
	return e.getMemory16(address)
}

func (e *Emulator) getRm8(m ModRM) uint8 {
	if m.mod == 3 {
		return e.getRegister8(m.rm) // TODO check OK?
	}
	address := e.calcMemoryAddress32(m)
	if e.cr[0]&1 == 0 {
		address = uint32(e.calcMemoryAddress16(m))
	}
	return e.getMemory8(address)
}

func (e *Emulator) getR32(m ModRM) uint32 {
	return e.getRegister32(m.opecode)
}

func (e *Emulator) getR16(m ModRM) uint16 {
	return e.getRegister16(m.opecode)
}

func (e *Emulator) getR8(m ModRM) uint8 {
	return e.getRegister8(m.opecode) // TOOD: Is index correct for 8bit register?
}

func (e *Emulator) setR32(m ModRM, value uint32) {
	e.setRegister32(m.opecode, value)
}

func (e *Emulator) setSreg16(index uint8, value uint16) {
	e.sreg[index] = uint32(value)
	if e.cr[0]&1 == 0x1 {
		e.genuineProtectedEnable = true
	} else {
		e.genuineProtectedEnable = false
	}
}

func (e *Emulator) setR8(m ModRM, value uint8) {
	e.setRegister8(m.opecode, value)
}

func (e *Emulator) setRm8(m ModRM, value uint8) {
	if m.mod == 3 {
		e.setRegister8(m.rm, value)
	} else {
		address := e.calcMemoryAddress32(m)
		e.setMemory8(address, value)
	}
}

func (e *Emulator) calcMemoryAddress16(m ModRM) uint16 {
	if m.mod == 0 {
		// [register + resiger]
		switch m.rm {
		case 0:
			return e.getRegister16(BX) + e.getRegister16(SI)
		case 1:
			return e.getRegister16(BX) + e.getRegister16(DI)
		case 2:
			return e.getRegister16(BP) + e.getRegister16(SI)
		case 3:
			return e.getRegister16(BP) + e.getRegister16(DI)
		case 4:
			return e.getRegister16(SI)
		case 5:
			return e.getRegister16(DI)
		case 6:
			return uint16(m.getDisp16())
		case 7:
			return e.getRegister16(BX)
		}
	} else if m.mod == 1 {
		// [register + disp8]
		if m.rm == 6 {
			return uint16(int32(e.getRegister16(BP)) + int32(m.getDisp8()))
		}
		m.mod = 0
		return uint16(int32(e.calcMemoryAddress16(m)) + int32(m.getDisp8()))
	} else if m.mod == 2 {
		// [redister + disp16]
		if m.rm == 6 {
			return uint16(int32(e.getRegister16(BP)) + int32(m.getDisp16()))
		}
		m.mod = 0
		return uint16(int32(e.calcMemoryAddress16(m)) + int32(m.getDisp16()))
	}
	// register
	panic("ModRM mod = 4 is not implemented")
}

func (e *Emulator) calcMemoryAddress32(m ModRM) uint32 {
	if m.mod == 0 {
		// [register + resiger]
		if m.rm == 5 {
			return m.disp32 // Is this a EBP?
		} else if m.rm == 4 {
			return m.getSib(e)
		}

		return e.getRegister32(m.rm)
	} else if m.mod == 1 {
		// [register + disp8]
		var result uint32
		if m.rm == 4 {
			result = m.getSib(e)
		} else {
			result = e.getRegister32(m.rm)
		}

		disp8 := m.getDisp8()
		if disp8 < 0 {
			// return e.getRegister32(m.rm) + m.getSib(e) - uint32(-disp8)
			result -= uint32(-disp8)
		} else {
			result += uint32(disp8)
		}

		// fmt.Printf("calcMemoryAddress32 = 0x%x 0x%x 0x%x\n",
		// 	e.getRegister32(m.rm) , m.getSib(e) , uint32(disp8))
		return result
	} else if m.mod == 2 {
		// [redister + disp16/32]
		var result uint32
		if m.rm == 4 {
			result = m.getSib(e)
		} else {
			result = e.getRegister32(m.rm)
		}
		result += m.disp32
		return result
	}
	// register
	panic("ModRM mod = 4 is not implemented")
}

func (e *Emulator) setRegister32(rm uint8, value uint32) {
	e.registers[rm] = value
}

func (e *Emulator) setRegister16(rm uint8, value uint16) {
	e.registers[rm] = (e.registers[rm] & 0xFFFF0000) | uint32(value)
}

func (e *Emulator) getRegister32(rm uint8) uint32 {
	return e.registers[rm]
}

func (e *Emulator) getRegister16(rm uint8) uint16 {
	return uint16(e.registers[rm] & 0xffff)
}

func (e *Emulator) getRegister8(rm uint8) uint8 {
	if rm < 4 {
		return uint8(e.registers[rm] & 0xff)
	}
	return uint8((e.registers[rm-4] >> 8) & 0xff)
}

func (e *Emulator) setRegister8(rm, value uint8) {
	if rm < 4 {
		e.registers[rm] = (e.registers[rm] & 0xffffff00) | uint32(value)
	} else {
		e.registers[rm-4] = (e.registers[rm-4] & 0xffff00ff) | (uint32(value) << 8)
	}
}

func (e *Emulator) incRegister32(rm uint8, value uint32) {
	e.registers[rm] += value
}

func (e *Emulator) decRegister32(rm uint8, value uint32) {
	e.registers[rm] -= value
}

func (e *Emulator) v2p(vaddress uint32) uint32 {
	var paddress uint32
	if (e.cr[0]&CR0PagingFlag != 0) && (e.cr[4]&CR4PageSizeExtension != 0) {
		// paing
		var pdtEntry uint32
		for i := uint32(0); i < 4; i++ {
			pdtEntry |= uint32(e.memory[(e.cr[3]>>22)+4*(vaddress>>22)+i]) << uint32(i*8)
		}
		// fmt.Printf("pdtEntry=0x%x index=%d offset=0x%x\n", pdtEntry, vaddress>>22, vaddress&0x3FFFFF)
		paddress = pdtEntry + vaddress&0x3FFFFF
	} else {
		paddress = vaddress
	}
	return paddress
}

// Memory mapped I/O
// TODO: Use paddr
const (
	ID    = 0x0020
	SVR   = 0x00F0
	ICRLO = 0x0300
	TIMER = 0x0320
	TICR  = 0x0380
	TDCR  = 0x03E0
)

var (
	ioapicData uint32
)

func (e *Emulator) setMemory8(address uint32, value uint8) {
	paddr := e.v2p(address)
	e.memory[paddr] = value

	if address == LocalAPICBase+SVR+1 && value&0x01 == 0x01 {
		fmt.Printf("Local APIC Enabled vaddr=0x%x paddr=0x%x\n", address, paddr)
	} else if address == LocalAPICBase+TIMER+2 && value&0x02 == 0x02 {
		fmt.Printf("Timer PERIODIC Enabled vaddr=0x%x paddr=0x%x\n", address, paddr)
	}
}

func (e *Emulator) setMemory16(address uint32, value uint16) {
	for i := uint32(0); i < 2; i++ {
		e.setMemory8(address+i, uint8(value>>uint32(i*8)&0xFF))
	}
}

func (e *Emulator) setMemory32(address, value uint32) {
	if address == IOAPICBase && value == 0x00 {
		ioapicData = 1 << 24
		fmt.Printf("ioapic read is called. I have to return ioapicid\n")
	}
	for i := uint32(0); i < 4; i++ {
		e.setMemory8(address+i, uint8(value>>uint32(i*8)&0xFF))
	}
}

// TODO: consider linear address transformation using DS
func (e *Emulator) getMemory8(address uint32) uint8 {
	// fmt.Printf("vaddr=%x paddr=%x\n", address, e.v2p(address))
	paddr := e.v2p(address)
	if LocalAPICBase+ICRLO <= address && address <= LocalAPICBase+ICRLO+3 {
		fmt.Printf("synched ICRLO = 0\n")
		e.memory[paddr] = 0
	} else if LocalAPICBase+ID+3 == address {
		e.memory[paddr] = 1
	}
	return e.memory[paddr]
}

func (e *Emulator) getMemory16(address uint32) uint16 {
	var ret uint16
	for i := uint32(0); i < 2; i++ {
		ret |= uint16(e.getMemory8(address+i)) << uint32(i*8)
	}
	return ret
}

func (e *Emulator) getMemory32(address uint32) uint32 {
	if address == IOAPICBase+4*4 {
		fmt.Printf("Return 0x%x as ioapic data\n", ioapicData)
		return ioapicData
	}

	var ret uint32
	for i := uint32(0); i < 4; i++ {
		ret |= uint32(e.getMemory8(address+i)) << uint32(i*8)
	}
	return ret
}

func (e *Emulator) getMemory64(address uint32) uint64 {
	var ret uint64
	for i := uint32(0); i < 8; i++ {
		ret |= uint64(e.getMemory8(address+i)) << uint32(i*8)
	}
	return ret
}

func (e *Emulator) push32(value uint32) {
	address := e.getRegister32(ESP) - 4
	e.setMemory32(address, value)
	e.setRegister32(ESP, address)
}

func (e *Emulator) pop32() uint32 {
	value := e.getMemory32(e.getRegister32(ESP))
	e.incRegister32(ESP, 4)
	return value
}

func (e *Emulator) halt() {
	fmt.Fprintf(e.writer, "The system has halted.\n")
	e.eip = 0x7c00
}

func (e *Emulator) leave() {
	ebp := e.getRegister32(EBP)
	e.setRegister32(ESP, ebp)
	e.setRegister32(EBP, e.pop32())
	e.eip++
}

func (e *Emulator) dump(index int) {
	color.New(color.FgBlack).Printf("" +
		fmt.Sprintf("%10d", index) +
		"---------------------" +
		"-----------------------------\n")
	color.New(color.FgCyan).Printf(""+
		"EAX=0x%08x "+
		"ECX=0x%08x "+
		"EDX=0x%08x "+
		"EBX=0x%08x "+
		"ESP=0x%08x\n"+
		"ESI=0x%08x "+
		"EDI=0x%08x "+
		"EBP=0x%08x "+
		"EIP=0x%08x[paddr=0x%08x] ",
		e.registers[EAX],
		e.registers[ECX],
		e.registers[EDX],
		e.registers[EBX],
		e.registers[ESP],
		e.registers[ESI],
		e.registers[EDI],
		e.registers[EBP],
		e.eip, e.v2p(e.eip),
	)
	color.New(color.FgGreen).Printf("(opecode=%02x, %s)\n",
		e.getCode8(0), e.disasm[uint64(e.eip)])
	color.New(color.FgCyan).Printf(""+
		"CR0=0x%08x "+
		"CR1=0x%08x "+
		"CR2=0x%08x "+
		"CR3=0x%08x "+
		"CR4=0x%08x\n",
		e.cr[0],
		e.cr[1],
		e.cr[2],
		e.cr[3],
		e.cr[4],
	)
	e.eflags.dump()
}

// get from eip

// TODO: consider linear address transformation using CS
func (e *Emulator) getCode8(index int32) uint8 {
	var addr uint32
	if index < 0 {
		addr = e.eip - uint32(-index)
	} else {
		addr = e.eip + uint32(index)
	}
	return e.memory[e.v2p(addr)]
}

func (e *Emulator) getSignCode8(index int32) int8 {
	return int8(e.getCode8(index))
}

func (e *Emulator) getSignCode16(index int32) int16 {
	return int16(e.getCode16(index))
}

func (e *Emulator) getSignCode32(index int32) int32 {
	return int32(e.getCode32(index))
}

func (e *Emulator) getCode16(index int32) uint16 {
	var ret uint16
	for i := int32(0); i < 4; i++ {
		ret |= uint16(e.getCode8(index+i)) << uint32(i*8)
	}
	return ret
}

func (e *Emulator) getCode32(index int32) uint32 {
	var ret uint32
	for i := int32(0); i < 4; i++ {
		ret |= uint32(e.getCode8(index+i)) << uint32(i*8)
	}
	return ret
}

func (e *Emulator) getSingedCode32(index int32) int32 {
	return int32(e.getCode32(index))
}

func (e *Emulator) getSingedCode16(index int32) int16 {
	return int16(e.getCode16(index))
}

// ModRM parameter
type ModRM struct {
	mod     uint8
	opecode uint8 // This can be regarded as regIndex.
	rm      uint8
	disp32  uint32 // This can be regarded as (disp8, signed int8, disp16 signed int16).
	sib     uint8  // sib byte
	// disp32Sib uint32 // disp32 for sib
}

func (m *ModRM) getSib(e *Emulator) uint32 { // Indicate [--][--]
	if m.mod < 3 && m.rm == 4 {
		base := uint8(m.sib & 0x7)
		index := uint8((m.sib >> 3) & 0x7)
		scale := uint8((m.sib >> 6) & 0x3)

		// calc base value
		var result uint32
		if base == 5 && m.mod == 0 {
			result = m.disp32
		} else {
			result = e.getRegister32(base)
		}
		// if m.mod == 2 {
		// 	result = e.getRegister32(base)
		// } else if m.mod == 1 {
		// 	result = e.getRegister32(base)
		// } else if base == 5 {
		// 	result = m.disp32
		// } else {
		// 	result = e.getRegister32(base)
		// }

		// index
		if index != 4 {
			result += e.getRegister32(index) * uint32(1<<scale)
		}

		// fmt.Printf("sib=0x%x base=0x%x index=0x%x scale=0x%x value=0x%x\n", m.sib, base, index, scale, result)
		return result
	}
	return uint32(0)
}

func (m *ModRM) getDisp8() int8 {
	return int8(m.disp32 & 0xff)
}

func (m *ModRM) getDisp16() int16 {
	return int16(m.disp32 & 0xffff)
}

func (m *ModRM) setDisp8(disp8 int8) {
	m.disp32 = (m.disp32 & 0xFFFFFF00) | uint32(disp8)
}

func (m *ModRM) setDisp16(disp16 int16) {
	m.disp32 = (m.disp32 & 0xFFFF0000) | uint32(disp16)
}

// load ModR/M & increment eip
func (e *Emulator) parseModRM() ModRM {
	code := e.getCode8(0)
	// fmt.Printf("modrm=0x%x\n", code)

	// 76  543                210
	// mod regIndex(opecode) r/m
	m := ModRM{
		mod:     (code >> 6) & 0x03,
		opecode: (code >> 3) & 0x07,
		rm:      code & 0x07,
	}

	e.eip++
	// fmt.Printf("get mod=0x%x opecode=0x%x rm=0x%x\n", m.mod, m.opecode, m.rm)

	if e.cr[0]&1 == 0 {
		// 16 bit mode
		if m.mod == 1 {
			m.setDisp8(e.getSignCode8(0))
			e.eip++
		} else if m.rm == 6 || m.mod == 2 {
			m.setDisp16(e.getSignCode16(0))
			e.eip += 2
			// fmt.Printf("set disp16 eip=0x%x\n", e.eip)
		}
	} else {
		// 32 bit mode
		if m.mod != 3 && m.rm == 4 {
			m.sib = e.getCode8(0)
			e.eip++
			// if m.mod == 2 || (m.mod == 0 && (m.sib&0x7 == 5)) {
			// 	m.disp32Sib = e.getCode32(0)
			// 	e.eip += 4
			// } else if m.mod == 1 {
			// 	m.disp32Sib = uint32(e.getCode8(0))
			// 	e.eip++
			// }
			// fmt.Printf("get sib=0x%x disp32Sib=0x%x\n", m.sib, m.disp32Sib)
		}

		if (m.mod == 0 && m.rm == 5) || m.mod == 2 || (m.mod == 0 && m.rm == 4 && m.sib&0x7 == 0x5) || (m.mod == 2 && m.rm == 4 && m.sib&0x7 == 0x5) {
			// The last two condition are as below:
			// [scaled index] + disp32
			// [scaled index] + disp32 + [EBP]
			// fmt.Printf("get disp32 from eip=0x%x\n", e.eip)
			m.disp32 = e.getCode32(0)
			e.eip += 4
		} else if m.mod == 1 || (m.mod == 1 && m.rm == 4 && m.sib&0x7 == 0x5) {
			m.setDisp8(e.getSignCode8(0))
			e.eip++
		}
	}

	return m
}
