package main

// Just enough MMX to run Go programs: the 386 runtime of Go uses MOVQ through
// an MMX register for its 64bit atomic loads and stores (and refuses to start
// on a cpu without MMX). The MMX registers are the mantissas of the fpu
// registers, which is why they live in x87State.
func (e *Emulator) executeMMX(op uint8) {
	if e.repPrefix != 0 || e.opSize == 16 {
		// the same opcodes with a 66/f2/f3 prefix are SSE instructions
		abort("SSE instruction 0x0f 0x%02x is not implemented", op)
	}
	switch op {
	case 0x6E: // movd mm, rm32
		m := e.parseModRM()
		e.fpu.setMMX(int(m.reg), uint64(e.readRM(m, 32)))
	case 0x7E: // movd rm32, mm
		m := e.parseModRM()
		e.writeRM(m, 32, uint32(e.fpu.mmx(int(m.reg))))
	case 0x6F: // movq mm, mm/m64
		m := e.parseModRM()
		if m.isMem {
			e.fpu.setMMX(int(m.reg), e.read64(m))
		} else {
			e.fpu.setMMX(int(m.reg), e.fpu.mmx(int(m.rm)))
		}
	case 0x7F: // movq mm/m64, mm
		m := e.parseModRM()
		if m.isMem {
			e.write64(m, e.fpu.mmx(int(m.reg)))
		} else {
			e.fpu.setMMX(int(m.rm), e.fpu.mmx(int(m.reg)))
		}
	case 0x77: // emms: the fpu registers are empty again
		for i := range e.fpu.tag {
			e.fpu.tag[i] = false
		}
	}
}
