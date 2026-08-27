package main

func (e *Emulator) getIndex(reg uint8) uint32 {
	return e.getReg(reg, e.addrSize)
}

func (e *Emulator) addIndex(reg uint8, delta uint32) {
	e.setReg(reg, e.addrSize, e.getReg(reg, e.addrSize)+delta)
}

// stringOp executes one of the string instructions, honouring the rep prefix.
func (e *Emulator) stringOp(op uint8, size int) {
	delta := uint32(size / 8)
	if e.eflags.isEnable(DirectionFlag) {
		delta = -delta
	}
	seg := e.dataSeg(DS)
	isCompare := op == 0xA6 || op == 0xA7 || op == 0xAE || op == 0xAF

	body := func() {
		switch op {
		case 0xA4, 0xA5: // movs
			v := e.readSeg(seg, e.getIndex(ESI), size)
			e.writeSeg(ES, e.getIndex(EDI), size, v)
			e.addIndex(ESI, delta)
			e.addIndex(EDI, delta)
		case 0xA6, 0xA7: // cmps
			a := e.readSeg(seg, e.getIndex(ESI), size)
			b := e.readSeg(ES, e.getIndex(EDI), size)
			e.updateFlagsSub(size, a, b, 0)
			e.addIndex(ESI, delta)
			e.addIndex(EDI, delta)
		case 0xAA, 0xAB: // stos
			e.writeSeg(ES, e.getIndex(EDI), size, e.getReg(EAX, size))
			e.addIndex(EDI, delta)
		case 0xAC, 0xAD: // lods
			e.setReg(EAX, size, e.readSeg(seg, e.getIndex(ESI), size))
			e.addIndex(ESI, delta)
		case 0xAE, 0xAF: // scas
			a := e.getReg(EAX, size)
			b := e.readSeg(ES, e.getIndex(EDI), size)
			e.updateFlagsSub(size, a, b, 0)
			e.addIndex(EDI, delta)
		case 0x6C, 0x6D: // ins
			e.checkIOPL()
			v := e.io.in(e.getRegister16(DX), size)
			e.writeSeg(ES, e.getIndex(EDI), size, v)
			e.addIndex(EDI, delta)
		case 0x6E, 0x6F: // outs
			e.checkIOPL()
			v := e.readSeg(seg, e.getIndex(ESI), size)
			e.io.out(e.getRegister16(DX), size, v)
			e.addIndex(ESI, delta)
		}
	}

	if e.repPrefix == 0 {
		body()
		return
	}

	for {
		count := e.getReg(ECX, e.addrSize)
		if count == 0 {
			break
		}
		body()
		e.setReg(ECX, e.addrSize, count-1)
		if !isCompare {
			continue
		}
		if e.repPrefix == 0xF3 && !e.eflags.isEnable(ZeroFlag) {
			break
		}
		if e.repPrefix == 0xF2 && e.eflags.isEnable(ZeroFlag) {
			break
		}
	}
}
