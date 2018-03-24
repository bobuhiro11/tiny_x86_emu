package main

import (
	"math/bits"
)

// Eflags is a set of flags
type Eflags uint32

// eflags
const (
	CARRY    = uint32(1) << 0
	PF       = uint32(1) << 2
	ZERO     = uint32(1) << 6
	SIGN     = uint32(1) << 7
	IF       = uint32(1) << 9 // interrupt enable flag
	OVERFLOW = uint32(1) << 11
)

func (ef *Eflags) setVal(flag uint32, value bool) {
	if value {
		ef.set(flag)
	} else {
		ef.unset(flag)
	}
}

func (ef *Eflags) set(flag uint32) {
	*ef = Eflags(uint32(*ef) | flag)
}

func (ef *Eflags) unset(flag uint32) {
	*ef = Eflags(uint32(*ef) & ^flag)
}

func (ef *Eflags) isEnable(flag uint32) bool {
	return uint32(*ef) & flag == flag 
}

func (ef *Eflags) updateBySub(v1, v2 uint32, result uint64) {
	sign1 := (v1 >> 31) & 0x01
	sign2 := (v2 >> 31) & 0x01
	signr := uint32((result >> 31) & 0x01)

	ef.setVal(CARRY, result>>32 != 0)
	ef.setVal(ZERO, result == 0)
	ef.setVal(SIGN, signr != 0)
	ef.setVal(OVERFLOW, sign1 != sign2 && sign1 != signr)
}

func (ef *Eflags) updatePF(result uint8) {
	popcnt := bits.OnesCount8(result)
	if popcnt%2 == 0 {
		ef.set(PF)
	} else {
		ef.unset(PF)
	}
}
