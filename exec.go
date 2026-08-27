package main

import "fmt"

// execInst executes a single instruction. Pending hardware interrupts are
// delivered before the instruction is fetched.
func (e *Emulator) execInst() error {
	if e.halted {
		// Nothing will happen until a device raises an interrupt, so jump
		// straight to the next timer deadline instead of spinning.
		e.io.idle()
		return e.checkInterrupts()
	}
	if e.instCount&0x3FF == 0 {
		e.io.step(e.instCount)
	}
	if err := e.checkInterrupts(); err != nil {
		return err
	}
	if e.shutdown {
		return nil
	}
	return e.runInstruction()
}

func (e *Emulator) runInstruction() (err error) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		switch ex := r.(type) {
		case cpuException:
			e.eip = e.instEIP
			err = e.deliver(ex)
		case cpuAbort:
			err = fmt.Errorf("eip=0x%08x: %s", e.instEIP, ex.msg)
		default:
			panic(r)
		}
	}()

	e.instEIP = e.eip
	e.segPrefix = -1
	e.repPrefix = 0
	if e.sreg[CS].db {
		e.opSize, e.addrSize = 32, 32
	} else {
		e.opSize, e.addrSize = 16, 16
	}
	if e.trace {
		e.traceInst()
	}

	opcode := e.fetchPrefixes()
	e.execute(opcode)
	e.instCount++
	return nil
}

// deliver dispatches an exception through the IDT. An exception raised while
// dispatching cannot be handled by the guest, so it is reported as an error.
func (e *Emulator) deliver(ex cpuException) (err error) {
	defer func() {
		if r := recover(); r != nil {
			switch r.(type) {
			case cpuException, cpuAbort:
				err = fmt.Errorf("eip=0x%08x: double fault while handling exception %d",
					e.instEIP, ex.vector)
			default:
				panic(r)
			}
		}
	}()
	e.interrupt(ex.vector, ex.errCode, ex.hasErr, false)
	return nil
}

func (e *Emulator) fetchPrefixes() uint8 {
	for {
		b := e.fetch8()
		switch b {
		case 0x26:
			e.segPrefix = ES
		case 0x2E:
			e.segPrefix = CS
		case 0x36:
			e.segPrefix = SS
		case 0x3E:
			e.segPrefix = DS
		case 0x64:
			e.segPrefix = FS
		case 0x65:
			e.segPrefix = GS
		case 0x66:
			if e.sreg[CS].db {
				e.opSize = 16
			} else {
				e.opSize = 32
			}
		case 0x67:
			if e.sreg[CS].db {
				e.addrSize = 16
			} else {
				e.addrSize = 32
			}
		case 0xF0: // lock
		case 0xF2, 0xF3:
			e.repPrefix = b
		default:
			return b
		}
	}
}

func (e *Emulator) dataSeg(def int) int {
	if e.segPrefix >= 0 {
		return e.segPrefix
	}
	return def
}

func (e *Emulator) jumpRel(size int) {
	rel := signExtend(e.fetchImm(size), size)
	e.eip += rel
}

