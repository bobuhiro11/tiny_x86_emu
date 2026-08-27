package main

import "math/bits"

func (ef *Eflags) setSZP(size int, result uint32) {
	ef.setVal(ZeroFlag, result&szMask(size) == 0)
	ef.setVal(SignFlag, result&szSign(size) != 0)
	ef.setVal(ParityFlag, bits.OnesCount8(uint8(result))%2 == 0)
}

// updateFlagsLogic sets the flags after AND/OR/XOR/TEST.
func (e *Emulator) updateFlagsLogic(size int, result uint32) {
	e.eflags.unset(CarryFlag)
	e.eflags.unset(OverflowFlag)
	e.eflags.unset(AdjustFlag)
	e.eflags.setSZP(size, result)
}

func (e *Emulator) updateFlagsAdd(size int, a, b, carryIn uint32) uint32 {
	mask := szMask(size)
	sign := szSign(size)
	full := uint64(a&mask) + uint64(b&mask) + uint64(carryIn)
	result := uint32(full) & mask
	e.eflags.setVal(CarryFlag, full > uint64(mask))
	e.eflags.setVal(OverflowFlag, (^(a^b)&(a^result))&sign != 0)
	e.eflags.setVal(AdjustFlag, (a^b^result)&0x10 != 0)
	e.eflags.setSZP(size, result)
	return result
}

func (e *Emulator) updateFlagsSub(size int, a, b, borrowIn uint32) uint32 {
	mask := szMask(size)
	sign := szSign(size)
	full := uint64(a&mask) - uint64(b&mask) - uint64(borrowIn)
	result := uint32(full) & mask
	e.eflags.setVal(CarryFlag, full>>63 != 0)
	e.eflags.setVal(OverflowFlag, ((a^b)&(a^result))&sign != 0)
	e.eflags.setVal(AdjustFlag, (a^b^result)&0x10 != 0)
	e.eflags.setSZP(size, result)
	return result
}

// alu executes one of the eight basic arithmetic/logic operations.
// index: 0=add 1=or 2=adc 3=sbb 4=and 5=sub 6=xor 7=cmp
func (e *Emulator) alu(index int, size int, a, b uint32) (uint32, bool) {
	carry := uint32(0)
	if e.eflags.isEnable(CarryFlag) {
		carry = 1
	}
	switch index {
	case 0:
		return e.updateFlagsAdd(size, a, b, 0), true
	case 1:
		r := (a | b) & szMask(size)
		e.updateFlagsLogic(size, r)
		return r, true
	case 2:
		return e.updateFlagsAdd(size, a, b, carry), true
	case 3:
		return e.updateFlagsSub(size, a, b, carry), true
	case 4:
		r := (a & b) & szMask(size)
		e.updateFlagsLogic(size, r)
		return r, true
	case 5:
		return e.updateFlagsSub(size, a, b, 0), true
	case 6:
		r := (a ^ b) & szMask(size)
		e.updateFlagsLogic(size, r)
		return r, true
	default: // cmp
		e.updateFlagsSub(size, a, b, 0)
		return 0, false
	}
}

func (e *Emulator) inc(size int, value uint32) uint32 {
	carry := e.eflags.isEnable(CarryFlag)
	r := e.updateFlagsAdd(size, value, 1, 0)
	e.eflags.setVal(CarryFlag, carry)
	return r
}

func (e *Emulator) dec(size int, value uint32) uint32 {
	carry := e.eflags.isEnable(CarryFlag)
	r := e.updateFlagsSub(size, value, 1, 0)
	e.eflags.setVal(CarryFlag, carry)
	return r
}

