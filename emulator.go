package main

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/fatih/color"
	"os"
)

const (
	// 32bit register
	EAX = 0
	ECX = 1
	EDX = 2
	EBX = 3
	EBP = 5
	ESI = 6
	EDI = 7

	// TODO: 16bit register

	// 8bit register
	AL = EAX
	CL = ECX
	DL = EDX
	BL = EBX
	AH = AL + 4
	CH = CL + 4
	DH = DL + 4
	BH = BL + 4

	// eflags
	CARRY_FLAG    = 1 << 0
	ZERO_FLAG     = 1 << 6
	SIGN_FLAG     = 1 << 7
	OVERFLOW_FLAG = 1 << 11
)

type Emulator struct {
	registers   [8]uint32 // general registers
	eflags      uint32    // eflags
	memory      []uint8   // physical memory
	eip         uint32    // program counter
	esp         uint32    // stack pointer (#reg = 4)
	is32bitmode bool      // if this value is false, the enulator work as 16 bit mode
	isSilent    bool      // silent mode
}

func NewEmulator(memory_size, eip, esp uint32, is32bitmode, isSilent bool) *Emulator {
	return &Emulator{
		memory:      make([]uint8, memory_size),
		eip:         eip,
		esp:         esp,
		is32bitmode: is32bitmode,
		isSilent:    isSilent,
	}
}

// emulate instruction

func (e *Emulator) exec_inst() error {
	switch e.get_code8(0) {
	case 0x01:
		e.add_rm32_r32()
	case 0x3b:
		e.cmp_r32_rm32()
	case 0x3c:
		e.cmp_al_imm8()
	case 0x40, 0x41, 0x42, 0x43, 0x44, 0x45, 0x46, 0x47:
		e.inc_r32()
	case 0x48, 0x49, 0x4a, 0x4b, 0x4c, 0x4d, 0x4e, 0x4f:
		e.dec_r32()
	case 0x50, 0x51, 0x52, 0x53, 0x54, 0x55, 0x56, 0x57:
		e.push_r32()
	case 0x58, 0x59, 0x5a, 0x5b, 0x5c, 0x5d, 0x5e, 0x5f:
		e.pop_r32()
	case 0x6a:
		e.push_imm8()
	case 0x74:
		e.jz()
	case 0x78:
		e.js()
	case 0x7E:
		e.jng()
	case 0x7F:
		e.jg()
	case 0x83:
		e.code_83()
	case 0x89:
		e.mov_rm32_r32()
	case 0x8A:
		e.mov_r8_rm8()
	case 0x8B:
		e.mov_r32_rm32()
	case 0xB0:
		e.mov_r8_imm8()
	case 0x90:
		e.nop()
	case 0xC3:
		e.ret()
	case 0xC7:
		e.mov_rm32_imm32()
	case 0xC9:
		e.leave()
	case 0xEB:
		e.short_jmp()
	case 0xB8, 0xB9, 0xBA, 0xBB, 0xBC, 0xBD, 0xBE, 0xBF:
		e.mov_r32_imm32()
	case 0xE8:
		e.call_rel32()
	case 0xE9:
		e.jmp_rel32()
	case 0xEC:
		e.in_al_dx()
	case 0xEE:
		e.out_al_dx()
	case 0xFF:
		e.code_ff()
	default:
		return errors.New(fmt.Sprintf("opecode = %x", e.get_code8(0)) + " is not implemented.")
	}
	return nil
}

func (e *Emulator) nop() {
	e.eip++
}

func (e *Emulator) mov_r32_imm32() {
	reg := e.get_code8(0) - 0xB8
	value := e.get_code32(1)
	e.registers[reg] = value
	e.eip += 5
}

func (e *Emulator) mov_rm32_imm32() {
	e.eip++
	m := e.parseModRM()
	value := e.get_code32(0)
	e.eip += 4
	e.set_rm32(m, value)
}

func (e *Emulator) code_83() {
	sub_rm32_imm8 := func(e *Emulator, m ModRM) {
		rm32 := e.get_rm32(m)
		imm8 := uint32(e.get_sign_code8(0))
		e.eip++
		result := uint64(rm32) - uint64(imm8)
		e.set_rm32(m, uint32(result))
		e.update_eflags_sub(rm32, imm8, result)
	}
	add_rm32_imm8 := func(e *Emulator, m ModRM) {
		rm32 := e.get_rm32(m)
		imm8 := uint32(e.get_sign_code8(0))
		e.eip++
		e.set_rm32(m, rm32+imm8)
	}
	cmp_rm32_imm8 := func(e *Emulator, m ModRM) {
		rm32 := e.get_rm32(m)
		imm8 := uint32(e.get_sign_code8(0))
		e.eip++
		result := uint64(rm32) - uint64(imm8)
		e.update_eflags_sub(rm32, imm8, result)
	}
	e.eip++
	m := e.parseModRM()
	switch m.opecode {
	case 0:
		add_rm32_imm8(e, m)
	case 5:
		sub_rm32_imm8(e, m)
	case 7:
		cmp_rm32_imm8(e, m)
	default:
		panic(fmt.Sprintf("opecode = %d\n", m.opecode) + "not implemented")
	}
}