func (e *Emulator) execute(op uint8) {
	size := e.opSize

	switch {
	// ---------------------------------------------------------------
	// 00-3F: the eight ALU operations, each with six addressing forms
	// ---------------------------------------------------------------
	case op < 0x40 && op&0x7 < 6 && op != 0x0F && op != 0x26 && op != 0x2E && op != 0x36 && op != 0x3E:
		index := int(op >> 3)
		form := op & 0x7
		switch form {
		case 0, 1: // rm, r
			opsz := 8
			if form == 1 {
				opsz = size
			}
			m := e.parseModRM()
			a := e.readRM(m, opsz)
			b := e.getReg(m.reg, opsz)
			if r, store := e.alu(index, opsz, a, b); store {
				e.writeRM(m, opsz, r)
			}
		case 2, 3: // r, rm
			opsz := 8
			if form == 3 {
				opsz = size
			}
			m := e.parseModRM()
			a := e.getReg(m.reg, opsz)
			b := e.readRM(m, opsz)
			if r, store := e.alu(index, opsz, a, b); store {
				e.setReg(m.reg, opsz, r)
			}
		case 4, 5: // al/eax, imm
			opsz := 8
			if form == 5 {
				opsz = size
			}
			a := e.getReg(EAX, opsz)
			b := e.fetchImm(opsz)
			if r, store := e.alu(index, opsz, a, b); store {
				e.setReg(EAX, opsz, r)
			}
		}

	case op == 0x0F:
		e.execute0F(e.fetch8())

	// push/pop segment registers
	case op == 0x06:
		e.push(uint32(e.sreg[ES].selector), size)
	case op == 0x07:
		e.loadSegment(ES, uint16(e.pop(size)))
	case op == 0x0E:
		e.push(uint32(e.sreg[CS].selector), size)
	case op == 0x16:
		e.push(uint32(e.sreg[SS].selector), size)
	case op == 0x17:
		e.loadSegment(SS, uint16(e.pop(size)))
	case op == 0x1E:
		e.push(uint32(e.sreg[DS].selector), size)
	case op == 0x1F:
		e.loadSegment(DS, uint16(e.pop(size)))

	case op >= 0x40 && op <= 0x47:
		i := op - 0x40
		e.setReg(i, size, e.inc(size, e.getReg(i, size)))
	case op >= 0x48 && op <= 0x4F:
		i := op - 0x48
		e.setReg(i, size, e.dec(size, e.getReg(i, size)))
	case op >= 0x50 && op <= 0x57:
		e.push(e.getReg(op-0x50, size), size)
	case op >= 0x58 && op <= 0x5F:
		e.setReg(op-0x58, size, e.pop(size))

	case op == 0x60: // pusha
		esp := e.registers[ESP]
		for _, r := range []uint8{EAX, ECX, EDX, EBX} {
			e.push(e.getReg(r, size), size)
		}
		e.push(esp, size)
		for _, r := range []uint8{EBP, ESI, EDI} {
			e.push(e.getReg(r, size), size)
		}
	case op == 0x61: // popa
		for _, r := range []uint8{EDI, ESI, EBP} {
			e.setReg(r, size, e.pop(size))
		}
		e.pop(size) // the saved esp is discarded
		for _, r := range []uint8{EBX, EDX, ECX, EAX} {
			e.setReg(r, size, e.pop(size))
		}

	case op == 0x68:
		e.push(e.fetchImm(size), size)
	case op == 0x6A:
		e.push(signExtend(uint32(e.fetch8()), 8), size)
	case op == 0x69: // imul r, rm, imm32
		m := e.parseModRM()
		a := e.readRM(m, size)
		b := e.fetchImm(size)
		e.setReg(m.reg, size, e.imul2(size, a, b))
	case op == 0x6B: // imul r, rm, imm8
		m := e.parseModRM()
		a := e.readRM(m, size)
		b := signExtend(uint32(e.fetch8()), 8)
		e.setReg(m.reg, size, e.imul2(size, a, b))

	case op == 0x6C: // insb
		e.stringOp(op, 8)
	case op == 0x6D: // insw/insd
		e.stringOp(op, size)
	case op == 0x6E: // outsb
		e.stringOp(op, 8)
	case op == 0x6F: // outsw/outsd
		e.stringOp(op, size)

	case op >= 0x70 && op <= 0x7F:
		rel := signExtend(uint32(e.fetch8()), 8)
		if e.condition(op - 0x70) {
			e.eip += rel
		}

	case op == 0x80 || op == 0x81 || op == 0x83:
		opsz := size
		if op == 0x80 {
			opsz = 8
		}
		m := e.parseModRM()
		var imm uint32
		switch op {
		case 0x80:
			imm = uint32(e.fetch8())
		case 0x81:
			imm = e.fetchImm(opsz)
		default:
			imm = signExtend(uint32(e.fetch8()), 8)
		}
		a := e.readRM(m, opsz)
		if r, store := e.alu(int(m.reg), opsz, a, imm); store {
			e.writeRM(m, opsz, r)
		}

	case op == 0x84 || op == 0x85: // test
		opsz := 8
		if op == 0x85 {
			opsz = size
		}
		m := e.parseModRM()
		e.updateFlagsLogic(opsz, e.readRM(m, opsz)&e.getReg(m.reg, opsz))

	case op == 0x86 || op == 0x87: // xchg
		opsz := 8
		if op == 0x87 {
			opsz = size
		}
		m := e.parseModRM()
		a := e.readRM(m, opsz)
		b := e.getReg(m.reg, opsz)
		e.writeRM(m, opsz, b)
		e.setReg(m.reg, opsz, a)

	case op == 0x88 || op == 0x89: // mov rm, r
		opsz := 8
		if op == 0x89 {
			opsz = size
		}
		m := e.parseModRM()
		e.writeRM(m, opsz, e.getReg(m.reg, opsz))
	case op == 0x8A || op == 0x8B: // mov r, rm
		opsz := 8
		if op == 0x8B {
			opsz = size
		}
		m := e.parseModRM()
		e.setReg(m.reg, opsz, e.readRM(m, opsz))

	case op == 0x8C: // mov rm, sreg
		m := e.parseModRM()
		if m.reg > GS {
			throwException(ExInvalidOpcode, 0, false)
		}
		if m.isMem {
			e.writeRM(m, 16, uint32(e.sreg[m.reg].selector))
		} else {
			e.writeRM(m, size, uint32(e.sreg[m.reg].selector))
		}
	case op == 0x8D: // lea
		m := e.parseModRM()
		e.setReg(m.reg, size, m.addr)
	case op == 0x8E: // mov sreg, rm
		m := e.parseModRM()
		if m.reg > GS || m.reg == CS {
			throwException(ExInvalidOpcode, 0, false)
		}
		e.loadSegment(int(m.reg), uint16(e.readRM(m, 16)))
	case op == 0x8F: // pop rm
		m := e.parseModRM()
		e.writeRM(m, size, e.pop(size))

	case op == 0x90: // nop
	case op >= 0x91 && op <= 0x97: // xchg eax, r
		i := op - 0x90
		a := e.getReg(EAX, size)
		e.setReg(EAX, size, e.getReg(i, size))
		e.setReg(i, size, a)

	case op == 0x98: // cwde / cbw
		if size == 32 {
			e.registers[EAX] = signExtend(e.registers[EAX]&0xFFFF, 16)
		} else {
			e.setRegister16(AX, uint16(signExtend(uint32(e.getRegister8(AL)), 8)))
		}
	case op == 0x99: // cdq / cwd
		if size == 32 {
			if e.registers[EAX]&0x80000000 != 0 {
				e.registers[EDX] = 0xFFFFFFFF
			} else {
				e.registers[EDX] = 0
			}
		} else {
			if e.getRegister16(AX)&0x8000 != 0 {
				e.setRegister16(DX, 0xFFFF)
			} else {
				e.setRegister16(DX, 0)
			}
		}
	case op == 0x9A: // call far
		offset := e.fetchImm(size)
		selector := e.fetch16()
		e.push(uint32(e.sreg[CS].selector), size)
		e.push(e.eip, size)
		e.loadSegment(CS, selector)
		e.eip = offset
	case op == 0x9B: // wait
	case op == 0x9C: // pushf
		e.push(uint32(e.eflags)&0x00FCFFFF, size)
	case op == 0x9D: // popf
		e.setEflags(e.pop(size), size)
	case op == 0x9E: // sahf
		e.eflags = Eflags((uint32(e.eflags) & 0xFFFFFF00) | uint32(e.getRegister8(AH)&0xD5) | 0x2)
	case op == 0x9F: // lahf
		e.setRegister8(AH, uint8(uint32(e.eflags)&0xD5)|0x2)

	case op == 0xA0 || op == 0xA1: // mov eax, moffs
		opsz := 8
		if op == 0xA1 {
			opsz = size
		}
		var off uint32
		if e.addrSize == 16 {
			off = uint32(e.fetch16())
		} else {
			off = e.fetch32()
		}
		e.setReg(EAX, opsz, e.readSeg(e.dataSeg(DS), off, opsz))
	case op == 0xA2 || op == 0xA3: // mov moffs, eax
		opsz := 8
		if op == 0xA3 {
			opsz = size
		}
		var off uint32
		if e.addrSize == 16 {
			off = uint32(e.fetch16())
		} else {
			off = e.fetch32()
		}
		e.writeSeg(e.dataSeg(DS), off, opsz, e.getReg(EAX, opsz))

	case op == 0xA4: // movsb
		e.stringOp(op, 8)
	case op == 0xA5: // movsw/movsd
		e.stringOp(op, size)
	case op == 0xA6: // cmpsb
		e.stringOp(op, 8)
	case op == 0xA7:
		e.stringOp(op, size)
	case op == 0xA8: // test al, imm8
		e.updateFlagsLogic(8, uint32(e.getRegister8(AL))&uint32(e.fetch8()))
	case op == 0xA9: // test eax, imm
		e.updateFlagsLogic(size, e.getReg(EAX, size)&e.fetchImm(size))
	case op == 0xAA: // stosb
		e.stringOp(op, 8)
	case op == 0xAB:
		e.stringOp(op, size)
	case op == 0xAC: // lodsb
		e.stringOp(op, 8)
	case op == 0xAD:
		e.stringOp(op, size)
	case op == 0xAE: // scasb
		e.stringOp(op, 8)
	case op == 0xAF:
		e.stringOp(op, size)

	case op >= 0xB0 && op <= 0xB7:
		e.setRegister8(op-0xB0, e.fetch8())
	case op >= 0xB8 && op <= 0xBF:
		e.setReg(op-0xB8, size, e.fetchImm(size))

	case op == 0xC0 || op == 0xC1: // shift group, imm8 count
		opsz := 8
		if op == 0xC1 {
			opsz = size
		}
		m := e.parseModRM()
		count := uint32(e.fetch8())
		if r, store := e.shift(int(m.reg), opsz, e.readRM(m, opsz), count); store {
			e.writeRM(m, opsz, r)
		}
	case op == 0xC2: // ret imm16
		n := uint32(e.fetch16())
		e.eip = e.pop(size)
		e.addToStackPointer(n)
	case op == 0xC3: // ret
		e.eip = e.pop(size)
	case op == 0xC4 || op == 0xC5: // les/lds
		m := e.parseModRM()
		e.setReg(m.reg, size, e.readSeg(m.seg, m.addr, size))
		sel := uint16(e.readSeg(m.seg, m.addr+uint32(size/8), 16))
		if op == 0xC4 {
			e.loadSegment(ES, sel)
		} else {
			e.loadSegment(DS, sel)
		}
	case op == 0xC6 || op == 0xC7: // mov rm, imm
		opsz := 8
		if op == 0xC7 {
			opsz = size
		}
		m := e.parseModRM()
		e.writeRM(m, opsz, e.fetchImm(opsz))
	case op == 0xC8: // enter
		alloc := uint32(e.fetch16())
		level := e.fetch8() % 32
		e.push(e.getReg(EBP, size), size)
		frame := e.registers[ESP]
		for i := uint8(1); i < level; i++ {
			e.setReg(EBP, size, e.getReg(EBP, size)-uint32(size/8))
			e.push(e.readSeg(SS, e.getReg(EBP, size), size), size)
		}
		if level > 0 {
			e.push(frame, size)
		}
		e.setReg(EBP, size, frame)
		e.registers[ESP] -= alloc
	case op == 0xC9: // leave
		if e.stackAddressSize() == 16 {
			e.setRegister16(SP, e.getRegister16(BP))
		} else {
			e.registers[ESP] = e.registers[EBP]
		}
		e.setReg(EBP, size, e.pop(size))
	case op == 0xCA || op == 0xCB: // retf
		var n uint32
		if op == 0xCA {
			n = uint32(e.fetch16())
		}
		e.eip = e.pop(size)
		e.loadSegment(CS, uint16(e.pop(size)))
		e.addToStackPointer(n)
	case op == 0xCC:
		e.softwareInterrupt(ExBreakpoint)
	case op == 0xCD:
		e.softwareInterrupt(int(e.fetch8()))
	case op == 0xCE:
		if e.eflags.isEnable(OverflowFlag) {
			e.softwareInterrupt(ExOverflow)
		}
	case op == 0xCF:
		e.iret()

	case op == 0xD0 || op == 0xD1 || op == 0xD2 || op == 0xD3:
		opsz := 8
		if op == 0xD1 || op == 0xD3 {
			opsz = size
		}
		m := e.parseModRM()
		count := uint32(1)
		if op == 0xD2 || op == 0xD3 {
			count = uint32(e.getRegister8(CL))
		}
		if r, store := e.shift(int(m.reg), opsz, e.readRM(m, opsz), count); store {
			e.writeRM(m, opsz, r)
		}
	case op == 0xD7: // xlat
		off := e.getReg(EBX, e.addrSize) + uint32(e.getRegister8(AL))
		e.setRegister8(AL, uint8(e.readSeg(e.dataSeg(DS), off, 8)))

	case op >= 0xD8 && op <= 0xDF: // x87 escape: not supported
		e.parseModRM()

	case op >= 0xE0 && op <= 0xE2: // loopne/loope/loop
		rel := signExtend(uint32(e.fetch8()), 8)
		count := e.getReg(ECX, e.addrSize) - 1
		e.setReg(ECX, e.addrSize, count)
		taken := count != 0
		if op == 0xE0 {
			taken = taken && !e.eflags.isEnable(ZeroFlag)
		} else if op == 0xE1 {
			taken = taken && e.eflags.isEnable(ZeroFlag)
		}
		if taken {
			e.eip += rel
		}
	case op == 0xE3: // jecxz
		rel := signExtend(uint32(e.fetch8()), 8)
		if e.getReg(ECX, e.addrSize) == 0 {
			e.eip += rel
		}
	case op == 0xE4 || op == 0xE5: // in al/eax, imm8
		opsz := 8
		if op == 0xE5 {
			opsz = size
		}
		port := uint16(e.fetch8())
		e.checkIOPL()
		e.setReg(EAX, opsz, e.io.in(port, opsz))
	case op == 0xE6 || op == 0xE7: // out imm8, al/eax
		opsz := 8
		if op == 0xE7 {
			opsz = size
		}
		port := uint16(e.fetch8())
		e.checkIOPL()
		e.io.out(port, opsz, e.getReg(EAX, opsz))
	case op == 0xE8: // call rel
		rel := signExtend(e.fetchImm(size), size)
		e.push(e.eip, size)
		e.eip += rel
	case op == 0xE9: // jmp rel
		e.jumpRel(size)
	case op == 0xEA: // jmp far
		offset := e.fetchImm(size)
		selector := e.fetch16()
		e.loadSegment(CS, selector)
		e.eip = offset
	case op == 0xEB:
		e.jumpRel(8)
	case op == 0xEC || op == 0xED: // in al/eax, dx
		opsz := 8
		if op == 0xED {
			opsz = size
		}
		e.checkIOPL()
		e.setReg(EAX, opsz, e.io.in(e.getRegister16(DX), opsz))
	case op == 0xEE || op == 0xEF: // out dx, al/eax
		opsz := 8
		if op == 0xEF {
			opsz = size
		}
		e.checkIOPL()
		e.io.out(e.getRegister16(DX), opsz, e.getReg(EAX, opsz))

	case op == 0xF4: // hlt
		e.hlt()
	case op == 0xF5: // cmc
		e.eflags.setVal(CarryFlag, !e.eflags.isEnable(CarryFlag))
	case op == 0xF6 || op == 0xF7:
		e.group3(op)
	case op == 0xF8:
		e.eflags.unset(CarryFlag)
	case op == 0xF9:
		e.eflags.set(CarryFlag)
	case op == 0xFA: // cli
		e.checkIOPL()
		e.eflags.unset(InterruptFlag)
	case op == 0xFB: // sti
		e.checkIOPL()
		e.eflags.set(InterruptFlag)
		e.intAfter = 1 // interrupts stay blocked for one instruction
	case op == 0xFC:
		e.eflags.unset(DirectionFlag)
	case op == 0xFD:
		e.eflags.set(DirectionFlag)
	case op == 0xFE || op == 0xFF:
		e.group45(op)

	default:
		abort("opcode 0x%02x is not implemented", op)
	}
}

