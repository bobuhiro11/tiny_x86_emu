package main

import (
	"bytes"
	"fmt"
	yaml "gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"testing"
)

type RegisterSet struct {
	Eax    string `yaml:"eax"`
	Ecx    string `yaml:"ecx"`
	Edx    string `yaml:"edx"`
	Ebx    string `yaml:"ebx"`
	Esp    string `yaml:"esp"`
	Ebp    string `yaml:"ebp"`
	Esi    string `yaml:"esi"`
	Edi    string `yaml:"edi"`
	Eip    string `yaml:"eip"`
	Eflags string `yaml:"eflags"`
	Cs     string `yaml:"cs"`
	Ss     string `yaml:"ss"`
	Ds     string `yaml:"ds"`
	Es     string `yaml:"es"`
	Fs     string `yaml:"fs"`
	Gs     string `yaml:"gs"`
}

const (
	NumStep = 70000
)

// return the path of the gdb script
func MakeGdbScript() string {
	f, _ := ioutil.TempFile("", "gdb.script")
	defer f.Close()

	f.Write([]byte(`
	target remote localhost:1234
	set architecture i8086
	set confirm off
	break *0x7c00
	c
	set variable $i = ` + strconv.Itoa(NumStep) + `
	while $i > 0
	si
	info registers
	set variable $i -= 1
	end
	quit
	`))

	return f.Name()
}

// return register values obtained from this emulator
func ExecEmu() []RegisterSet {
	// setup emulator
	reader := &bytes.Buffer{}
	writer := &bytes.Buffer{}
	e := NewEmulator(0x7c00+0x10240000, 0x7c00, 0x6f04, false, true, reader, writer, map[uint64]string{})

	// load file
	bin, err := LoadFile("./xv6-public/xv6.img")
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	for i := 0; i < len(bin); i++ {
		e.memory[uint32(i+0x7c00)] = bin[i]
	}
	e.io.hdds[0], _ = os.Open("./xv6-public/xv6.img")

	// main loop
	var res []RegisterSet
	for i := 0; i < NumStep; i++ {
		err := e.execInst()
		if err != nil {
			panic(err.Error())
		}
		if e.eip == 0 || e.eip == 0x7c00 {
			break
		}
		regSet := RegisterSet{
			Eax: fmt.Sprintf("0x%x", e.getRegister32(EAX)),
			Ecx: fmt.Sprintf("0x%x", e.getRegister32(ECX)),
			Ebx: fmt.Sprintf("0x%x", e.getRegister32(EBX)),
			Edx: fmt.Sprintf("0x%x", e.getRegister32(EDX)),
			Esp: fmt.Sprintf("0x%x", e.getRegister32(ESP)),
			Ebp: fmt.Sprintf("0x%x", e.getRegister32(EBP)),
			Esi: fmt.Sprintf("0x%x", e.getRegister32(ESI)),
			Edi: fmt.Sprintf("0x%x", e.getRegister32(EDI)),
			Eip: fmt.Sprintf("0x%x", e.eip),
		}
		res = append(res, regSet)
	}
	return res
}