// shift executes one of the rotate/shift operations of group 2.
// index: 0=rol 1=ror 2=rcl 3=rcr 4=shl 5=shr 6=sal 7=sar
func (e *Emulator) shift(index int, size int, value, count uint32) (uint32, bool) {
	mask := szMask(size)
	sign := szSign(size)
	value &= mask
	count &= 0x1F
	if index == 2 || index == 3 {
		// rcl/rcr rotate through the carry flag
		switch size {
		case 8:
			count %= 9
		case 16:
			count %= 17
		}
	}
	if count == 0 {
		return value, false
	}

	carry := uint32(0)
	if e.eflags.isEnable(CarryFlag) {
		carry = 1
	}
	result := value

	switch index {
	case 0: // rol
		n := count % uint32(size)
		result = ((value << n) | (value >> (uint32(size) - n))) & mask
		if n == 0 {
			result = value
		}
		e.eflags.setVal(CarryFlag, result&0x1 != 0)
		e.eflags.setVal(OverflowFlag, (result&sign != 0) != (result&0x1 != 0))
	case 1: // ror
		n := count % uint32(size)
		result = ((value >> n) | (value << (uint32(size) - n))) & mask
		if n == 0 {
			result = value
		}
		e.eflags.setVal(CarryFlag, result&sign != 0)
		msb := result & sign
		msb2 := result & (sign >> 1)
		e.eflags.setVal(OverflowFlag, (msb != 0) != (msb2 != 0))
	case 2: // rcl
		for i := uint32(0); i < count; i++ {
			top := (result & sign) >> uint(size-1)
			result = ((result << 1) & mask) | carry
			carry = top
		}
		e.eflags.setVal(CarryFlag, carry != 0)
		e.eflags.setVal(OverflowFlag, (result&sign != 0) != (carry != 0))
	case 3: // rcr
		for i := uint32(0); i < count; i++ {
			bit := result & 0x1
			result = (result >> 1) | (carry << uint(size-1))
			carry = bit
		}
		e.eflags.setVal(CarryFlag, carry != 0)
		e.eflags.setVal(OverflowFlag, (result&sign != 0) != (result&(sign>>1) != 0))
	case 4, 6: // shl/sal
		if count <= uint32(size) {
			e.eflags.setVal(CarryFlag, (value<<(count-1))&sign != 0)
		} else {
			e.eflags.unset(CarryFlag)
		}
		result = (value << count) & mask
		e.eflags.setVal(OverflowFlag, (result&sign != 0) != e.eflags.isEnable(CarryFlag))
		e.eflags.setSZP(size, result)
	case 5: // shr
		if count <= uint32(size) {
			e.eflags.setVal(CarryFlag, (value>>(count-1))&0x1 != 0)
		} else {
			e.eflags.unset(CarryFlag)
		}
		result = (value >> count) & mask
		e.eflags.setVal(OverflowFlag, value&sign != 0)
		e.eflags.setSZP(size, result)
	case 7: // sar
		sv := int32(signExtend(value, size))
		n := count
		if n > uint32(size)-1 {
			n = uint32(size) - 1
		}
		e.eflags.setVal(CarryFlag, (sv>>(n))&0x1 != 0)
		if count >= uint32(size) {
			e.eflags.setVal(CarryFlag, sv < 0)
		}
		result = uint32(sv>>count) & mask
		if count >= uint32(size) {
			if sv < 0 {
				result = mask
			} else {
				result = 0
			}
		}
		e.eflags.unset(OverflowFlag)
		e.eflags.setSZP(size, result)
	}
	return result, true
}

func (e *Emulator) mulUnsigned(size int, value uint32) {
	switch size {
	case 8:
		r := uint16(e.getRegister8(AL)) * uint16(value)
		e.setRegister16(AX, r)
		e.eflags.setVal(CarryFlag, r>>8 != 0)
		e.eflags.setVal(OverflowFlag, r>>8 != 0)
	case 16:
		r := uint32(e.getRegister16(AX)) * (value & 0xFFFF)
		e.setRegister16(AX, uint16(r))
		e.setRegister16(DX, uint16(r>>16))
		e.eflags.setVal(CarryFlag, r>>16 != 0)
		e.eflags.setVal(OverflowFlag, r>>16 != 0)
	default:
		r := uint64(e.registers[EAX]) * uint64(value)
		e.registers[EAX] = uint32(r)
		e.registers[EDX] = uint32(r >> 32)
		e.eflags.setVal(CarryFlag, r>>32 != 0)
		e.eflags.setVal(OverflowFlag, r>>32 != 0)
	}
}

