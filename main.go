//go:build !wasm
// +build !wasm

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func printf(format string, a ...interface{}) {
	fmt.Printf(format, a...)
}

// loadSymbols disassembles the kernel so that the instruction trace can show
// where the guest is executing. It is best effort: without objdump the trace
// simply has no symbol names.
func loadSymbols(kernel string) map[uint64]string {
	disasm := map[uint64]string{}
	out, err := exec.Command("sh", "-c",
		"objdump -d "+kernel+` | grep -E "^[ ]*[0-9a-f]+:"`).CombinedOutput()
	if err != nil {
		return disasm
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.SplitN(strings.TrimSpace(line), ":", 2)
		if len(fields) != 2 {
			continue
		}
		ix, err := strconv.ParseUint(fields[0], 16, 64)
		if err != nil {
			continue
		}
		text := strings.TrimSpace(fields[1])
		if i := strings.Index(text, "\t"); i >= 0 {
			text = strings.TrimSpace(text[i:])
		}
		disasm[ix] = text
		disasm[ix-0x80000000] = text
	}
	return disasm
}

// rawMode puts the terminal in raw mode so that the guest sees every key
// press. It returns a function which restores the previous settings.
func rawMode() func() {
	fi, err := os.Stdin.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return func() {}
	}
	saved, err := exec.Command("sh", "-c", "stty -g < /dev/tty").Output()
	if err != nil {
		return func() {}
	}
	if err := exec.Command("sh", "-c", "stty raw -echo < /dev/tty").Run(); err != nil {
		return func() {}
	}
	return func() {
		exec.Command("sh", "-c", "stty "+strings.TrimSpace(string(saved))+" < /dev/tty").Run()
	}
}

func main() {
	filename := flag.String("f", "xv6-public/xv6.img", "boot disk image")
	fsname := flag.String("fs", "xv6-public/fs.img", "file system disk image")
	kernel := flag.String("kernel", "xv6-public/kernel", "kernel binary used for the trace")
	trace := flag.Bool("trace", false, "dump every executed instruction")
	maxInst := flag.Uint64("max", 0, "stop after this many instructions (0: no limit)")
	flag.Parse()

	boot, err := os.Open(*filename)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	defer boot.Close()

	disasm := map[uint64]string{}
	if *trace {
		disasm = loadSymbols(*kernel)
	}

	// The BIOS loads the first sector of the boot disk at 0x7c00 and jumps
	// to it. Everything else is read by the guest through the IDE ports.
	e := NewEmulator(0x7c00, 0x7c00, false, os.Stdout, disasm)
	e.trace = *trace
	bootSector := make([]byte, SectorSize)
	if _, err := boot.ReadAt(bootSector, 0); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	copy(e.memory[0x7c00:], bootSector)

	e.io.hdds[0] = NewDisk(boot)
	if fs, err := os.OpenFile(*fsname, os.O_RDWR, 0); err == nil {
		defer fs.Close()
		e.io.hdds[1] = NewDisk(fs)
	} else {
		fmt.Fprintf(os.Stderr, "warning: %s is not available: %v\n", *fsname, err)
	}

	restore := rawMode()
	defer restore()

	// Feed the console with the keys typed by the user.
	input := make(chan byte, 256)
	go func() {
		buf := make([]byte, 64)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				close(input)
				return
			}
			for _, b := range buf[:n] {
				input <- b
			}
		}
	}()

	for {
		if e.halted {
			// the guest is waiting for an interrupt: give the host cpu a
			// break, one timer tick is delivered per round
			time.Sleep(2 * time.Millisecond)
		}
		if e.instCount&0xFFF == 0 || e.halted {
			select {
			case b, ok := <-input:
				if ok {
					if b == 0x1D { // ctrl-]
						restore()
						printf("\r\nemulator stopped.\r\n")
						return
					}
					e.io.PushInput(b)
				}
			default:
			}
		}

		if err := e.execInst(); err != nil {
			restore()
			printf("\r\n%s\r\n", err.Error())
			e.dump(int(e.instCount))
			os.Exit(1)
		}
		if e.shutdown {
			break
		}
		if *maxInst != 0 && e.instCount > *maxInst {
			restore()
			printf("\r\ninstruction limit reached\r\n")
			e.dump(int(e.instCount))
			return
		}
	}
	restore()
	printf("\r\nEnd of program\r\n")
}

// LoadFile reads a whole file into memory.
func LoadFile(filename string) ([]byte, error) {
	bytes, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}