func (e *Emulator) code_ff() {
	inc_rm32 := func(e *Emulator, m ModRM) {
		rm32 := e.get_rm32(m)
		e.set_rm32(m, rm32+1)
	}
	e.eip++
	m := e.parseModRM()
	switch m.opecode {
	case 0:
		inc_rm32(e, m)
	default:
		panic("not implemented")
	}
}

func (e *Emulator) mov_rm8_r8() {
	e.eip++
	m := e.parseModRM()
	r8 := e.get_r8(m)
	e.set_rm8(m, r8)
}

func (e *Emulator) mov_rm32_r32() {
	e.eip++
	m := e.parseModRM()
	r32 := e.get_r32(m)
	e.set_rm32(m, r32)
}

func (e *Emulator) add_rm32_r32() {
	e.eip++
	m := e.parseModRM()
	r32 := e.get_r32(m)
	rm32 := e.get_rm32(m)
	e.set_rm32(m, r32+rm32)
}

func (e *Emulator) mov_r32_rm32() {
	e.eip++
	m := e.parseModRM()
	rm32 := e.get_rm32(m)
	e.set_r32(m, rm32)
}

func (e *Emulator) mov_r8_rm8() {
	e.eip++
	m := e.parseModRM()
	rm8 := e.get_rm8(m)
	e.set_r8(m, rm8)
}

func (e *Emulator) mov_r8_imm8() {
	reg := e.get_code8(0) - 0xB0
	e.set_register8(reg, e.get_code8(1))
	e.eip += 2
}

func (e *Emulator) cmp_r32_rm32() {
	e.eip++
	m := e.parseModRM()
	r32 := e.get_r32(m)
	rm32 := e.get_rm32(m)
	result := uint64(r32) - uint64(rm32)
	e.update_eflags_sub(r32, rm32, result)
}

func (e *Emulator) cmp_al_imm8() {
	al := uint32(e.get_register8(AL))
	value := uint32(e.get_code8(1))
	result := uint64(al) - uint64(value)
	e.update_eflags_sub(al, value, result)
	e.eip += 2
}

func (e *Emulator) short_jmp() {
	diff := int32(e.get_sign_code8(1))
	if diff < 0 {
		e.eip = e.eip - uint32(-diff) + uint32(2)
	} else {
		e.eip = e.eip + uint32(diff) + uint32(2)
	}
}

func (e *Emulator) jmp_rel32() {
	diff := e.get_sign_code32(1)
	if diff < 0 {
		e.eip = e.eip - uint32(-diff) + uint32(5)
	} else {
		e.eip = e.eip + uint32(diff) + uint32(5)
	}
}

func (e *Emulator) push_r32() {
	reg := e.get_code8(0) - 0x50
	e.push32(e.get_register32(reg))
	e.eip++
}

func (e *Emulator) inc_r32() {
	reg := e.get_code8(0) - 0x40
	e.set_register32(reg, e.get_register32(reg)+1)
	e.eip++
}

func (e *Emulator) dec_r32() {
	reg := e.get_code8(0) - 0x48
	e.set_register32(reg, e.get_register32(reg)-1)
	e.eip++
}

func (e *Emulator) pop_r32() {
	reg := e.get_code8(0) - 0x58
	e.set_register32(reg, e.pop32())
	e.eip++
}

func (e *Emulator) push_imm8() {
	value := uint32(e.get_code8(1))
	e.push32(value)
	e.eip += 2
}

