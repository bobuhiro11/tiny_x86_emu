package main

import "math"

// The x87 floating point unit. The registers are stored in the 80bit format
// of the real thing, but every computation is done with float64: that is
// precise enough for the guests the emulator runs and it keeps the code
// short.
type x87State struct {
	st  [8]x87Reg
	tag [8]bool // true when the register holds a value
	top int
	cw  uint16 // control word
	sw  uint16 // status word: only the condition codes are interesting
}

// x87Reg is one register of the stack in the 80bit format the fpu really
// uses. Keeping the raw bits (instead of a float64) matters because the MMX
// registers are the same registers, and because the kernel saves and restores
// them on every context switch.
type x87Reg struct {
	lo uint64 // the mantissa
	hi uint16 // the sign and the exponent
}

// status word bits
const (
	x87C0 = uint16(1) << 8
	x87C1 = uint16(1) << 9
	x87C2 = uint16(1) << 10
	x87C3 = uint16(1) << 14
)

func (f *x87State) init() {
	f.st = [8]x87Reg{}
	f.tag = [8]bool{}
	f.top = 0
	f.cw = 0x037F
	f.sw = 0
}

// status returns the status word with the current stack top filled in.
func (f *x87State) status() uint16 {
	return (f.sw &^ 0x3800) | uint16(f.top&0x7)<<11
}

// tagWord returns the tag word: two bits per physical register, 3 meaning
// empty and 0 meaning "holds a valid number".
func (f *x87State) tagWord() uint16 {
	tw := uint16(0)
	for i := 0; i < 8; i++ {
		if !f.tag[i] {
			tw |= 3 << uint(2*i)
		}
	}
	return tw
}

func (f *x87State) index(i int) int { return (f.top + i) & 7 }

func (f *x87State) get(i int) float64 { return f.st[f.index(i)].float() }

func (f *x87State) set(i int, v float64) {
	f.st[f.index(i)] = newX87Reg(v)
	f.tag[f.index(i)] = true
}

func (f *x87State) push(v float64) {
	f.top = (f.top - 1) & 7
	f.st[f.top] = newX87Reg(v)
	f.tag[f.top] = true
}

func (f *x87State) pop() float64 {
	v := f.st[f.top].float()
	f.tag[f.top] = false
	f.top = (f.top + 1) & 7
	return v
}

// mmx returns the MMX register with the given number. MMX registers are the
// mantissas of the fpu registers, without the stack rotation.
func (f *x87State) mmx(i int) uint64 { return f.st[i&7].lo }

func (f *x87State) setMMX(i int, v uint64) {
	f.st[i&7] = x87Reg{lo: v, hi: 0xFFFF}
	f.tag[i&7] = true
}

// float converts the 80bit extended format into a float64.
func (r x87Reg) float() float64 {
	exp := int(r.hi & 0x7FFF)
	sign := 1.0
	if r.hi&0x8000 != 0 {
		sign = -1.0
	}
	switch {
	case exp == 0 && r.lo == 0:
		return sign * 0
	case exp == 0x7FFF:
		if r.lo<<1 != 0 {
			return math.NaN()
		}
		return math.Inf(int(sign))
	}
	return sign * math.Ldexp(float64(r.lo), exp-16383-63)
}

// newX87Reg converts a float64 into the 80bit extended format.
func newX87Reg(v float64) x87Reg {
	var r x87Reg
	if math.Signbit(v) {
		r.hi = 0x8000
		v = -v
	}
	switch {
	case v == 0:
	case math.IsNaN(v):
		r.hi |= 0x7FFF
		r.lo = 0xC000000000000000
	case math.IsInf(v, 1):
		r.hi |= 0x7FFF
		r.lo = 0x8000000000000000
	default:
		frac, exp := math.Frexp(v) // frac is in [0.5, 1)
		r.lo = uint64(math.Ldexp(frac, 64))
		r.hi |= uint16(exp - 1 + 16383)
	}
	return r
}

// compare sets the condition codes the way FCOM does.
func (f *x87State) compare(a, b float64) {
	f.sw &^= x87C0 | x87C2 | x87C3
	switch {
	case math.IsNaN(a) || math.IsNaN(b):
		f.sw |= x87C0 | x87C2 | x87C3
	case a > b:
	case a < b:
		f.sw |= x87C0
	default:
		f.sw |= x87C3
	}
}

