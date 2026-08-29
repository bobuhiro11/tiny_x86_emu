package main

import (
	"os"
	"strings"
	"testing"
)

const (
	bzImagePath = "linux/bzImage"
	initrdPath  = "linux/initramfs.cpio"
)

// bootLinuxGuest starts the emulator with the Linux kernel and the u-root
// initramfs built by script/build-linux.sh.
func bootLinuxGuest(t *testing.T) (*Emulator, *console) {
	t.Helper()

	bzImage, err := os.ReadFile(bzImagePath)
	if err != nil {
		t.Skipf("%s is missing, run make linux first: %v", bzImagePath, err)
	}
	initrd, err := os.ReadFile(initrdPath)
	if err != nil {
		t.Skipf("%s is missing, run make linux first: %v", initrdPath, err)
	}

	out := &console{}
	e := NewEmulator(0, 0, false, out, map[uint64]string{})
	if err := LoadLinux(e, bzImage, initrd, DefaultCmdline); err != nil {
		t.Fatal(err)
	}
	return e, out
}

// TestLinuxBoot boots Linux with u-root as its userland, all the way to the
// shell prompt.
func TestLinuxBoot(t *testing.T) {
	if testing.Short() {
		t.Skip("booting Linux takes a minute")
	}
	e, out := bootLinuxGuest(t)
	runUntil(t, e, out, "Welcome to u-root!", 4*1000*1000*1000)

	for _, want := range []string{
		"Linux version",
		"Run /init as init process",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in the boot log:\n%s", want, out.String())
		}
	}
	for _, bad := range []string{"Kernel panic", "BUG:", "cut here"} {
		if strings.Contains(out.String(), bad) {
			t.Errorf("%q shows up in the boot log:\n%s", bad, out.String())
		}
	}
}

// TestLinuxShell types a command into the shell of u-root and waits for its
// output.
func TestLinuxShell(t *testing.T) {
	if testing.Short() {
		t.Skip("booting Linux takes a minute")
	}
	e, out := bootLinuxGuest(t)
	runUntil(t, e, out, "Welcome to u-root!", 4*1000*1000*1000)

	for _, b := range []byte("echo xyzzy\n") {
		e.io.PushInput(b)
	}
	// the word shows up twice: once echoed by the terminal driver while it
	// is typed, and once as the output of echo
	for i := 0; i < 1000*1000*1000; i++ {
		if err := e.execInst(); err != nil {
			t.Fatalf("%v\nconsole:\n%s", err, out.String())
		}
		if i%4096 == 0 && strings.Count(out.String(), "xyzzy") >= 2 {
			return
		}
	}
	t.Fatalf("echo did not run, console:\n%s", out.String())
}
