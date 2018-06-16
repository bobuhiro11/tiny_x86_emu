package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	yaml "gopkg.in/yaml.v2"
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
	NumStep = 140000
	// NumStep = 2000000 // This is for qemu_xv6.log
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
func ExecEmu() {
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
			Eax:    fmt.Sprintf("0x%x", e.getRegister32(EAX)),
			Ecx:    fmt.Sprintf("0x%x", e.getRegister32(ECX)),
			Ebx:    fmt.Sprintf("0x%x", e.getRegister32(EBX)),
			Edx:    fmt.Sprintf("0x%x", e.getRegister32(EDX)),
			Esp:    fmt.Sprintf("0x%x", e.getRegister32(ESP)),
			Ebp:    fmt.Sprintf("0x%x", e.getRegister32(EBP)),
			Esi:    fmt.Sprintf("0x%x", e.getRegister32(ESI)),
			Edi:    fmt.Sprintf("0x%x", e.getRegister32(EDI)),
			Eip:    fmt.Sprintf("0x%x", e.eip),
			Eflags: fmt.Sprintf("0x%x", e.eflags),
			Cs:     fmt.Sprintf("0x%x", e.sreg[CS]),
			Ss:     fmt.Sprintf("0x%x", e.sreg[SS]),
			Ds:     fmt.Sprintf("0x%x", e.sreg[DS]),
			Es:     fmt.Sprintf("0x%x", e.sreg[ES]),
			Fs:     fmt.Sprintf("0x%x", 0),
			Gs:     fmt.Sprintf("0x%x", e.sreg[GS]),
		}
		res = append(res, regSet)
	}

	// dump to emu_xv6.log
	content, err := yaml.Marshal(&res)
	if err != nil {
		panic(err)
	}
	ioutil.WriteFile("emu_xv6.log", content, os.ModePerm)
}

// return register values obtained from qemu and gdb
func ExecQemu() {
	_, err := os.Stat("qemu_xv6.log")
	if err == nil {
		// Already Exist
		return
	}

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

	file, _ := os.Create(`qemu_xv6.log`)
	defer file.Close()

	file.Write(([]byte)(gdbOutput))

	var res []RegisterSet
	err = yaml.Unmarshal([]byte(gdbOutput), &res)
	if err != nil {
		panic(err.Error())
	}
	// return res
}

func TestXv6(t *testing.T) {
	b := time.Now()
	ExecEmu()
	a := time.Now()
	fmt.Printf("Emu Execution Time is %v\n", a.Sub(b))

	b = time.Now()
	ExecQemu()
	a = time.Now()
	fmt.Printf("Qemu Execution Time is %v\n", a.Sub(b))

	// wcStr, err := exec.Command(`wc`, `-l`, `*.log`).Output()
	// if err != nil {
	// 	t.Error(err)
	// }
	// fmt.Printf("%s", string(wcStr))

	command := `diff <(sed 's/"//g' emu_xv6.log | head -n ` + fmt.Sprintf("%d", NumStep*16) + ` | grep -v eflags) <(head -n ` + fmt.Sprintf("%d", NumStep*16) + ` qemu_xv6.log | grep -v eflags)`
	fmt.Printf("Diff Command is \"%s\"\n", command)
	diffStr, err := exec.Command("bash", "-c", command).Output()
	if err != nil {
		t.Error(err)
	}

	if len(diffStr) > 0 {
		t.Errorf("Register Difference is as below:\n%s", string(diffStr))
	}

	// if len(QemuRegSet) != NumStep || len(EmuRegSet) != NumStep {
	// 	t.Fatalf("len(QemuRegSet)=%d len(EmuRegSet)=%d NumStep=%d\n",
	// 		len(QemuRegSet), len(EmuRegSet), NumStep)
	// }
}