// compareEflags is what FCOMI and FUCOMI do: the result goes into the status
// register of the cpu instead of the condition codes of the fpu.
func (e *Emulator) compareEflags(a, b float64) {
	e.eflags.unset(ZeroFlag | ParityFlag | CarryFlag)
	switch {
	case math.IsNaN(a) || math.IsNaN(b):
		e.eflags.set(ZeroFlag | ParityFlag | CarryFlag)
	case a > b:
	case a < b:
		e.eflags.set(CarryFlag)
	default:
		e.eflags.set(ZeroFlag)
	}
}

// arith performs one of the eight arithmetic operations of the fpu. index is
// the reg field of the opcode: add, mul, com, comp, sub, subr, div, divr.
func (e *Emulator) x87Arith(index int, dst int, a, b float64, pop bool) {
	f := &e.fpu
	var r float64
	switch index {
	case 0:
		r = a + b
	case 1:
		r = a * b
	case 2, 3: // fcom, fcomp
		f.compare(a, b)
		if index == 3 || pop {
			f.pop()
		}
		return
	case 4:
		r = a - b
	case 5:
		r = b - a
	case 6:
		r = a / b
	case 7:
		r = b / a
	}
	f.set(dst, r)
	if pop {
		f.pop()
	}
}

// executeX87 handles the D8-DF escape opcodes.
func (e *Emulator) executeX87(op uint8) {
	f := &e.fpu
	m := e.parseModRM()

	if m.isMem {
		e.x87Memory(op, m)
		return
	}

	modrm := 0xC0 | m.reg<<3 | m.rm
	i := int(m.rm)

	switch op {
	case 0xD8: // arithmetic with st(i), result in st(0)
		e.x87Arith(int(m.reg), 0, f.get(0), f.get(i), false)

	case 0xD9:
		switch {
		case modrm >= 0xC0 && modrm <= 0xC7: // fld st(i)
			f.push(f.get(i))
		case modrm >= 0xC8 && modrm <= 0xCF: // fxch
			v := f.get(0)
			f.set(0, f.get(i))
			f.set(i, v)
		case modrm == 0xD0: // fnop
		case modrm == 0xE0: // fchs
			f.set(0, -f.get(0))
		case modrm == 0xE1: // fabs
			f.set(0, math.Abs(f.get(0)))
		case modrm == 0xE4: // ftst
			f.compare(f.get(0), 0)
		case modrm == 0xE5: // fxam
			f.sw &^= x87C0 | x87C1 | x87C2 | x87C3
			v := f.get(0)
			if math.Signbit(v) {
				f.sw |= x87C1
			}
			switch {
			case !f.tag[f.index(0)]:
				f.sw |= x87C0 | x87C3
			case math.IsNaN(v):
				f.sw |= x87C0
			case math.IsInf(v, 0):
				f.sw |= x87C0 | x87C2
			case v == 0:
				f.sw |= x87C3
			default:
				f.sw |= x87C2
			}
		case modrm == 0xE8:
			f.push(1)
		case modrm == 0xE9:
			f.push(math.Log2(10))
		case modrm == 0xEA:
			f.push(math.Log2(math.E))
		case modrm == 0xEB:
			f.push(math.Pi)
		case modrm == 0xEC:
			f.push(math.Log10(2))
		case modrm == 0xED:
			f.push(math.Ln2)
		case modrm == 0xEE:
			f.push(0)
		case modrm == 0xF0: // f2xm1
			f.set(0, math.Exp2(f.get(0))-1)
		case modrm == 0xF1: // fyl2x
			v := f.get(1) * math.Log2(f.get(0))
			f.pop()
			f.set(0, v)
		case modrm == 0xF2: // fptan
			f.set(0, math.Tan(f.get(0)))
			f.push(1)
			f.sw &^= x87C2
		case modrm == 0xF3: // fpatan
			v := math.Atan2(f.get(1), f.get(0))
			f.pop()
			f.set(0, v)
		case modrm == 0xF8: // fprem
			f.set(0, math.Mod(f.get(0), f.get(1)))
			f.sw &^= x87C2
		case modrm == 0xFA: // fsqrt
			f.set(0, math.Sqrt(f.get(0)))
		case modrm == 0xFC: // frndint
			f.set(0, x87Round(f.get(0), f.cw))
		case modrm == 0xFD: // fscale
			f.set(0, f.get(0)*math.Exp2(math.Trunc(f.get(1))))
		case modrm == 0xFE: // fsin
			f.set(0, math.Sin(f.get(0)))
		case modrm == 0xFF: // fcos
			f.set(0, math.Cos(f.get(0)))
		default:
			abort("x87 instruction 0xd9 0x%02x is not implemented", modrm)
		}

	case 0xDA:
		if modrm == 0xE9 { // fucompp
			f.compare(f.get(0), f.get(1))
			f.pop()
			f.pop()
			return
		}
		abort("x87 instruction 0xda 0x%02x is not implemented", modrm)

	case 0xDB:
		switch {
		case modrm == 0xE2: // fnclex
			f.sw &^= 0x80FF
		case modrm == 0xE3: // fninit
			f.init()
		case modrm >= 0xE8 && modrm <= 0xF7: // fucomi, fcomi
			e.compareEflags(f.get(0), f.get(i))
		default:
			abort("x87 instruction 0xdb 0x%02x is not implemented", modrm)
		}

	case 0xDC: // arithmetic with st(0), result in st(i)
		e.x87Arith(int(m.reg), i, f.get(i), f.get(0), false)

	case 0xDD:
		switch {
		case modrm >= 0xC0 && modrm <= 0xC7: // ffree
			f.tag[f.index(i)] = false
		case modrm >= 0xD0 && modrm <= 0xD7: // fst st(i)
			f.set(i, f.get(0))
		case modrm >= 0xD8 && modrm <= 0xDF: // fstp st(i)
			f.set(i, f.get(0))
			f.pop()
		case modrm >= 0xE0 && modrm <= 0xE7: // fucom
			f.compare(f.get(0), f.get(i))
		case modrm >= 0xE8 && modrm <= 0xEF: // fucomp
			f.compare(f.get(0), f.get(i))
			f.pop()
		default:
			abort("x87 instruction 0xdd 0x%02x is not implemented", modrm)
		}

	case 0xDE:
		if modrm == 0xD9 { // fcompp
			f.compare(f.get(0), f.get(1))
			f.pop()
			f.pop()
			return
		}
		// the arithmetic forms all pop the stack
		e.x87Arith(int(m.reg), i, f.get(i), f.get(0), true)

	case 0xDF:
		switch {
		case modrm == 0xE0: // fnstsw ax
			e.setRegister16(AX, f.status())
		case modrm >= 0xE8 && modrm <= 0xF7: // fucomip, fcomip
			e.compareEflags(f.get(0), f.get(i))
			f.pop()
		default:
			abort("x87 instruction 0xdf 0x%02x is not implemented", modrm)
		}
	}
}

