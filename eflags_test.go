package main

import (
	"testing"
	// "fmt"
)

func TestFlags(t *testing.T) {
	var e Eflags
	v1 := uint32(0x7FFFFFFF)                     // INT_MAX
	v2 := uint32(0xFFFFFFFF)                     // -1
	e.updateBySub(v1, v2, uint64(v1)-uint64(v2)) // overflow
	if !e.isEnable(OverflowFlag) {
		t.Fatalf("Overflow = %v\n", e.isEnable(OverflowFlag))
	}
}
