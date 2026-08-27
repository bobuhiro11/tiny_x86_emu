package main

// ModRM is a decoded ModR/M byte together with the effective address it
// describes.
type ModRM struct {
	mod   uint8
	reg   uint8 // register field, also used as an opcode extension
	rm    uint8
	isMem bool
	seg   int    // segment register used for the memory operand
	addr  uint32 // offset of the memory operand inside the segment
}

// parseModRM decodes the ModR/M byte (and the SIB byte and displacement, if
// any) at the current instruction pointer.
func (e *Emulator) parseModRM() ModRM {
	b := e.fetch8()
	m := ModRM{
		mod: (b >> 6) & 0x3,
		reg: (b >> 3) & 0x7,
		rm:  b & 0x7,
	}
	if m.mod == 3 {
		return m
	}
	m.isMem = true
	if e.addrSize == 16 {
		e.decodeAddress16(&m)
	} else {
		e.decodeAddress32(&m)
	}
	if e.segPrefix >= 0 {
		m.seg = e.segPrefix
	}
	return m
}

func (e *Emulator) decodeAddress32(m *ModRM) {
	m.seg = DS
	var base uint32

	if m.rm == 4 {
		sib := e.fetch8()
		scale := (sib >> 6) & 0x3
		index := (sib >> 3) & 0x7
		baseReg := sib & 0x7

		if index != 4 {
			base += e.registers[index] << scale
		}
		if baseReg == 5 && m.mod == 0 {
			base += e.fetch32()
		} else {
			base += e.registers[baseReg]
			if baseReg == ESP || baseReg == EBP {
				m.seg = SS
			}
		}
	} else if m.rm == 5 && m.mod == 0 {
		base = e.fetch32()
		m.addr = base
		return
	} else {
		base = e.registers[m.rm]
		if m.rm == EBP {
			m.seg = SS
		}
	}

	switch m.mod {
	case 1:
		base += signExtend(uint32(e.fetch8()), 8)
	case 2:
		base += e.fetch32()
	}
	m.addr = base
}

func (e *Emulator) decodeAddress16(m *ModRM) {
	m.seg = DS
	var base uint16

	switch m.rm {
	case 0:
		base = e.getRegister16(BX) + e.getRegister16(SI)
	case 1:
		base = e.getRegister16(BX) + e.getRegister16(DI)
	case 2:
		base = e.getRegister16(BP) + e.getRegister16(SI)
		m.seg = SS
	case 3:
		base = e.getRegister16(BP) + e.getRegister16(DI)
		m.seg = SS
	case 4:
		base = e.getRegister16(SI)
	case 5:
		base = e.getRegister16(DI)
	case 6:
		if m.mod == 0 {
			m.addr = uint32(e.fetch16())
			return
		}
		base = e.getRegister16(BP)
		m.seg = SS
	case 7:
		base = e.getRegister16(BX)
	}

	switch m.mod {
	case 1:
		base += uint16(signExtend(uint32(e.fetch8()), 8))
	case 2:
		base += e.fetch16()
	}
	m.addr = uint32(base)
}

func (e *Emulator) readRM(m ModRM, size int) uint32 {
	if !m.isMem {
		return e.getReg(m.rm, size)
	}
	return e.readSeg(m.seg, m.addr, size)
}

func (e *Emulator) writeRM(m ModRM, size int, value uint32) {
	if !m.isMem {
		e.setReg(m.rm, size, value)
		return
	}
	e.writeSeg(m.seg, m.addr, size, value)
}

// getRm32 and friends are convenience wrappers kept for readability.
func (e *Emulator) getRm32(m ModRM) uint32       { return e.readRM(m, 32) }
func (e *Emulator) getRm16(m ModRM) uint16       { return uint16(e.readRM(m, 16)) }
func (e *Emulator) getRm8(m ModRM) uint8         { return uint8(e.readRM(m, 8)) }
func (e *Emulator) setRm32(m ModRM, v uint32)    { e.writeRM(m, 32, v) }
func (e *Emulator) setRm16(m ModRM, v uint16)    { e.writeRM(m, 16, uint32(v)) }
func (e *Emulator) setRm8(m ModRM, v uint8)      { e.writeRM(m, 8, uint32(v)) }
func (e *Emulator) getR32(m ModRM) uint32        { return e.registers[m.reg] }
func (e *Emulator) setR32(m ModRM, value uint32) { e.registers[m.reg] = value }
