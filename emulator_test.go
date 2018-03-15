package main

import (
	"testing"
)

func TestAddJmp(t *testing.T) {
	e := run(t, "guest/addjmp.bin")
	assetRegister32(t, e, "EAX", EAX, 0x0029)
	assetRegister32(t, e, "ECX", ECX, 0x0000)
	assetRegister32(t, e, "EDX", EDX, 0x0000)
	assetRegister32(t, e, "EBX", EBX, 0x0000)
	assetRegister32(t, e, "ESI", ESI, 0x0000)
	assetRegister32(t, e, "EDI", EDI, 0x0000)
	assetRegister32(t, e, "EBP", EBP, 0x0000)
}

func TestCall(t *testing.T) {
	e := run(t, "guest/call-test.bin")
	assetRegister32(t, e, "EAX", EAX, 0x00f1)
	assetRegister32(t, e, "ECX", ECX, 0x011a)
	assetRegister32(t, e, "EDX", EDX, 0x0000)
	assetRegister32(t, e, "EBX", EBX, 0x0029)
	assetRegister32(t, e, "ESI", ESI, 0x0000)
	assetRegister32(t, e, "EDI", EDI, 0x0000)
	assetRegister32(t, e, "EBP", EBP, 0x0000)
}

// func TestInc(t *testing.T) {
// 	e := run(t, "guest/inc.bin")
// 	assetRegister32(t, e, "EAX", EAX, 0x0000)
// 	assetRegister32(t, e, "ECX", ECX, 0x0000)
// 	assetRegister32(t, e, "EDX", EDX, 0x0000)
// 	assetRegister32(t, e, "EBX", EBX, 0x0000)
// 	assetRegister32(t, e, "ESI", ESI, 0x0000)
// 	assetRegister32(t, e, "EDI", EDI, 0x0000)
// 	assetRegister32(t, e, "EBP", EBP, 0x0000)
// }

func TestModRM(t *testing.T) {
	e := run(t, "guest/modrm-test.bin")
	assetRegister32(t, e, "EAX", EAX, 0x0002)
	assetRegister32(t, e, "ECX", ECX, 0x0000)
	assetRegister32(t, e, "EDX", EDX, 0x0000)
	assetRegister32(t, e, "EBX", EBX, 0x0000)
	assetRegister32(t, e, "ESI", ESI, 0x0007)
	assetRegister32(t, e, "EDI", EDI, 0x0008)
	assetRegister32(t, e, "EBP", EBP, 0x7be0)
}

func Test132(t *testing.T) {
	e := run(t, "guest/test132.bin")
	assetRegister32(t, e, "EAX", EAX, 0x0003)
	assetRegister32(t, e, "ECX", ECX, 0x0000)
	assetRegister32(t, e, "EDX", EDX, 0x0000)
	assetRegister32(t, e, "EBX", EBX, 0x0000)
	assetRegister32(t, e, "ESI", ESI, 0x0000)
	assetRegister32(t, e, "EDI", EDI, 0x0000)
	assetRegister32(t, e, "EBP", EBP, 0x0000)
}

func Test133(t *testing.T) {
	e := run(t, "guest/test133.bin")
	assetRegister32(t, e, "EAX", EAX, 0x0037)
	assetRegister32(t, e, "ECX", ECX, 0x0000)
	assetRegister32(t, e, "EDX", EDX, 0x0000)
	assetRegister32(t, e, "EBX", EBX, 0x0000)
	assetRegister32(t, e, "ESI", ESI, 0x0000)
	assetRegister32(t, e, "EDI", EDI, 0x0000)
	assetRegister32(t, e, "EBP", EBP, 0x0000)
}

func Test141(t *testing.T) {
	e := run(t, "guest/test141.bin")
	assetRegister32(t, e, "EAX", EAX, 0x000a)
	assetRegister32(t, e, "ECX", ECX, 0x0000)
	assetRegister32(t, e, "EDX", EDX, 0x03f8)
	assetRegister32(t, e, "EBX", EBX, 0x0000)
	assetRegister32(t, e, "ESI", ESI, 0x0000)
	assetRegister32(t, e, "EDI", EDI, 0x0000)
	assetRegister32(t, e, "EBP", EBP, 0x0000)
}

func run(t *testing.T, filename string) *Emulator {
	bytes, err := loadFile(filename)
	if err != nil {
		t.Fatal(err.Error())
	}
	e := NewEmulator(0x7c00+0x10000, 0x7c00, 0x7c00, true, true, map[uint64]string{})
	e.cr[0] = 1 // 32bit mode
	for i := 0; i < len(bytes); i++ {
		e.memory[i+0x7c00] = bytes[i]
	}
	for e.eip < 0x7c00+0x10000 {
		err := e.execInst()
		if err != nil {
			t.Fatal(err.Error())
		}
		if e.eip == 0 || e.eip == 0x7c00 {
			break
		}
	}
	return e
}

func assetRegister32(t *testing.T, e *Emulator, name string, index uint8, expected uint32) {
	if e.getRegister32(index) != expected {
		t.Fatalf("Bad %s, expected=%08x, actual=%08x\n",
			name, expected, e.getRegister32(index))
	}
}