// checkIOPL raises a general protection fault when the current privilege level
// is not allowed to touch the I/O ports or the interrupt flag.
func (e *Emulator) checkIOPL() {
	if e.protectedMode() && e.cpl > uint8((uint32(e.eflags)>>12)&0x3) {
		throwException(ExGeneralProtect, 0, true)
	}
}

// addToStackPointer is used by the ret/retf forms which pop arguments.
func (e *Emulator) addToStackPointer(n uint32) {
	if e.stackAddressSize() == 16 {
		e.setRegister16(SP, e.getRegister16(SP)+uint16(n))
	} else {
		e.registers[ESP] += n
	}
}

func (e *Emulator) group3(op uint8) {
	size := 8
	if op == 0xF7 {
		size = e.opSize
	}
	m := e.parseModRM()
	switch m.reg {
	case 0, 1: // test rm, imm
		imm := e.fetchImm(size)
		e.updateFlagsLogic(size, e.readRM(m, size)&imm)
	case 2: // not
		e.writeRM(m, size, ^e.readRM(m, size))
	case 3: // neg
		v := e.readRM(m, size)
		r := e.updateFlagsSub(size, 0, v, 0)
		e.eflags.setVal(CarryFlag, v&szMask(size) != 0)
		e.writeRM(m, size, r)
	case 4:
		e.mulUnsigned(size, e.readRM(m, size))
	case 5:
		e.mulSigned(size, e.readRM(m, size))
	case 6:
		e.divUnsigned(size, e.readRM(m, size))
	case 7:
		e.divSigned(size, e.readRM(m, size))
	}
}

