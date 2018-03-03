package main

import (
	"errors"
	"fmt"
	"github.com/fatih/color"
)

const (
	EAX = 0
	ECX = 1
	EDX = 2
	EBX = 3
	EBP = 5
	ESI = 6
	EDI = 7
)

type Emulator struct {
	registers [8]uint32 // general registers
	eflags    uint32    // eflags
	memory    []uint8   // physical memory
	eip       uint32    // program counter
	esp       uint32    // stack pointer (#reg = 4)
}

func NewEmulator(memory_size, eip, esp uint32) *Emulator {
	return &Emulator{
		memory: make([]uint8, memory_size),
		eip:    eip,
		esp:    esp,
	}
}

// emulate instruction

func (e *Emulator) exec_inst() error {
	switch e.get_code8(0) {
	case 0x01:
		e.add_rm32_r32()
	case 0x83:
		e.code_83()
	case 0x89:
		e.mov_rm32_r32()
	case 0x8B:
		e.mov_r32_rm32()
	case 0xC3:
		e.ret()
	case 0xC7:
		e.mov_rm32_imm32()
	case 0xEB:
		e.short_jmp()
	case 0xB8, 0xB9, 0xBA, 0xBB, 0xBC, 0xBD, 0xBE, 0xBF:
		e.mov_r32_imm32()
	case 0xE8:
		e.call_rel32()
	case 0xE9:
		e.jmp_rel32()
	case 0xFF:
		e.code_ff()
	default:
		return errors.New(fmt.Sprintf("opecode = %x", e.get_code8(0)) + " is not implemented.")
	}
	return nil
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
		e.set_rm32(m, rm32-imm8)
	}
	e.eip++
	m := e.parseModRM()
	switch m.opecode {
	case 5:
		sub_rm32_imm8(e, m)
	default:
		panic("not implemented")
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

func (e *Emulator) short_jmp() {
	diff := uint32(e.get_sign_code8(1))
	e.eip += diff + uint32(2)
	// if diff < 0 {
	// 	e.eip = e.eip + uint32(-diff) + uint32(2)
	// } else {
	// 	e.eip = e.eip + uint32(diff) + uint32(2)
	// }
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
	reg := e.get_code8(0) - 0x05
	e.push32(e.get_register32(reg))
	e.eip++
}

func (e *Emulator) pop_r32() {
	reg := e.get_code8(0) - 0x05
	e.set_register32(reg, e.pop32())
	e.eip++
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

func (e *Emulator) get_r32(m ModRM) uint32 {
	return e.get_register32(m.opecode)
}

func (e *Emulator) set_r32(m ModRM, value uint32) {
	e.set_register32(m.opecode, value)
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
	if rm == 4 {
		return e.esp
	} else {
		return e.registers[rm]
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

func (e *Emulator) dump() {
	color.New(color.FgBlack).Printf("" +
		"-------------------------------" +
		"-----------------------------\n")
	color.New(color.FgCyan).Printf(""+
		"EAP=0x%08x "+
		"EBP=0x%08x "+
		"ECP=0x%08x "+
		"EDP=0x%08x\n"+
		"ESI=0x%08x "+
		"EDI=0x%08x "+
		"EBP=0x%08x "+
		"ESP=0x%08x "+
		"EIP=0x%08x ",
		e.registers[EAX],
		e.registers[EBX],
		e.registers[ECX],
		e.registers[EDX],
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