// return register values obtained from qemu and gdb
func ExecQemu() []RegisterSet {
	gdbScriptPath := MakeGdbScript()

	qemuCmd := exec.Command("qemu-system-i386",
		"-drive", "file=./xv6-public/fs.img,index=1,media=disk,format=raw", "-drive", "file=./xv6-public/xv6.img,index=0,media=disk,format=raw", "-smp", "2", "-m", "512",
		"-S", "-gdb", "tcp::1234", "-nographic")
	qemuCmd.Start()
	defer func() {
		qemuCmd.Process.Kill()
		qemuCmd.Wait()
	}()
	fmt.Printf("qemu pid=%d\n", qemuCmd.Process.Pid)

	exec.Command("sh", "-c", "ps -aux | grep qemu").Output()
	// fmt.Printf("ps result=%s\n", s)

	gdbOutput, _ := exec.Command("sh", "-c", "gdb -x "+gdbScriptPath+` 2>/dev/null | grep \
	-e "eax\s*0x" \
	-e "ecx\s*0x" \
	-e "edx\s*0x" \
	-e "ebx\s*0x" \
	-e "esp\s*0x" \
	-e "ebp\s*0x" \
	-e "esi\s*0x" \
	-e "edi\s*0x" \
	-e "eip\s*0x" \
	-e "eflags\s*0x" \
	-e "cs\s*0x" \
	-e "ss\s*0x" \
	-e "ds\s*0x" \
	-e "es\s*0x" \
	-e "fs\s*0x" \
	-e "gs\s*0x" \
	| awk '{ if ($1=="eax") print "- " $1 ": " $2; else print "  " $1 ": " $2; }'
	`).Output()
	// fmt.Printf("gdb output=<output>%s</output>\n", gdbOutput)

	file, _ := os.Create(`qemu.log`)
	defer file.Close()

	file.Write(([]byte)(gdbOutput))

	var res []RegisterSet
	err := yaml.Unmarshal([]byte(gdbOutput), &res)
	if err != nil {
		panic(err.Error())
	}

	return res
}

func TestXv6(t *testing.T) {
	QemuRegSet := ExecQemu()
	EmuRegSet := ExecEmu()
	if len(QemuRegSet) != NumStep || len(EmuRegSet) != NumStep {
		t.Fatalf("len(QemuRegSet)=%d len(EmuRegSet)=%d NumStep=%d\n",
			len(QemuRegSet), len(EmuRegSet), NumStep)
	}

	for i := 0; i < NumStep; i++ {
		fmt.Printf("[qemu #%d] eip=%s eax=%s ecx=%s esp=%s edx=%s edi=%s ebp=%s\n",
			i, QemuRegSet[i].Eip, QemuRegSet[i].Eax, QemuRegSet[i].Ecx, QemuRegSet[i].Esp,
			QemuRegSet[i].Edx, QemuRegSet[i].Edi, QemuRegSet[i].Ebp)
		fmt.Printf("[tiny #%d] eip=%s eax=%s ecx=%s esp=%s edx=%s edi=%s ebp=%s\n",
			i, EmuRegSet[i].Eip, EmuRegSet[i].Eax, EmuRegSet[i].Ecx, EmuRegSet[i].Esp,
			EmuRegSet[i].Edx, EmuRegSet[i].Edi, EmuRegSet[i].Ebp)
		if QemuRegSet[i].Eip != EmuRegSet[i].Eip {
			t.Fatalf("bad eip")
		} else if QemuRegSet[i].Eax != EmuRegSet[i].Eax {
			t.Fatalf("bad eax")
		} else if QemuRegSet[i].Ecx != EmuRegSet[i].Ecx {
			t.Fatalf("bad ecx")
		} else if QemuRegSet[i].Edx != EmuRegSet[i].Edx {
			t.Fatalf("bad edx")
		} else if QemuRegSet[i].Ebx != EmuRegSet[i].Ebx {
			t.Fatalf("bad ebx")
		} else if QemuRegSet[i].Esp != EmuRegSet[i].Esp {
			t.Fatalf("bad esp")
		} else if QemuRegSet[i].Ebp != EmuRegSet[i].Ebp {
			t.Fatalf("bad ebp")
		} else if QemuRegSet[i].Esi != EmuRegSet[i].Esi {
			t.Fatalf("bad esi")
		} else if QemuRegSet[i].Edi != EmuRegSet[i].Edi {
			t.Fatalf("bad edi")
		}
		// if QemuRegSet[i].Eax != EmuRegSet[i].Eax {
		// 	t.Fatalf("bad eax: qemu_eip=%s qemu_eax=%s emu_eax=%s\n",
		// 	QemuRegSet[i].Eip, QemuRegSet[i].Eax, EmuRegSet[i].Eax)
		// } else {
		// 	fmt.Printf("correct eax: qemu_eip=%s qemu_eax=%s emu_eax=%s\n",
		// 	QemuRegSet[i].Eip, QemuRegSet[i].Eax, EmuRegSet[i].Eax)
		// }
	}
}
