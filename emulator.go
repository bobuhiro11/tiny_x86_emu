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
	ES=0
	CS=1
	SS=2
	DS=3
	GS=5
)

// Emulator is an i386 Virtual Machine
type Emulator struct {
	registers   [8]uint32  // general registers
	cr          [16]uint32 // controll registers
	sreg        [6]uint32  // segment registers
	eflags      Eflags     // eflags
	gdtrSize	uint16     // global table descriptor table's size
	gdtrBase	uint32     // global table descriptor table's base phys address
	memory      map[uint32]uint8    // physical memory
	eip         uint32     // program counter
	isSilent    bool       // silent mode
	reader      io.Reader
	writer      io.Writer
	io			IO
	operandSizeOverride bool  // true if operand size override (0x66) is enabled
	genuineProtectedEnable bool // procted mode is refreshed only when sreg is changed
	disasm      map[uint64]string // disasmed code (ex. 32255 -> "0000 add [bx+si],al")
}

// NewEmulator creates New Emulator
func NewEmulator(memorySize, eip, esp uint32, protectedMode, isSilent bool, reader io.Reader, writer io.Writer, disasm map[uint64]string) *Emulator {
		e := &Emulator{
		// memory:      make([]uint8, memorySize * 2),
		memory:      map[uint32]uint8{},
		eip:         eip,
		isSilent:    isSilent,
		reader:      reader,
		writer:      writer,
		disasm:      disasm,
	}
	e.registers[ESP] = esp
	e.io = NewIO(&reader, &writer)
	if protectedMode{
		e.cr[0] |= 1
		e.genuineProtectedEnable = true
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
	case 0x0D:
		e.orEaxImm32()
	case 0x0F:
		e.code0f()
	case 0x25:
		if (e.genuineProtectedEnable && !e.operandSizeOverride) || (! e.genuineProtectedEnable && e.operandSizeOverride) {
			e.andEaxImm32()
		} else {
			e.andAxImm16()
		}
	case 0x29:
		e.subRm32R32()
	case 0x31:
		if (e.genuineProtectedEnable && !e.operandSizeOverride) || (! e.genuineProtectedEnable && e.operandSizeOverride) {
			e.xorRm32R32()
		} else {
			e.xorRm16R16()
		}
	case 0x39:
		e.cmpRm32R32()
	case 0x3b:
		e.cmpR32Rm32()
	case 0x3c:
		e.cmpAlImm8()
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
		if (e.genuineProtectedEnable && !e.operandSizeOverride) || (! e.genuineProtectedEnable && e.operandSizeOverride) {
			e.pushImm32()
		} else {
			e.pushImm16()
		}
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
	case 0x81:
		e.code81()
	case 0x83:
		e.code83()
	case 0x89:
		e.movRm32R32()
	case 0x8A:
		e.movR8Rm8()
	case 0x8B:
		e.movR32Rm32()
	case 0x8D:
		e.leaR32Rm32()
	case 0xA1:
		e.movEaxMoffs32()
	case 0xA8:
		e.testAlImm8()
	case 0xA9:
		if (e.genuineProtectedEnable && !e.operandSizeOverride) || (! e.genuineProtectedEnable && e.operandSizeOverride) {
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
	case 0xC1:
		e.codeC1()
	case 0xC3:
		e.ret()
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
		if (e.genuineProtectedEnable && !e.operandSizeOverride) || (! e.genuineProtectedEnable && e.operandSizeOverride) {
			e.movR32Imm32()
		} else {
			e.movR16Imm16()
		}
	case 0xE8:
		if (e.genuineProtectedEnable && !e.operandSizeOverride) || (! e.genuineProtectedEnable && e.operandSizeOverride) {
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
	case 0xF3:
		// rep prefix
		fmt.Printf("repeat %d times.\n", e.getRegister32(ECX))
		eip := e.eip + 1
		for e.getRegister32(ECX) > 1 {
			e.eip = eip
			e.execInst()
			e.decRegister32(ECX, 1)
		}
	case 0xF4:
		e.halt()
	case 0xFA:
		e.cli()
	case 0xFC:
		e.cld()
	case 0xFF:
		e.codeFf()
	default:
		return errors.New(fmt.Sprintf("opecode = %x", e.getCode8(0)) + " is not implemented.")
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

func (e *Emulator) insd() {
	ioAddress := e.getRegister16(DX)
	value := e.io.in32(ioAddress)
	memAddress := e.getRegister32(EDI)
	fmt.Printf("(insd) input 0x%08x from io[0x%x] to memory[0x%x]\n",
		value, ioAddress, memAddress)
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

func (e *Emulator) code0f() {
	lgdt := func() {
		m := e.parseModRM()
		address := uint32(e.calcMemoryAddress16(m))
		e.gdtrSize = e.getMemory16(address)
		e.gdtrBase = e.getMemory32(address+2)
		fmt.Printf("address=0x%x gdtrSize=0x%x gdtrBase=0x%x\n",
		address, e.gdtrSize, e.gdtrBase)
		e.dumpGDTEntry(e.gdtrBase)
		e.dumpGDTEntry(e.gdtrBase + 8)
		e.dumpGDTEntry(e.gdtrBase + 16)
	}
	movR32Cr0 := func() {
		m := e.parseModRM()
		e.setR32(m, e.cr[0])
	}
	movCr0R32 := func() {
		m := e.parseModRM()
		e.cr[0] = e.getR32(m)
	}
	MovzxR32Rm8:= func() {
		m := e.parseModRM()
		fmt.Printf("m.disp8=0x%x\n", m.disp32)
		e.setRegister32(m.opecode, uint32(e.getMemory8(m.disp32)))
	}
	MovzxR32Rm16 := func() {
		m := e.parseModRM()
		fmt.Printf("m.disp32=0x%x\n", m.disp32)
		e.setRegister32(m.opecode, uint32(e.getMemory16(m.disp32)))
	}

	second := e.getCode8(1)
	e.eip+=2
	if second == 0x01 {
		lgdt()
	} else if second == 0x20 {
		movR32Cr0()
	} else if second == 0x22 {
		movCr0R32()
	} else if second == 0xB6 {
		MovzxR32Rm8()
	} else if second == 0xB7 {
		MovzxR32Rm16()
	} else {
		panic(fmt.Sprintf("0x0F 0x%x is not implemented\n",second))
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
	e.eip+=2
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
	fmt.Printf("value=0x%x\n", value)
	e.setRegister32(EAX, value)
	e.eip += 5
}

func (e *Emulator) movRm32Imm32() {
	e.eip++
	m := e.parseModRM()
	value := e.getCode32(0)
	e.eip += 4
	e.setRm32(m, value)
}

func (e *Emulator) orEaxImm32() {
	value := e.getCode32(1)
	e.setRegister32(EAX, value)
	e.eip += 5
}

func (e *Emulator) code81() {
	addRm32Imm32 := func(e *Emulator, m ModRM) {
		rm32 := e.getRm32(m)
		imm32 := e.getCode32(0)
		e.eip+=4
		fmt.Printf("rm32 value=0x%x imm32 value=0x%x\n", rm32, imm32)
	result := uint64(rm32) + uint64(imm32)
		e.setRm32(m, uint32(result))
	}
	cmpRm32Imm32 := func(e *Emulator, m ModRM) {
		rm32 := e.getRm32(m)
		imm32 := e.getCode32(0)
		e.eip+=4
		fmt.Printf("rm32 value=0x%x imm32 value=0x%x\n", rm32, imm32)
		result := uint64(rm32) - uint64(imm32)
		e.eflags.updateBySub(rm32, imm32, result)
		// e.setRm32(m, uint32(result))
	}

	e.eip++
	m := e.parseModRM()
	
	if (e.genuineProtectedEnable == false && e.operandSizeOverride == false ||
		e.genuineProtectedEnable == true && e.operandSizeOverride == true) {
		panic("16bit mode is not implemented")
	}

	switch m.opecode {
	case 0:
		addRm32Imm32(e, m)
	case 7:
		cmpRm32Imm32(e, m)
	default:
		panic(fmt.Sprintf("opecode = %d\n", m.opecode) + "not implemented")
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
		e.setRm32(m, rm32 & uint32(imm8))
	}
	orRm32Imm8 := func(e *Emulator, m ModRM) {
		rm32 := e.getRm32(m)
		imm8 := uint32(e.getSignCode8(0))
		e.eip++
		e.setRm32(m, rm32 | uint32(imm8))
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
	
	if (e.genuineProtectedEnable == false && e.operandSizeOverride == false ||
		e.genuineProtectedEnable == true && e.operandSizeOverride == true) {
		panic("16bit mode is not implemented")
	}

	switch m.opecode {
	case 0:
		addRm32Imm8(e, m)
	case 1:
		orRm32Imm8(e,m)
	case 4:
		andRm32Imm8(e,m)
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
		e.setRm32(m, rm32 >> imm8)
		// TODO: change elfags
	}

	shlRm32Imm8 := func(e *Emulator, m ModRM) {
		rm32 := e.getRm32(m)
		imm8 := uint32(e.getCode8(0))
		e.eip++
		e.setRm32(m, rm32 << imm8)
		// TODO: change elfags
	}

	switch m.opecode {
	case 4:
		shlRm32Imm8(e,m)
	case 5:
		shrRm32Imm8(e,m)
	default:
		panic(fmt.Sprintf("opecode = %d\n", m.opecode) + "not implemented at codeC1")
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
		fmt.Printf("address=0x%x jmpAddress=0x%x\n", address, jmpAddress)
		e.eip = jmpAddress
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
	// case 4:
	// 	jmpRm32(e, m)
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
	fmt.Printf("rm32=0x%x r32=0x%x\n",rm32,r32)
	e.setRm32(m, rm32-r32)
}

func (e *Emulator) addRm32R32() {
	e.eip++
	m := e.parseModRM()
	r32 := e.getR32(m)
	rm32 := e.getRm32(m)
	e.setRm32(m, r32+rm32)
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
	fmt.Printf("leaR32Rm32 r=%d\n", m.opecode)
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

func (e *Emulator) cmpRm32R32() { e.eip++
	m := e.parseModRM()
	r32 := e.getR32(m)
	rm32 := e.getRm32(m)
	result := uint64(rm32) - uint64(r32)
	e.eflags.updateBySub(rm32, r32, result)
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
	e.eip += 5
}

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
	if e.eflags.isEnable(ZeroFlag) && e.eflags.isEnable(SignFlag) == e.eflags.isEnable(OverflowFlag) {
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

// util

// dump GDT entry
func (e *Emulator) dumpGDTEntry(physAddr uint32) {
	entry := e.getMemory64(physAddr)
	segmentBaseAddr := uint32((((entry >> 56)&0xFF)<<24) | (((entry >>32) & 0xFF)<<16) | ((entry>>16) & 0xFFFF))
	segmentLimit := uint32((((entry>>48)&0xF) <<16) | (entry & 0xFFFF))
	isCodeSegment := (entry>>43) & 1
	fmt.Printf("GDTEntry[%d]={entryPhysAddr=0x%x segmentBaseAddr=0x%x segmentLimit=0x%x isCodeSegment=0x%x}\n",
	(physAddr-e.gdtrBase)/8,physAddr, segmentBaseAddr,segmentLimit,isCodeSegment)
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
	fmt.Printf("rm32 address=0x%x\n", address)
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
		}

		return e.getRegister32(m.rm)
	} else if m.mod == 1 {
		// [register + disp8]
		disp8 := m.getDisp8()
		if disp8 < 0 {
			return e.getRegister32(m.rm) - uint32(-disp8)
		}
		return e.getRegister32(m.rm) + uint32(disp8)
	} else if m.mod == 2 {
		// [redister + disp16/32]
		return e.getRegister32(m.rm) + m.disp32
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

func (e *Emulator) setMemory8(address uint32, value uint8) {
	e.memory[address] = value
}

func (e *Emulator) setMemory16(address uint32, value uint16) {
	for i := uint32(0); i < 2; i++ {
		e.setMemory8(address+i, uint8(value>>uint32(i*8)&0xFF))
	}
}

func (e *Emulator) setMemory32(address, value uint32) {
	for i := uint32(0); i < 4; i++ {
		e.setMemory8(address+i, uint8(value>>uint32(i*8)&0xFF))
	}
}

// TODO: consider linear address transformation using DS
func (e *Emulator) getMemory8(address uint32) uint8 {
	return e.memory[address]
}

func (e *Emulator) getMemory16(address uint32) uint16 {
	var ret uint16
	for i := uint32(0); i < 2; i++ {
		ret |= uint16(e.getMemory8(address+i)) << uint32(i*8)
	}
	return ret
}

func (e *Emulator) getMemory32(address uint32) uint32 {
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

func (e *Emulator) dump() {
	color.New(color.FgBlack).Printf("" +
		"-------------------------------" +
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
		"EIP=0x%08x ",
		e.registers[EAX],
		e.registers[ECX],
		e.registers[EDX],
		e.registers[EBX],
		e.registers[ESP],
		e.registers[ESI],
		e.registers[EDI],
		e.registers[EBP],
		e.eip,
	)
	color.New(color.FgGreen).Printf("(opecode=%02x, %s)\n",
		e.getCode8(0), e.disasm[uint64(e.eip)])
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
	return e.memory[addr]
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
	sib     uint8
	disp32  uint32 // This can be regarded as (disp8, signed int8, disp16 signed int16).
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
		}

		if (m.mod == 0 && m.rm == 5) || m.mod == 2 {
			// fmt.Printf("get disp32 from eip=0x%x\n", e.eip)
			m.disp32 = e.getCode32(0)
			e.eip += 4
		} else if m.mod == 1 {
			m.setDisp8(e.getSignCode8(0))
			e.eip++
		}
	}

	return m
}