func (e *Emulator) group45(op uint8) {
	size := 8
	if op == 0xFF {
		size = e.opSize
	}
	m := e.parseModRM()
	switch m.reg {
	case 0: // inc
		e.writeRM(m, size, e.inc(size, e.readRM(m, size)))
	case 1: // dec
		e.writeRM(m, size, e.dec(size, e.readRM(m, size)))
	case 2: // call rm
		target := e.readRM(m, size)
		e.push(e.eip, size)
		e.eip = target
	case 3: // call far m16:32
		offset := e.readSeg(m.seg, m.addr, size)
		selector := uint16(e.readSeg(m.seg, m.addr+uint32(size/8), 16))
		e.push(uint32(e.sreg[CS].selector), size)
		e.push(e.eip, size)
		e.loadSegment(CS, selector)
		e.eip = offset
	case 4: // jmp rm
		e.eip = e.readRM(m, size)
	case 5: // jmp far m16:32
		offset := e.readSeg(m.seg, m.addr, size)
		selector := uint16(e.readSeg(m.seg, m.addr+uint32(size/8), 16))
		e.loadSegment(CS, selector)
		e.eip = offset
	case 6: // push rm
		e.push(e.readRM(m, size), size)
	default:
		abort("opcode 0x%02x /%d is not implemented", op, m.reg)
	}
}

