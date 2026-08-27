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
		// memset and memcpy of the guest are hot enough to be worth copying
		// whole runs of bytes at once instead of one element at a time
		if n := e.repBlock(op, size, seg, count); n > 0 {
			e.setReg(ECX, e.addrSize, count-n)
			continue
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

// repBlock executes as many iterations of a rep movs/rep stos as fit into the
// physical pages the source and the destination currently live in. It returns
// the number of iterations it did, or 0 when the fast path does not apply and
// the caller has to fall back to a single generic iteration.
func (e *Emulator) repBlock(op uint8, size int, seg int, count uint32) uint32 {
	if e.addrSize != 32 || e.eflags.isEnable(DirectionFlag) {
		return 0
	}
	step := uint32(size / 8)
	user := e.cpl == 3

	dst := e.sreg[ES].base + e.registers[EDI]
	n := (0x1000 - dst&0xFFF) / step
	if n > count {
		n = count
	}
	if n == 0 {
		return 0
	}

	switch op {
	case 0xAA, 0xAB: // stos
		phys := e.translate(dst, true, user)
		buf := e.ram(phys, n*step)
		if buf == nil {
			return 0
		}
		value := e.getReg(EAX, size)
		for i := uint32(0); i < step; i++ {
			buf[i] = uint8(value >> (8 * i))
		}
		// grow the pattern by doubling it, which lets the runtime move whole
		// blocks instead of single bytes
		for filled := step; filled < uint32(len(buf)); filled *= 2 {
			copy(buf[filled:], buf[:filled])
		}
		e.registers[EDI] += n * step
		return n

	case 0xA4, 0xA5: // movs
		src := e.sreg[seg].base + e.registers[ESI]
		if m := (0x1000 - src&0xFFF) / step; m < n {
			n = m
		}
		if n == 0 {
			return 0
		}
		physTo := e.translate(dst, true, user)
		physFrom := e.translate(src, false, user)
		if physTo > physFrom && physTo < physFrom+n*step {
			// the copy propagates bytes forward, which copy() does not do
			return 0
		}
		to := e.ram(physTo, n*step)
		from := e.ram(physFrom, n*step)
		if to == nil || from == nil {
			return 0
		}
		copy(to, from)
		e.registers[EDI] += n * step
		e.registers[ESI] += n * step
		return n
	}
	return 0
}

// ram returns a slice of the physical memory, or nil if the range is not
// backed by memory (a memory mapped device, for example).
func (e *Emulator) ram(phys, length uint32) []uint8 {
	if phys >= DEVSPACE || uint64(phys)+uint64(length) > uint64(len(e.memory)) {
		return nil
	}
	return e.memory[phys : phys+length : phys+length]
}