func (e *Emulator) call_rel32() {
	diff := e.get_singed_code32(1)
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

func (e *Emulator) jz() {
	if e.get_eflag(ZERO_FLAG) {
		e.eip += uint32(2) + uint32(e.get_sign_code8(1))
	} else {
		e.eip += uint32(2)
	}
}

func (e *Emulator) js() {
	if e.get_eflag(SIGN_FLAG) {
		e.eip += uint32(2) + uint32(e.get_sign_code8(1))
	} else {
		e.eip += uint32(2)
	}
}

func (e *Emulator) jns() {
	if e.get_eflag(SIGN_FLAG) {
		e.eip += uint32(2)
	} else {
		e.eip += uint32(2) + uint32(e.get_sign_code8(1))
	}
}

func (e *Emulator) jg() {
	if e.get_eflag(ZERO_FLAG) && e.get_eflag(SIGN_FLAG) == e.get_eflag(OVERFLOW_FLAG) {
		e.eip += uint32(2) + uint32(e.get_sign_code8(1))
	} else {
		e.eip += uint32(2)
	}
}

func (e *Emulator) jng() {
	if e.get_eflag(ZERO_FLAG) || e.get_eflag(SIGN_FLAG) != e.get_eflag(OVERFLOW_FLAG) {
		e.eip += uint32(2) + uint32(e.get_sign_code8(1))
	} else {
		e.eip += uint32(2)
	}
}

func (e *Emulator) jl() {
	if e.get_eflag(SIGN_FLAG) != e.get_eflag(OVERFLOW_FLAG) {
		e.eip += uint32(e.get_sign_code8(1))
	} else {
		e.eip += 2
	}
}

func (e *Emulator) jle() {
	if e.get_eflag(ZERO_FLAG) || e.get_eflag(SIGN_FLAG) != e.get_eflag(OVERFLOW_FLAG) {
		e.eip += uint32(e.get_sign_code8(1))
	} else {
		e.eip += 2
	}
}

func (e *Emulator) in_al_dx() {
	address := uint16(e.get_register32(EDX) & 0xffff)
	value := e.io_in8(address)
	e.set_register8(AL, value)
	e.eip++
}

func (e *Emulator) out_al_dx() {
	address := uint16(e.get_register32(EDX) & 0xffff)
	value := e.get_register8(AL)
	e.io_out8(address, value)
	e.eip++
}

// util
func (e *Emulator) set_rm32(m ModRM, value uint32) {
	if m.mod == 3 {
		e.set_register32(m.rm, value)
	} else {
		address := e.calc_memory_address(m)
		e.set_memory32(address, value)
	}
}

func (e *Emulator) get_rm32(m ModRM) uint32 {
	if m.mod == 3 {
		return e.get_register32(m.rm)
	} else {
		address := e.calc_memory_address(m)
		return e.get_memory32(address)
	}
}

func (e *Emulator) get_rm8(m ModRM) uint8 {
	if m.mod == 3 {
		return e.get_register8(m.rm) // TODO check OK?
	} else {
		address := e.calc_memory_address(m)
		return e.get_memory8(address)
	}
}

func (e *Emulator) get_r32(m ModRM) uint32 {
	return e.get_register32(m.opecode)
}

func (e *Emulator) get_r8(m ModRM) uint8 {
	return e.get_register8(m.opecode) // TOOD: Is index correct for 8bit register?
}

func (e *Emulator) set_r32(m ModRM, value uint32) {
	e.set_register32(m.opecode, value)
}

func (e *Emulator) set_r8(m ModRM, value uint8) {
	e.set_register8(m.opecode, value)
}

func (e *Emulator) set_rm8(m ModRM, value uint8) {
	if m.mod == 3 {
		e.set_register8(m.rm, value)
	} else {
		address := e.calc_memory_address(m)
		e.set_memory8(address, value)
	}
}

func (e *Emulator) calc_memory_address(m ModRM) uint32 {
	if m.mod == 0 {
		// [register + resiger]
		if m.rm == 5 {
			return m.disp32 // Is this a EBP?
		} else {
			return e.get_register32(m.rm)
		}
	} else if m.mod == 1 {
		// [register + disp8]
		disp8 := m.getDisp8()
		if disp8 < 0 {
			return e.get_register32(m.rm) - uint32(-disp8)
		} else {
			return e.get_register32(m.rm) + uint32(disp8)
		}
	} else if m.mod == 2 {
		// [redister + disp16/32]
		return e.get_register32(m.rm) + m.disp32
	} else {
		// register
		panic("ModRM mod = 4 is not implemented")
	}
}

func (e *Emulator) set_register32(rm uint8, value uint32) {
	if rm == 4 {
		e.esp = value
	} else {
		e.registers[rm] = value
	}
}

func (e *Emulator) get_register32(rm uint8) uint32 {
	// TODO: e.esp should be moved to e.registers
	if rm == 4 {
		return e.esp
	} else {
		return e.registers[rm]
	}
}

func (e *Emulator) get_register8(rm uint8) uint8 {
	if rm < 4 {
		return uint8(e.registers[rm] & 0xff)
	} else {
		return uint8((e.registers[rm-4] >> 8) & 0xff)
	}
}

func (e *Emulator) set_register8(rm, value uint8) {
	if rm < 4 {
		e.registers[rm] = (e.registers[rm] & 0xffffff00) | uint32(value)
	} else {
		e.registers[rm-4] = (e.registers[rm-4] & 0xffff00ff) | (uint32(value) << 8)
	}
}

func (e *Emulator) set_memory8(address uint32, value uint8) {
	e.memory[address] = value
}

func (e *Emulator) set_memory32(address, value uint32) {
	for i := uint32(0); i < 4; i++ {
		e.set_memory8(address+i, uint8(value>>uint32(i*8)&0xFF))
	}
}

func (e *Emulator) get_memory8(address uint32) uint8 {
	return e.memory[address]
}

func (e *Emulator) get_memory32(address uint32) uint32 {
	var ret uint32
	for i := uint32(0); i < 4; i++ {
		ret |= uint32(e.get_memory8(address+i)) << uint32(i*8)
	}
	return ret
}

func (e *Emulator) push32(value uint32) {
	address := e.esp - 4
	e.set_memory32(address, value)
	e.esp = address
}

func (e *Emulator) pop32() uint32 {
	value := e.get_memory32(e.esp)
	e.esp += 4
	return value
}

func (E *Emulator) io_in8(address uint16) uint8 {
	switch address {
	case 0x03f8:
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		return input[0]
	default:
		return 0
	}
}

func (E *Emulator) io_out8(address uint16, value uint8) {
	switch address {
	case 0x03f8:
		fmt.Print(string(value))
	default:
		return
	}
}

func (e *Emulator) leave() {
	ebp := e.get_register32(EBP)
	e.esp = ebp
	e.set_register32(EBP, e.pop32())
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
		"ESI=0x%08x\n"+
		"EDI=0x%08x "+
		"EBP=0x%08x "+
		"ESP=0x%08x "+
		"EIP=0x%08x ",
		e.registers[EAX],
		e.registers[ECX],
		e.registers[EDX],
		e.registers[EBX],
		e.registers[ESI],
		e.registers[EDI],
		e.registers[EBP],
		e.esp,
		e.eip,
	)
	color.New(color.FgGreen).Printf("(opecode=%x)\n",
		e.get_code8(0))
}