func (e *Emulator) execute0F(op uint8) {
	size := e.opSize

	switch {
	case op == 0x00: // group 6
		m := e.parseModRM()
		switch m.reg {
		case 0: // sldt
			e.writeRM(m, size, 0)
		case 1: // str
			e.writeRM(m, size, uint32(e.tr.selector))
		case 2: // lldt
		case 3: // ltr
			e.loadTaskRegister(uint16(e.readRM(m, 16)))
		default:
			abort("0f 00 /%d is not implemented", m.reg)
		}

	case op == 0x01: // group 7
		m := e.parseModRM()
		switch m.reg {
		case 0: // sgdt
			e.writeSeg(m.seg, m.addr, 16, uint32(e.gdtr.limit))
			e.writeSeg(m.seg, m.addr+2, 32, e.gdtr.base)
		case 1: // sidt
			e.writeSeg(m.seg, m.addr, 16, uint32(e.idtr.limit))
			e.writeSeg(m.seg, m.addr+2, 32, e.idtr.base)
		case 2: // lgdt
			e.gdtr.limit = uint16(e.readSeg(m.seg, m.addr, 16))
			base := e.readSeg(m.seg, m.addr+2, 32)
			if size == 16 {
				base &= 0x00FFFFFF
			}
			e.gdtr.base = base
		case 3: // lidt
			e.idtr.limit = uint16(e.readSeg(m.seg, m.addr, 16))
			base := e.readSeg(m.seg, m.addr+2, 32)
			if size == 16 {
				base &= 0x00FFFFFF
			}
			e.idtr.base = base
		case 4: // smsw
			e.writeRM(m, size, e.cr[0]&0xFFFF)
		case 6: // lmsw
			e.cr[0] = (e.cr[0] &^ 0xF) | (e.readRM(m, 16) & 0xF)
		case 7: // invlpg
			e.flushTLB()
		default:
			abort("0f 01 /%d is not implemented", m.reg)
		}

	case op == 0x06: // clts
		e.cr[0] &^= 0x8
	case op == 0x08, op == 0x09: // invd / wbinvd
	case op == 0x0B: // ud2
		throwException(ExInvalidOpcode, 0, false)

	case op == 0x20: // mov r32, cr
		m := e.parseModRM()
		e.registers[m.rm] = e.cr[m.reg]
	case op == 0x21: // mov r32, dr
		m := e.parseModRM()
		e.registers[m.rm] = 0
	case op == 0x22: // mov cr, r32
		m := e.parseModRM()
		e.setCR(int(m.reg), e.registers[m.rm])
	case op == 0x23: // mov dr, r32
		e.parseModRM()

	case op >= 0x40 && op <= 0x4F: // cmovcc
		m := e.parseModRM()
		v := e.readRM(m, size)
		if e.condition(op - 0x40) {
			e.setReg(m.reg, size, v)
		}

	case op == 0x30: // wrmsr
	case op == 0x31: // rdtsc
		e.registers[EAX] = uint32(e.instCount)
		e.registers[EDX] = uint32(e.instCount >> 32)
	case op == 0x32: // rdmsr
		e.registers[EAX] = 0
		e.registers[EDX] = 0

	case op >= 0x80 && op <= 0x8F: // jcc rel32
		rel := signExtend(e.fetchImm(size), size)
		if e.condition(op - 0x80) {
			e.eip += rel
		}
	case op >= 0x90 && op <= 0x9F: // setcc
		m := e.parseModRM()
		if e.condition(op - 0x90) {
			e.writeRM(m, 8, 1)
		} else {
			e.writeRM(m, 8, 0)
		}

	case op == 0xA0: // push fs
		e.push(uint32(e.sreg[FS].selector), size)
	case op == 0xA1: // pop fs
		e.loadSegment(FS, uint16(e.pop(size)))
	case op == 0xA8: // push gs
		e.push(uint32(e.sreg[GS].selector), size)
	case op == 0xA9: // pop gs
		e.loadSegment(GS, uint16(e.pop(size)))

	case op == 0xA2: // cpuid
		e.cpuid()

	case op == 0xA3, op == 0xAB, op == 0xB3, op == 0xBB: // bt/bts/btr/btc
		m := e.parseModRM()
		e.bitTest(m, int(e.getReg(m.reg, size)), op)
	case op == 0xBA: // group 8: bt/bts/btr/btc with an immediate
		m := e.parseModRM()
		index := int(e.fetch8())
		switch m.reg {
		case 4:
			e.bitTest(m, index, 0xA3)
		case 5:
			e.bitTest(m, index, 0xAB)
		case 6:
			e.bitTest(m, index, 0xB3)
		case 7:
			e.bitTest(m, index, 0xBB)
		default:
			abort("0f ba /%d is not implemented", m.reg)
		}

	case op == 0xA4 || op == 0xA5 || op == 0xAC || op == 0xAD: // shld/shrd
		m := e.parseModRM()
		var count uint32
		if op == 0xA4 || op == 0xAC {
			count = uint32(e.fetch8())
		} else {
			count = uint32(e.getRegister8(CL))
		}
		e.doubleShift(m, size, count, op == 0xA4 || op == 0xA5)

	case op == 0xAF: // imul r, rm
		m := e.parseModRM()
		e.setReg(m.reg, size, e.imul2(size, e.getReg(m.reg, size), e.readRM(m, size)))

	case op == 0xB0 || op == 0xB1: // cmpxchg
		opsz := 8
		if op == 0xB1 {
			opsz = size
		}
		m := e.parseModRM()
		dst := e.readRM(m, opsz)
		acc := e.getReg(EAX, opsz)
		e.updateFlagsSub(opsz, acc, dst, 0)
		if acc&szMask(opsz) == dst&szMask(opsz) {
			e.writeRM(m, opsz, e.getReg(m.reg, opsz))
		} else {
			e.setReg(EAX, opsz, dst)
		}

	case op == 0xB2 || op == 0xB4 || op == 0xB5: // lss/lfs/lgs
		m := e.parseModRM()
		e.setReg(m.reg, size, e.readSeg(m.seg, m.addr, size))
		sel := uint16(e.readSeg(m.seg, m.addr+uint32(size/8), 16))
		switch op {
		case 0xB2:
			e.loadSegment(SS, sel)
		case 0xB4:
			e.loadSegment(FS, sel)
		case 0xB5:
			e.loadSegment(GS, sel)
		}

	case op == 0xB6: // movzx r, rm8
		m := e.parseModRM()
		e.setReg(m.reg, size, e.readRM(m, 8)&0xFF)
	case op == 0xB7: // movzx r, rm16
		m := e.parseModRM()
		e.setReg(m.reg, size, e.readRM(m, 16)&0xFFFF)
	case op == 0xBE: // movsx r, rm8
		m := e.parseModRM()
		e.setReg(m.reg, size, signExtend(e.readRM(m, 8), 8))
	case op == 0xBF: // movsx r, rm16
		m := e.parseModRM()
		e.setReg(m.reg, size, signExtend(e.readRM(m, 16), 16))

	case op == 0xBC || op == 0xBD: // bsf/bsr
		m := e.parseModRM()
		v := e.readRM(m, size) & szMask(size)
		e.eflags.setVal(ZeroFlag, v == 0)
		if v != 0 {
			var i uint32
			if op == 0xBC {
				for i = 0; v&(1<<i) == 0; i++ {
				}
			} else {
				for i = uint32(size) - 1; v&(1<<i) == 0; i-- {
				}
			}
			e.setReg(m.reg, size, i)
		}

	case op == 0xC0 || op == 0xC1: // xadd
		opsz := 8
		if op == 0xC1 {
			opsz = size
		}
		m := e.parseModRM()
		a := e.readRM(m, opsz)
		b := e.getReg(m.reg, opsz)
		e.setReg(m.reg, opsz, a)
		e.writeRM(m, opsz, e.updateFlagsAdd(opsz, a, b, 0))

	case op >= 0xC8 && op <= 0xCF: // bswap
		i := op - 0xC8
		v := e.registers[i]
		e.registers[i] = (v>>24)&0xFF | (v>>8)&0xFF00 | (v<<8)&0xFF0000 | (v << 24)

	default:
		abort("opcode 0x0f 0x%02x is not implemented", op)
	}
}

