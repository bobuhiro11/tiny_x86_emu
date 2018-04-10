package main

import (
	"math/bits"
	// "fmt"
	"github.com/fatih/color"
)

// Eflags is a set of flags
type Eflags uint32

// eflags
const (
	CarryFlag     = uint32(1) << 0
	ParityFlag    = uint32(1) << 2
	AdjustFlag    = uint32(1) << 4
	ZeroFlag      = uint32(1) << 6
	SignFlag      = uint32(1) << 7
	TrapFlag      = uint32(1) << 8
	InterruptFlag = uint32(1) << 9
	DirectionFlag = uint32(1) << 10
	OverflowFlag  = uint32(1) << 11
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

	// fmt.Printf("result=0x%08x sign1=%v sign2=%v signr=%v\n", result, sign1, sign2, signr)

	ef.setVal(CarryFlag, (result>>32) != 0)
	ef.setVal(ZeroFlag, result == 0)
	ef.setVal(SignFlag, signr != 0)
	// ef.setVal(OverflowFlag, sign1 != sign2 && sign1 != signr)
	ef.setVal(OverflowFlag,
		(sign1 == 0 && sign2 ==1 && signr == 1) || (sign1 == 1 && sign2 == 0 && signr == 0))
}

func (ef *Eflags) updatePF(result uint8) {
	popcnt := bits.OnesCount8(result)
	ef.setVal(ParityFlag, popcnt%2 == 0)
}

func (ef *Eflags) dump() {
	s := "EFLAGS="
	if ef.isEnable(CarryFlag) {
		s += "CF "
	}
	if ef.isEnable(ParityFlag) {
		s += "PF "
	}
	if ef.isEnable(AdjustFlag) {
		s += "AF "
	}
	if ef.isEnable(ZeroFlag) {
		s += "ZF "
	}
	if ef.isEnable(SignFlag) {
		s += "SF "
	}
	if ef.isEnable(TrapFlag) {
		s += "TF "
	}
	if ef.isEnable(InterruptFlag) {
		s += "IF "
	}
	if ef.isEnable(DirectionFlag) {
		s += "DF "
	}
	if ef.isEnable(OverflowFlag) {
		s += "OF "
	}
	s += "\n"
	color.New(color.FgCyan).Print(s)
}