// get text segment

func (e *Emulator) get_code8(index int32) uint8 {
	var addr uint32
	if index < 0 {
		addr = e.eip - uint32(-index)
	} else {
		addr = e.eip + uint32(index)
	}
	return e.memory[addr]
}

func (e *Emulator) get_sign_code8(index int32) int8 {
	return int8(e.get_code8(index))
}

func (e *Emulator) get_sign_code32(index int32) int32 {
	return int32(e.get_code32(index))
}

func (e *Emulator) get_code32(index int32) uint32 {
	var ret uint32
	for i := int32(0); i < 4; i++ {
		ret |= uint32(e.get_code8(index+i)) << uint32(i*8)
	}
	return ret
}

func (e *Emulator) get_singed_code32(index int32) int32 {
	return int32(e.get_code32(index))
}

func (e *Emulator) update_eflags_sub(v1, v2 uint32, result uint64) {
	sign1 := (v1 >> 31) & 0x01
	sign2 := (v2 >> 31) & 0x01
	signr := uint32((result >> 31) & 0x01)

	e.set_eflag(CARRY_FLAG, result>>32 != 0)
	e.set_eflag(ZERO_FLAG, result == 0)
	e.set_eflag(SIGN_FLAG, signr != 0)
	e.set_eflag(OVERFLOW_FLAG, sign1 != sign2 && sign1 != signr)
}

func (e *Emulator) set_eflag(flag uint32, cond bool) {
	if cond {
		e.eflags |= flag
	} else {
		e.eflags &= ^flag
	}
}

func (e *Emulator) get_eflag(flag uint32) bool {
	return e.eflags&flag != 0
}

// modrm

type ModRM struct {
	mod     uint8
	opecode uint8 // This can be regarded as reg_index.
	rm      uint8
	sib     uint8
	disp32  uint32 // This can be regarded as (disp8, signed int8).
}

func (m *ModRM) getDisp8() int8 {
	return int8(m.disp32 & 0xff)
}

func (m *ModRM) setDisp8(disp8 int8) {
	m.disp32 = (m.disp32 & 0xFFFFFF00) | uint32(disp8)
}

// load ModR/M & increment eip
func (e *Emulator) parseModRM() ModRM {
	code := e.get_code8(0)

	// 76  543                210
	// mod reg_index(opecode) r/m
	m := ModRM{
		mod:     (code >> 6) & 0x03,
		opecode: (code >> 3) & 0x07,
		rm:      code & 0x07,
	}

	e.eip++

	if m.mod != 3 && m.rm == 4 {
		m.sib = e.get_code8(0)
		e.eip++
	}

	if (m.mod == 0 && m.rm == 5) || m.mod == 2 {
		m.disp32 = e.get_code32(0) // Is this a bug on the book?
		e.eip += 4
	} else if m.mod == 1 {
		m.setDisp8(e.get_sign_code8(0))
		e.eip++
	}

	return m
}
