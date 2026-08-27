package main

import "fmt"

// interrupt dispatches vector through the IDT (or through the real mode
// interrupt vector table when protected mode is off).
func (e *Emulator) interrupt(vector int, errCode uint32, hasErr, software bool) {
	if !e.protectedMode() {
		e.realModeInterrupt(vector)
		return
	}

	offset := uint32(vector) * 8
	if offset+7 > uint32(e.idtr.limit) {
		throwException(ExGeneralProtect, uint32(vector)*8+2, true)
	}
	gate := e.readPhys64(e.translate(e.idtr.base+offset, false, false))

	gateType := uint8(gate>>40) & 0x1F
	dpl := uint8(gate>>45) & 0x3
	present := (gate>>47)&0x1 == 1
	selector := uint16(gate >> 16)
	handler := uint32(gate&0xFFFF) | uint32((gate>>32)&0xFFFF0000)

	if !present {
		throwException(ExSegmentNotPres, uint32(vector)*8+2, true)
	}
	if software && dpl < e.cpl {
		throwException(ExGeneralProtect, uint32(vector)*8+2, true)
	}
	if gateType != 0x0E && gateType != 0x0F && gateType != 0x06 && gateType != 0x07 {
		abort("interrupt %d: unsupported gate type 0x%x", vector, gateType)
	}
	size := 32
	if gateType == 0x06 || gateType == 0x07 {
		size = 16
	}

	oldCPL := e.cpl
	oldSS := e.sreg[SS].selector
	oldESP := e.registers[ESP]
	newCPL := uint8(selector & 0x3)

	if newCPL < oldCPL {
		// switch to the kernel stack described by the task state segment
		esp0 := e.readPhys32(e.translate(e.tr.base+4, false, false))
		ss0 := uint16(e.readPhys32(e.translate(e.tr.base+8, false, false)))
		e.cpl = newCPL // the new stack is accessed with the new privilege
		e.loadSegment(SS, ss0)
		e.registers[ESP] = esp0
	}

	eflags := uint32(e.eflags)
	e.eflags.unset(TrapFlag)
	if gateType == 0x0E || gateType == 0x06 {
		e.eflags.unset(InterruptFlag)
	}

	if newCPL < oldCPL {
		e.push(uint32(oldSS), size)
		e.push(oldESP, size)
	}
	e.push(eflags, size)
	e.push(uint32(e.sreg[CS].selector), size)
	e.push(e.eip, size)
	if hasErr {
		e.push(errCode, size)
	}

	e.loadSegment(CS, selector)
	e.eip = handler
}

func (e *Emulator) realModeInterrupt(vector int) {
	e.push(uint32(e.eflags), 16)
	e.push(uint32(e.sreg[CS].selector), 16)
	e.push(e.eip, 16)
	e.eflags.unset(InterruptFlag)
	e.eflags.unset(TrapFlag)
	offset := e.readLinear(uint32(vector)*4, 16)
	segment := e.readLinear(uint32(vector)*4+2, 16)
	e.loadSegment(CS, uint16(segment))
	e.eip = offset
}

// softwareInterrupt implements the INT instruction.
func (e *Emulator) softwareInterrupt(vector int) {
	if !e.protectedMode() && e.biosInterrupt(vector) {
		return
	}
	e.interrupt(vector, 0, false, true)
}

func (e *Emulator) iret() {
	size := e.opSize
	if !e.protectedMode() {
		e.eip = e.pop(16)
		e.loadSegment(CS, uint16(e.pop(16)))
		e.setEflags(e.pop(16), 16)
		return
	}

	newEIP := e.pop(size)
	newCS := uint16(e.pop(size))
	newFlags := e.pop(size)
	newCPL := uint8(newCS & 0x3)

	if newCPL > e.cpl {
		newESP := e.pop(size)
		newSS := uint16(e.pop(size))
		e.loadSegment(CS, newCS)
		e.eip = newEIP
		e.setEflags(newFlags, size)
		e.loadSegment(SS, newSS)
		e.registers[ESP] = newESP
		return
	}

	e.loadSegment(CS, newCS)
	e.eip = newEIP
	e.setEflags(newFlags, size)
}

func (e *Emulator) loadTaskRegister(selector uint16) {
	desc, ok := e.descriptorAt(selector)
	if !ok {
		throwException(ExGeneralProtect, uint32(selector&0xFFFC), true)
	}
	s := decodeDescriptor(desc)
	e.tr.selector = selector
	e.tr.base = s.base
	e.tr.limit = s.limit
}

// raiseIRQ marks a vector as pending. It is delivered at the next instruction
// boundary if interrupts are enabled.
func (e *Emulator) raiseIRQ(vector int) {
	if vector < 0 || vector > 255 {
		return
	}
	if !e.irr[vector] {
		e.irr[vector] = true
		e.numIRR++
	}
	e.halted = false
}

func (e *Emulator) checkInterrupts() error {
	if e.numIRR == 0 || !e.eflags.isEnable(InterruptFlag) {
		return nil
	}
	if e.intAfter > 0 {
		e.intAfter--
		return nil
	}
	for v := 0; v < 256; v++ {
		if !e.irr[v] {
			continue
		}
		e.irr[v] = false
		e.numIRR--
		e.halted = false
		e.instEIP = e.eip
		return e.deliver(cpuException{vector: v})
	}
	return nil
}

// biosInterrupt emulates the few BIOS services the boot sector of the test
// images relies on. It returns false when the interrupt has to be dispatched
// through the interrupt vector table instead.
func (e *Emulator) biosInterrupt(vector int) bool {
	switch vector {
	case 0x10:
		switch e.getRegister8(AH) {
		case 0x0E: // teletype output
			fmt.Fprintf(e.writer, "%c", e.getRegister8(AL))
			return true
		case 0x00: // set video mode
			return true
		}
	case 0x12: // memory size
		e.setRegister16(AX, 640)
		return true
	}
	return false
}