// x87Memory handles the forms of the escape opcodes which take a memory
// operand.
func (e *Emulator) x87Memory(op uint8, m ModRM) {
	f := &e.fpu
	reg := int(m.reg)

	switch op {
	case 0xD8: // arithmetic with a 32bit float
		e.x87Arith(reg, 0, f.get(0), float64(math.Float32frombits(e.readSeg(m.seg, m.addr, 32))), false)

	case 0xDC: // arithmetic with a 64bit float
		e.x87Arith(reg, 0, f.get(0), math.Float64frombits(e.read64(m)), false)

	case 0xDA: // arithmetic with a 32bit integer
		e.x87Arith(reg, 0, f.get(0), float64(int32(e.readSeg(m.seg, m.addr, 32))), false)

	case 0xDE: // arithmetic with a 16bit integer
		e.x87Arith(reg, 0, f.get(0), float64(int16(e.readSeg(m.seg, m.addr, 16))), false)

	case 0xD9:
		switch reg {
		case 0: // fld m32
			f.push(float64(math.Float32frombits(e.readSeg(m.seg, m.addr, 32))))
		case 2, 3: // fst/fstp m32
			e.writeSeg(m.seg, m.addr, 32, math.Float32bits(float32(f.get(0))))
			if reg == 3 {
				f.pop()
			}
		case 4: // fldenv
			e.loadX87Env(m)
		case 5: // fldcw
			f.cw = uint16(e.readSeg(m.seg, m.addr, 16))
		case 6: // fnstenv
			e.storeX87Env(m)
		case 7: // fnstcw
			e.writeSeg(m.seg, m.addr, 16, uint32(f.cw))
		}

	case 0xDB:
		switch reg {
		case 0: // fild m32
			f.push(float64(int32(e.readSeg(m.seg, m.addr, 32))))
		case 1, 2, 3: // fisttp/fist/fistp m32
			v := f.get(0)
			if reg != 1 {
				v = x87Round(v, f.cw)
			} else {
				v = math.Trunc(v)
			}
			e.writeSeg(m.seg, m.addr, 32, uint32(int32(v)))
			if reg != 2 {
				f.pop()
			}
		case 5: // fld m80
			r := e.readX87Reg(m, 0)
			f.top = (f.top - 1) & 7
			f.st[f.top] = r
			f.tag[f.top] = true
		case 7: // fstp m80
			e.writeX87Reg(m, 0, f.st[f.index(0)])
			f.pop()
		default:
			abort("x87 instruction 0xdb /%d is not implemented", reg)
		}

	case 0xDD:
		switch reg {
		case 0: // fld m64
			f.push(math.Float64frombits(e.read64(m)))
		case 1, 2, 3: // fisttp/fst/fstp m64
			if reg == 1 {
				e.write64(m, uint64(int64(math.Trunc(f.get(0)))))
			} else {
				e.write64(m, math.Float64bits(f.get(0)))
			}
			if reg != 2 {
				f.pop()
			}
		case 4: // frstor
			e.loadX87Env(m)
			for i := 0; i < 8; i++ {
				f.st[(f.top+i)&7] = e.readX87Reg(m, 28+uint32(i)*10)
			}
		case 6: // fnsave
			e.storeX87Env(m)
			for i := 0; i < 8; i++ {
				e.writeX87Reg(m, 28+uint32(i)*10, f.st[(f.top+i)&7])
			}
			f.init()
		case 7: // fnstsw
			e.writeSeg(m.seg, m.addr, 16, uint32(f.status()))
		default:
			abort("x87 instruction 0xdd /%d is not implemented", reg)
		}

	case 0xDF:
		switch reg {
		case 0: // fild m16
			f.push(float64(int16(e.readSeg(m.seg, m.addr, 16))))
		case 2, 3: // fist/fistp m16
			e.writeSeg(m.seg, m.addr, 16, uint32(int16(x87Round(f.get(0), f.cw))))
			if reg == 3 {
				f.pop()
			}
		case 5: // fild m64
			f.push(float64(int64(e.read64(m))))
		case 7: // fistp m64
			e.write64(m, uint64(int64(x87Round(f.get(0), f.cw))))
			f.pop()
		default:
			abort("x87 instruction 0xdf /%d is not implemented", reg)
		}
	}
}

