package main

import (
	"errors"
	"github.com/fatih/color"
	"strconv"
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
		e.memory: make([]uint8, memory_size),
		e.eip:    eip,
		e.esp:    esp,
	}
}

// emulate instruction

func (e *Emulator) exec_inst() error {
	switch e.get_code8(0) {
	case 0x07:
		e.mov_rm32_imm32()
	case 0xEB:
		e.short_jmp()
	case 0xB8, 0xB9, 0xBA, 0xBB, 0xBC, 0xBD, 0xBE, 0xBF:
		e.mov_r32_imm32()
	default:
		return errors.New(strconv.Itoa(int(e.get_code8(0))) + " is not implemented.")
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

func (e *Emulator) short_jmp() {
	diff := uint32(e.get_sign_code8(1))
	e.eip += diff + uint32(2)
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

func (e *Emulator) dump() {
	color.New(color.FgCyan).Printf(""+
		"eap=0x%x,%d "+
		"ebp=0x%x,%d "+
		"ecp=0x%x,%d "+
		"edp=0x%x,%d "+
		"esi=0x%x,%d "+
		"ebp=0x%x,%d "+
		"esp=0x%x,%d "+
		"eip=0x%x,%d\n",
		e.registers[EAX], e.registers[EAX],
		e.registers[EBX], e.registers[EBX],
		e.registers[ECX], e.registers[ECX],
		e.registers[EDX], e.registers[EDX],
		e.registers[ESI], e.registers[ESI],
		e.registers[EBP], e.registers[EBP],
		e.esp, e.esp,
		e.eip, e.eip,
	)
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
	} else {
		m.setDisp8(e.get_sign_code8(0))
		e.eip++
	}

	return m
}
