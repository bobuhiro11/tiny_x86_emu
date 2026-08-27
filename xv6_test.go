package main

import (
	"bytes"
	"os"
	"strings"
	"sync"
	"testing"
)

// console collects everything the guest writes to the serial port.
type console struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (c *console) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.Write(p)
}

func (c *console) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.String()
}

// bootXv6 starts the emulator with the xv6 images and runs it until the guest
// printed want, or until the instruction budget is exhausted.
func bootXv6(t *testing.T) (*Emulator, *console) {
	t.Helper()

	boot, err := os.Open("xv6-public/xv6.img")
	if err != nil {
		t.Skipf("xv6-public/xv6.img is missing, run make first: %v", err)
	}
	t.Cleanup(func() { boot.Close() })

	fs, err := os.Open("xv6-public/fs.img")
	if err != nil {
		t.Skipf("xv6-public/fs.img is missing, run make first: %v", err)
	}
	t.Cleanup(func() { fs.Close() })

	out := &console{}
	e := NewEmulator(0x7c00, 0x7c00, false, out, map[uint64]string{})

	sector := make([]byte, SectorSize)
	if _, err := boot.ReadAt(sector, 0); err != nil {
		t.Fatal(err)
	}
	copy(e.memory[0x7c00:], sector)

	e.io.hdds[0] = NewDisk(boot)
	// The file system is opened read only: a test must not modify the image.
	e.io.hdds[1] = NewDisk(fs)
	return e, out
}

// runUntil executes instructions until the console output contains want.
func runUntil(t *testing.T, e *Emulator, out *console, want string, budget int) {
	t.Helper()
	for i := 0; i < budget; i++ {
		if err := e.execInst(); err != nil {
			t.Fatalf("%v\nconsole:\n%s", err, out.String())
		}
		if e.shutdown {
			break
		}
		if i%4096 == 0 && strings.Contains(out.String(), want) {
			return
		}
	}
	if !strings.Contains(out.String(), want) {
		t.Fatalf("%q did not show up within %d instructions, console:\n%s",
			want, budget, out.String())
	}
}

// TestXv6Boot boots xv6 up to the shell prompt.
func TestXv6Boot(t *testing.T) {
	e, out := bootXv6(t)
	runUntil(t, e, out, "init: starting sh", 200*1000*1000)

	for _, want := range []string{"xv6...", "cpu0: starting 0", "init: starting sh"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in the boot log:\n%s", want, out.String())
		}
	}
}

// TestXv6Shell types a command into the shell and checks its output.
func TestXv6Shell(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping the shell test in short mode")
	}
	e, out := bootXv6(t)
	runUntil(t, e, out, "init: starting sh", 200*1000*1000)

	for _, b := range []byte("echo xyzzy\n") {
		e.io.PushInput(b)
	}
	// the command shows up twice: once echoed by the console driver while it
	// is typed, and once as the output of the echo program
	for i := 0; i < 200*1000*1000; i++ {
		if err := e.execInst(); err != nil {
			t.Fatalf("%v\nconsole:\n%s", err, out.String())
		}
		if i%4096 == 0 && strings.Count(out.String(), "xyzzy") >= 2 {
			return
		}
	}
	t.Fatalf("echo did not run, console:\n%s", out.String())
}
