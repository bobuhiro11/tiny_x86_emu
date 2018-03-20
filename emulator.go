package main

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/fatih/color"
	"io"
	"math/bits"
	"os"
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

// eflags
const (
	CARRY    = uint32(1) << 0
	PF       = uint32(1) << 2
	ZERO     = uint32(1) << 6
	SIGN     = uint32(1) << 7
	IF       = uint32(1) << 9 // interrupt enable flag
	OVERFLOW = uint32(1) << 11
)

// Emulator is an i386 Virtual Machine
type Emulator struct {
	registers   [8]uint32  // general registers
	cr          [16]uint32 // controll registers
	sreg        [4]uint32  // segment registers
	eflags      uint32     // eflags
	gdtrSize	uint16     // global table descriptor table's size
	gdtrBase	uint32     // global table descriptor table's base phys address
	memory      []uint8    // physical memory
	eip         uint32     // program counter
	isSilent    bool       // silent mode
	reader      io.Reader
	writer      io.Writer
	operandSizeOverride bool  // true if operand size override (0x66) is enabled
	genuineProtectedEnable bool // procted mode is refreshed only when sreg is changed
	disasm      map[uint64]string // disasmed code (ex. 32255 -> "0000 add [bx+si],al")
}

// NewEmulator creates New Emulator
	func NewEmulator(memorySize, eip, esp uint32, protectedMode, isSilent bool, reader io.Reader, writer io.Writer, disasm map[uint64]string) *Emulator {
		e := &Emulator{
		memory:      make([]uint8, memorySize),
		eip:         eip,
		isSilent:    isSilent,
		reader:      reader,
		writer:      writer,
		disasm:      disasm,
	}
	e.registers[ESP] = esp
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
	case 0x0F:
		e.code0f()
	case 0x31:
		if e.genuineProtectedEnable {
			e.xorRm32R32()
		} else {
			e.xorRm16R16()
		}
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
	case 0x6a:
		e.pushImm8()
	case 0x74:
		e.jz()
	case 0x75:
		e.jnz()
	case 0x78:
		e.js()
	case 0x7E:
		e.jng()
	case 0x7F:
		e.jg()
	case 0x83:
		e.code83()
	case 0x89:
		e.movRm32R32()
	case 0x8A:
		e.movR8Rm8()
	case 0x8B:
		e.movR32Rm32()
	case 0xA8:
		e.testAlImm8()
	case 0xA9:
		if e.genuineProtectedEnable {
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
		if e.genuineProtectedEnable {
			e.movR32Imm32()
		} else {
			e.movR16Imm16()
		}
	case 0xE8:
		if e.genuineProtectedEnable {
			e.callRel32()
		} else {
			e.callRel16()
		}
	case 0xE9:
		e.jmpRel32()
	case 0xEC:
		e.inAlDx()
	case 0xEE:
		e.outAlDx()
	case 0xF4:
		e.halt()
	case 0xFA:
		e.cli()
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

func (e *Emulator) cli() {
	e.eflags |= IF
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

	second := e.getCode8(1)
	e.eip+=2
	if second == 0x01 {
		lgdt()
	} else if second == 0x20 {
		movR32Cr0()
	} else if second == 0x22 {
		movCr0R32()
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
	ioAddress := e.getCode8(1)
	e.getRegister16(AL)
	if ioAddress == 0x60 {
		// keyboard input register
		e.eip += 2
	} else if ioAddress == 0x64 {
		// keyboard command register
		e.eip += 2
	} else {
		panic(fmt.Sprintf("inAlImm8 not implemented"))
	}
}

func (e *Emulator) inAlImm8() {
	ioAddress := e.getCode8(1)
	if ioAddress == 0x64 {
		e.setRegister8(AL, 0x0) // keyboard status register. 0x0 means not busy.
		e.eip += 2
	} else {
		panic(fmt.Sprintf("inAlImm8 not implemented"))
	}
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

func (e *Emulator) movRm32Imm32() {
	e.eip++
	m := e.parseModRM()
	value := e.getCode32(0)
	e.eip += 4
	e.setRm32(m, value)
}

func (e *Emulator) code83() {
	subRm32Imm8 := func(e *Emulator, m ModRM) {
		rm32 := e.getRm32(m)
		imm8 := uint32(e.getSignCode8(0))
		e.eip++
		result := uint64(rm32) - uint64(imm8)
		e.setRm32(m, uint32(result))
		e.updateEflagsSub(rm32, imm8, result)
	}
	addRm32Imm8 := func(e *Emulator, m ModRM) {
		rm32 := e.getRm32(m)
		imm8 := uint32(e.getSignCode8(0))
		e.eip++
		e.setRm32(m, rm32+imm8)
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
		e.updateEflagsSub(rm32, imm8, result)
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
	case 5:
		subRm32Imm8(e, m)
	case 7:
		cmpRm32Imm8(e, m)
	default:
		panic(fmt.Sprintf("opecode = %d\n", m.opecode) + "not implemented")
	}
}

func (e *Emulator) codeFf() {
	incRm32 := func(e *Emulator, m ModRM) {
		rm32 := e.getRm32(m)
		e.setRm32(m, rm32+1)
	}
	e.eip++
	m := e.parseModRM()
	switch m.opecode {
	case 0:
		incRm32(e, m)
	default:
		panic("not implemented")
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

func (e *Emulator) addRm32R32() {
	e.eip++
	m := e.parseModRM()
	r32 := e.getR32(m)
	rm32 := e.getRm32(m)
	e.setRm32(m, r32+rm32)
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
	e.setSreg16(m, rm16)
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
	e.updateEflagsSub(r32, rm32, result)
}

func (e *Emulator) testEaxImm32() {
	ax := e.getRegister32(EAX)
	value := e.getCode32(1)
	result := ax & value
	if result == 0 {
		e.eflags |= ZERO
	} else {
		e.eflags &= ^ZERO
	}
	e.eflags &= ^CARRY
	e.eflags &= ^OVERFLOW
	e.updateEflagsPf(uint8(result & 0xFF))
	e.eip += 5
}

func (e *Emulator) testAxImm16() {
	ax := uint32(e.getRegister16(AX))
	value := uint32(e.getCode16(1))
	result := ax & value
	if result == 0 {
		e.eflags |= ZERO
	} else {
		e.eflags &= ^ZERO
	}
	e.eflags &= ^CARRY
	e.eflags &= ^OVERFLOW
	e.updateEflagsPf(uint8(result & 0xFF))
	e.eip += 3
}

func (e *Emulator) testAlImm8() {
	al := uint32(e.getRegister8(AL))
	value := uint32(e.getCode8(1))
	result := al & value
	if result == 0 {
		e.eflags |= ZERO
	} else {
		e.eflags &= ^ZERO
	}
	e.eflags &= ^CARRY
	e.eflags &= ^OVERFLOW
	e.updateEflagsPf(uint8(result & 0xFF))
	e.eip += 2
}

func (e *Emulator) cmpAlImm8() {
	al := uint32(e.getRegister8(AL))
	value := uint32(e.getCode8(1))
	result := uint64(al) - uint64(value)
	e.updateEflagsSub(al, value, result)
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
	if e.getEflag(ZERO) {
		e.eip += uint32(2)
	} else {
		e.eip += uint32(2) + uint32(e.getSignCode8(1))
	}
}

func (e *Emulator) jz() {
	if e.getEflag(ZERO) {
		e.eip += uint32(2) + uint32(e.getSignCode8(1))
	} else {
		e.eip += uint32(2)
	}
}

func (e *Emulator) js() {
	if e.getEflag(SIGN) {
		e.eip += uint32(2) + uint32(e.getSignCode8(1))
	} else {
		e.eip += uint32(2)
	}
}

func (e *Emulator) jns() {
	if e.getEflag(SIGN) {
		e.eip += uint32(2)
	} else {
		e.eip += uint32(2) + uint32(e.getSignCode8(1))
	}
}

func (e *Emulator) jg() {
	if e.getEflag(ZERO) && e.getEflag(SIGN) == e.getEflag(OVERFLOW) {
		e.eip += uint32(2) + uint32(e.getSignCode8(1))
	} else {
		e.eip += uint32(2)
	}
}

func (e *Emulator) jng() {
	if e.getEflag(ZERO) || e.getEflag(SIGN) != e.getEflag(OVERFLOW) {
		e.eip += uint32(2) + uint32(e.getSignCode8(1))
	} else {
		e.eip += uint32(2)
	}
}

func (e *Emulator) jl() {
	if e.getEflag(SIGN) != e.getEflag(OVERFLOW) {
		e.eip += uint32(e.getSignCode8(1))
	} else {
		e.eip += 2
	}
}

func (e *Emulator) jle() {
	if e.getEflag(ZERO) || e.getEflag(SIGN) != e.getEflag(OVERFLOW) {
		e.eip += uint32(e.getSignCode8(1))
	} else {
		e.eip += 2
	}
}

func (e *Emulator) inAlDx() {
	address := uint16(e.getRegister32(EDX) & 0xffff)
	value := e.ioIn8(address)
	e.setRegister8(AL, value)
	e.eip++
}

func (e *Emulator) outAlDx() {
	address := uint16(e.getRegister32(EDX) & 0xffff)
	value := e.getRegister8(AL)
	e.ioOut8(address, value)
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

func (e *Emulator) setSreg16(m ModRM, value uint16) {
	e.sreg[m.opecode] = uint32(value)
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

func (e *Emulator) ioIn8(address uint16) uint8 {
	switch address {
	case 0x03f8:
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		return input[0]
	default:
		return 0
	}
}

func (e *Emulator) ioOut8(address uint16, value uint8) {
	switch address {
	case 0x03f8:
		fmt.Fprint(e.writer, string(value))
	default:
		return
	}
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
	color.New(color.FgGreen).Printf("(opecode=%x, %s)\n",
		e.getCode8(0), e.disasm[uint64(e.eip)])
	color.New(color.FgCyan).Printf("EFLAGS=0x%08x\n", e.eflags)
}

// get from eip

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

func (e *Emulator) updateEflagsPf(result uint8) {
	popcnt := bits.OnesCount8(result)
	if popcnt%2 == 0 {
		e.eflags |= PF
	} else {
		e.eflags &= ^PF
	}
}

func (e *Emulator) updateEflagsSub(v1, v2 uint32, result uint64) {
	sign1 := (v1 >> 31) & 0x01
	sign2 := (v2 >> 31) & 0x01
	signr := uint32((result >> 31) & 0x01)

	e.setEflag(CARRY, result>>32 != 0)
	e.setEflag(ZERO, result == 0)
	e.setEflag(SIGN, signr != 0)
	e.setEflag(OVERFLOW, sign1 != sign2 && sign1 != signr)
}

func (e *Emulator) setEflag(flag uint32, cond bool) {
	if cond {
		e.eflags |= flag
	} else {
		e.eflags &= ^flag
	}
}

func (e *Emulator) getEflag(flag uint32) bool {
	return e.eflags&flag != 0
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
			m.disp32 = e.getCode32(0)
			e.eip += 4
		} else if m.mod == 1 {
			m.setDisp8(e.getSignCode8(0))
			e.eip++
		}
	}

	return m
}