func (e *Emulator) bitTest(m ModRM, index int, op uint8) {
	size := e.opSize
	var value uint32
	addr := m.addr
	if m.isMem {
		// for memory operands the bit index selects the addressed word
		addr += uint32(index/size) * uint32(size/8)
		index %= size
		if index < 0 {
			index += size
			addr -= uint32(size / 8)
		}
		value = e.readSeg(m.seg, addr, size)
	} else {
		index &= size - 1
		value = e.getReg(m.rm, size)
	}
	bit := uint32(1) << uint(index)
	e.eflags.setVal(CarryFlag, value&bit != 0)

	switch op {
	case 0xAB:
		value |= bit
	case 0xB3:
		value &^= bit
	case 0xBB:
		value ^= bit
	default:
		return
	}
	if m.isMem {
		e.writeSeg(m.seg, addr, size, value)
	} else {
		e.setReg(m.rm, size, value)
	}
}

func (e *Emulator) doubleShift(m ModRM, size int, count uint32, left bool) {
	count &= 0x1F
	if count == 0 {
		return
	}
	dst := e.readRM(m, size) & szMask(size)
	src := e.getReg(m.reg, size) & szMask(size)
	var result uint32
	if left {
		if count > uint32(size) {
			return
		}
		result = (dst << count) | (src >> (uint32(size) - count))
		e.eflags.setVal(CarryFlag, dst&(1<<(uint32(size)-count)) != 0)
	} else {
		if count > uint32(size) {
			return
		}
		result = (dst >> count) | (src << (uint32(size) - count))
		e.eflags.setVal(CarryFlag, dst&(1<<(count-1)) != 0)
	}
	result &= szMask(size)
	e.eflags.setSZP(size, result)
	e.writeRM(m, size, result)
}