// x87Round rounds according to the rounding mode of the control word.
func x87Round(v float64, cw uint16) float64 {
	switch (cw >> 10) & 0x3 {
	case 1:
		return math.Floor(v)
	case 2:
		return math.Ceil(v)
	case 3:
		return math.Trunc(v)
	default:
		return math.RoundToEven(v)
	}
}

func (e *Emulator) read64(m ModRM) uint64 {
	return uint64(e.readSeg(m.seg, m.addr, 32)) |
		uint64(e.readSeg(m.seg, m.addr+4, 32))<<32
}

func (e *Emulator) write64(m ModRM, v uint64) {
	e.writeSeg(m.seg, m.addr, 32, uint32(v))
	e.writeSeg(m.seg, m.addr+4, 32, uint32(v>>32))
}

// readX87Reg and writeX87Reg move one register in its 80bit form.
func (e *Emulator) readX87Reg(m ModRM, off uint32) x87Reg {
	return x87Reg{
		lo: uint64(e.readSeg(m.seg, m.addr+off, 32)) |
			uint64(e.readSeg(m.seg, m.addr+off+4, 32))<<32,
		hi: uint16(e.readSeg(m.seg, m.addr+off+8, 16)),
	}
}

func (e *Emulator) writeX87Reg(m ModRM, off uint32, r x87Reg) {
	e.writeSeg(m.seg, m.addr+off, 32, uint32(r.lo))
	e.writeSeg(m.seg, m.addr+off+4, 32, uint32(r.lo>>32))
	e.writeSeg(m.seg, m.addr+off+8, 16, uint32(r.hi))
}

// storeX87Env writes the 28 byte 32bit protected mode environment.
func (e *Emulator) storeX87Env(m ModRM) {
	e.writeSeg(m.seg, m.addr, 32, uint32(e.fpu.cw))
	e.writeSeg(m.seg, m.addr+4, 32, uint32(e.fpu.status()))
	e.writeSeg(m.seg, m.addr+8, 32, uint32(e.fpu.tagWord()))
	for i := uint32(12); i < 28; i += 4 {
		e.writeSeg(m.seg, m.addr+i, 32, 0)
	}
}

func (e *Emulator) loadX87Env(m ModRM) {
	f := &e.fpu
	f.cw = uint16(e.readSeg(m.seg, m.addr, 16))
	sw := uint16(e.readSeg(m.seg, m.addr+4, 16))
	f.sw = sw
	f.top = int(sw>>11) & 0x7
	tw := uint16(e.readSeg(m.seg, m.addr+8, 16))
	for i := 0; i < 8; i++ {
		f.tag[i] = (tw>>uint(2*i))&0x3 != 3
	}
}
