package main

import (
	"errors"
	"github.com/fatih/color"
	"strconv"
)

const (
	EAX = 0
	EBX = 1
	ECX = 2
	EDX = 3
	ESI = 4
	EBP = 5
)

type Emulator struct {

	// general registers
	registers [6]uint32

	// eflags
	eflags uint32

	// physical memory
	memory []uint8

	// program counter
	eip uint32

	// stack pointer
	esp uint32
}

func NewEmulator(memory_size, eip, esp uint32) *Emulator {
	e := Emulator{}
	e.memory = make([]uint8, memory_size)
	e.eip = eip
	e.esp = esp
	return &e
}

// emulate instruction

func (e *Emulator) exec_inst() error {
	switch e.get_code8(0) {
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

func (e *Emulator) short_jmp() {
	diff := uint32(e.get_sign_code8(1))
	e.eip += diff + uint32(2)
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

// util

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