func (e *Emulator) mulSigned(size int, value uint32) {
	switch size {
	case 8:
		r := int16(int8(e.getRegister8(AL))) * int16(int8(value))
		e.setRegister16(AX, uint16(r))
		ok := r == int16(int8(r))
		e.eflags.setVal(CarryFlag, !ok)
		e.eflags.setVal(OverflowFlag, !ok)
	case 16:
		r := int32(int16(e.getRegister16(AX))) * int32(int16(value))
		e.setRegister16(AX, uint16(r))
		e.setRegister16(DX, uint16(r>>16))
		ok := r == int32(int16(r))
		e.eflags.setVal(CarryFlag, !ok)
		e.eflags.setVal(OverflowFlag, !ok)
	default:
		r := int64(int32(e.registers[EAX])) * int64(int32(value))
		e.registers[EAX] = uint32(r)
		e.registers[EDX] = uint32(uint64(r) >> 32)
		ok := r == int64(int32(r))
		e.eflags.setVal(CarryFlag, !ok)
		e.eflags.setVal(OverflowFlag, !ok)
	}
}

// imul2 multiplies two signed operands and returns the truncated result.
func (e *Emulator) imul2(size int, a, b uint32) uint32 {
	r := int64(int32(signExtend(a, size))) * int64(int32(signExtend(b, size)))
	result := uint32(r) & szMask(size)
	ok := r == int64(int32(signExtend(result, size)))
	e.eflags.setVal(CarryFlag, !ok)
	e.eflags.setVal(OverflowFlag, !ok)
	e.eflags.setSZP(size, result)
	return result
}

func (e *Emulator) divUnsigned(size int, value uint32) {
	if value == 0 {
		throwException(ExDivideError, 0, false)
	}
	switch size {
	case 8:
		x := uint32(e.getRegister16(AX))
		q := x / value
		if q > 0xFF {
			throwException(ExDivideError, 0, false)
		}
		e.setRegister8(AL, uint8(q))
		e.setRegister8(AH, uint8(x%value))
	case 16:
		x := (uint32(e.getRegister16(DX)) << 16) | uint32(e.getRegister16(AX))
		q := x / value
		if q > 0xFFFF {
			throwException(ExDivideError, 0, false)
		}
		e.setRegister16(AX, uint16(q))
		e.setRegister16(DX, uint16(x%value))
	default:
		x := (uint64(e.registers[EDX]) << 32) | uint64(e.registers[EAX])
		q := x / uint64(value)
		if q > 0xFFFFFFFF {
			throwException(ExDivideError, 0, false)
		}
		e.registers[EAX] = uint32(q)
		e.registers[EDX] = uint32(x % uint64(value))
	}
}

func (e *Emulator) divSigned(size int, value uint32) {
	if value == 0 {
		throwException(ExDivideError, 0, false)
	}
	switch size {
	case 8:
		x := int32(int16(e.getRegister16(AX)))
		d := int32(int8(value))
		q := x / d
		if q > 127 || q < -128 {
			throwException(ExDivideError, 0, false)
		}
		e.setRegister8(AL, uint8(q))
		e.setRegister8(AH, uint8(x%d))
	case 16:
		x := int32((uint32(e.getRegister16(DX)) << 16) | uint32(e.getRegister16(AX)))
		d := int32(int16(value))
		q := x / d
		if q > 32767 || q < -32768 {
			throwException(ExDivideError, 0, false)
		}
		e.setRegister16(AX, uint16(q))
		e.setRegister16(DX, uint16(x%d))
	default:
		x := int64((uint64(e.registers[EDX]) << 32) | uint64(e.registers[EAX]))
		d := int64(int32(value))
		q := x / d
		if q > 2147483647 || q < -2147483648 {
			throwException(ExDivideError, 0, false)
		}
		e.registers[EAX] = uint32(q)
		e.registers[EDX] = uint32(x % d)
	}
}

// condition evaluates the condition code of a Jcc/SETcc/CMOVcc instruction.
func (e *Emulator) condition(code uint8) bool {
	cf := e.eflags.isEnable(CarryFlag)
	zf := e.eflags.isEnable(ZeroFlag)
	sf := e.eflags.isEnable(SignFlag)
	of := e.eflags.isEnable(OverflowFlag)
	pf := e.eflags.isEnable(ParityFlag)

	switch code >> 1 {
	case 0: // o / no
		return of != (code&1 == 1)
	case 1: // b / ae
		return cf != (code&1 == 1)
	case 2: // e / ne
		return zf != (code&1 == 1)
	case 3: // be / a
		return (cf || zf) != (code&1 == 1)
	case 4: // s / ns
		return sf != (code&1 == 1)
	case 5: // p / np
		return pf != (code&1 == 1)
	case 6: // l / ge
		return (sf != of) != (code&1 == 1)
	default: // le / g
		return (zf || sf != of) != (code&1 == 1)
	}
}