func (e *Emulator) cpuid() {
	switch e.registers[EAX] {
	case 0:
		e.registers[EAX] = 1
		e.registers[EBX] = 0x756E6547 // "Genu"
		e.registers[EDX] = 0x49656E69 // "ineI"
		e.registers[ECX] = 0x6C65746E // "ntel"
	case 1:
		e.registers[EAX] = 0x00000480 // family 4, model 8
		e.registers[EBX] = 0
		e.registers[ECX] = 0
		e.registers[EDX] = 0x00000011 // FPU, TSC... keep it minimal
	default:
		e.registers[EAX] = 0
		e.registers[EBX] = 0
		e.registers[ECX] = 0
		e.registers[EDX] = 0
	}
}

func (e *Emulator) setCR(index int, value uint32) {
	old := e.cr[index]
	e.cr[index] = value
	switch index {
	case 0:
		if (old^value)&(CR0PagingFlag|CR0ProtectedModeEnable|CR0WriteProtect) != 0 {
			e.flushTLB()
		}
		if (old^value)&CR0ProtectedModeEnable != 0 && value&CR0ProtectedModeEnable != 0 {
			// entering protected mode: the segment caches keep their real
			// mode contents until the next far jump/segment load
			e.cpl = 0
		}
	case 3:
		e.flushTLB()
	case 4:
		e.flushTLB()
	}
}

func (e *Emulator) hlt() {
	if !e.eflags.isEnable(InterruptFlag) {
		// interrupts are disabled: nothing can ever wake the cpu again
		e.shutdown = true
		fmt.Fprintf(e.writer, "The system has halted.\n")
		return
	}
	e.halted = true
}

func (e *Emulator) setEflags(value uint32, size int) {
	v := value
	if size == 16 {
		v = (uint32(e.eflags) & 0xFFFF0000) | (value & 0xFFFF)
	}
	e.eflags = Eflags((v & 0x003F7FD5) | 0x2)
}
