package main

import (
	"testing"
	"os/exec"
	"fmt"
	"strconv"
	"io/ioutil"
	yaml "gopkg.in/yaml.v2"
)

type QemuStatus struct {
	Eax      string `yaml:"eax"`
	Ecx      string `yaml:"ecx"`
	Edx      string `yaml:"edx"`
	Ebx      string `yaml:"ebx"`
	Esp      string `yaml:"esp"`
	Ebp      string `yaml:"ebp"`
	Esi      string `yaml:"esi"`
	Edi      string `yaml:"edi"`
	Eip      string `yaml:"eip"`
	Eflags   string `yaml:"eflags"`
	Cs       string `yaml:"cs"`
	Ss       string `yaml:"ss"`
	Ds       string `yaml:"ds"`
	Es       string `yaml:"es"`
	Fs       string `yaml:"fs"`
	Gs       string `yaml:"gs"`
}

const (
	NumStep = 1000
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
info registers
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

func ExecQemu() {
	gdbScriptPath := MakeGdbScript()

	qemuCmd := exec.Command("qemu-system-i386","-hdb","./xv6-public/xv6.img","-S","-gdb","tcp::1234","-nographic")
	qemuCmd.Start()
	fmt.Printf("qemu pid=%d\n", qemuCmd.Process.Pid)

	gdbOutput, _ := exec.Command("sh", "-c", "gdb -x " + gdbScriptPath + ` 2>/dev/null | grep \
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
	fmt.Printf("gdb output=<output>%s</output>\n", gdbOutput)

	// var d YamlData
	var d []QemuStatus
	err := yaml.Unmarshal([]byte(gdbOutput), &d)
	if err != nil {
		panic(err)
	}

	qemuCmd.Process.Kill()
	qemuCmd.Wait()
}

func TestHello(t *testing.T) {
	ExecQemu()
	if false {
		t.Fatalf("TestHello fail.")
	}
}
