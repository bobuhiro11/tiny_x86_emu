# tiny_x86_emu [![CI](https://github.com/bobuhiro11/tiny_x86_emu/actions/workflows/ci.yml/badge.svg)](https://github.com/bobuhiro11/tiny_x86_emu/actions/workflows/ci.yml) [![Go Reference](https://pkg.go.dev/badge/github.com/bobuhiro11/tiny_x86_emu.svg)](https://pkg.go.dev/github.com/bobuhiro11/tiny_x86_emu) [![Go Report Card](https://goreportcard.com/badge/github.com/bobuhiro11/tiny_x86_emu)](https://goreportcard.com/report/github.com/bobuhiro11/tiny_x86_emu) ![](https://img.shields.io/github/license/bobuhiro11/tiny_x86_emu.svg)

This is an experimental x86 emulator written in Go. It boots
[xv6](https://github.com/mit-pdos/xv6-public) from an emulated IDE disk and the
latest [Linux](https://kernel.org) kernel with
[u-root](https://github.com/u-root/u-root) as its userland, both all the way to
an interactive shell. The same emulator also runs in the browser as
WebAssembly.

## Demo

https://bobuhiro11.net/tiny_x86_emu/

Click the terminal and type, for example:

```
$ ls
$ cat README
$ echo hello
$ forktest
```

![screenshot](https://raw.githubusercontent.com/bobuhiro11/tiny_x86_emu/master/screenshot.png)

## What is emulated

- 32bit protected mode with segmentation (including the per-cpu `%gs` segment
  xv6 relies on) and 4KB/4MB paging, plus enough real mode to run the boot
  sector.
- The i386 instruction set, ring 0/ring 3 privilege levels, interrupt and trap
  gates, stack switching through the TSS and `iret`, and cpu exceptions such as
  page faults.
- Enough of the fpu for a kernel to initialize it, save it and restore it, the
  arithmetic of the x87 stack (computed with `float64`), and the MMX moves the
  386 runtime of Go uses for its 64bit atomics.
- Devices: IDE disks (boot disk and file system disk), 16550 UART, the two
  8259 interrupt controllers, the 8254 timer, the CMOS clock, local APIC
  (including its timer), I/O APIC, the CGA cursor registers and the keyboard
  controller ports.
- The MP tables the guest needs to find its (single) cpu and its I/O APIC.
- The 32bit boot protocol of Linux, so that a `bzImage` and an initramfs can be
  started without a BIOS or a boot loader.

The emulated cpu runs at one instruction per cycle and the timers are driven by
the instruction count, which makes the machine a ~100MHz Pentium Pro as far as
the guest can tell.

The console of the guest is the serial port: what the guest writes to COM1
shows up on the terminal, and what you type is delivered to the guest as a COM1
interrupt.

The machine has the 224MB of RAM xv6 expects by default (see `PHYSTOP` in
`xv6-public/memlayout.h` and `emulator.go`, both have to agree). The emulator
allocates all of it up front and xv6 clears it while booting, which is most of
the few seconds the boot takes in the browser.

## Preparation

Please make sure that make, go (>=1.21), gcc, objdump, nasm and ndisasm are
installed. For example, if you are using ubuntu, you can install them using the
following command.

```bash
$ sudo apt-get install -y nasm gcc git tar wget make bsdmainutils golang
```

Building the Linux guest needs a few more packages:

```bash
$ sudo apt-get install -y flex bison bc libelf-dev libssl-dev
```

## Usage

`make` builds two versions of the emulator and the xv6 images:

- Emulator for the host OS: `tiny_x86_emu`.
- Emulator for wasm: `wasm/tiny_x86_emu.wasm`. Both disk images are compressed
  and embedded into the binary.
- xv6 images: the makefile of xv6 is executed recursively.

```bash
# Build both versions of the emulator and the guest xv6 images
$ make

# Execute the CLI version of the emulator in your terminal.
# Ctrl-] stops the emulator.
$ ./tiny_x86_emu -f xv6-public/xv6.img -fs xv6-public/fs.img

# Start a web server hosting the wasm file.
# Then, please open http://localhost:8000 in your browser.
$ ./httpserv
```

Useful flags of the CLI version:

| flag | description |
| ---- | ----------- |
| `-f`       | boot disk image (default `xv6-public/xv6.img`) |
| `-fs`      | file system disk image (default `xv6-public/fs.img`) |
| `-bzImage` | Linux kernel to boot instead of the disk image |
| `-initrd`  | initramfs of the Linux guest |
| `-cmdline` | Linux kernel command line |
| `-trace`   | dump every executed instruction, annotated with `objdump` output |
| `-max`     | stop after the given number of instructions |

## Booting Linux

`make linux` clones Linux and u-root next to the emulator and builds the two
images the Linux guest is made of:

- `linux/bzImage`: a 32bit kernel, configured by `linux/tiny_x86_emu.config`.
  It is about as small as a kernel gets while still being useful: a serial
  console, an initramfs, no PCI, no ACPI, no SMP and no local APIC, so that the
  guest uses the 8259 interrupt controllers and the 8254 timer.
- `linux/initramfs.cpio`: the userland, built by u-root. It is compiled for 386
  with `GO386=softfloat`, because the emulator has no SSE.

```bash
# Build the kernel and the initramfs (this takes a few minutes and needs
# flex, bison, libelf and a 32bit capable gcc)
$ make linux

# Boot it. Ctrl-] stops the emulator.
$ ./tiny_x86_emu -bzImage linux/bzImage -initrd linux/initramfs.cpio
```

The shell prompt shows up after about six seconds:

```
[    0.449495] Run /init as init process
2026/08/29 10:30:47 Welcome to u-root!
                              _
   _   _      _ __ ___   ___ | |_
  | | | |____| '__/ _ \ / _ \| __|
  | |_| |____| | | (_) | (_) | |_
   \__,_|    |_|  \___/ \___/ \__|

init: 2026/08/29 10:30:47 Setting console log level to 5...
$ cat /proc/version
Linux version 7.2.0 (gcc 13.3.0, GNU ld 2.42) #2 PREEMPT
$ shutdown -r
[   53.694701] reboot: Restarting system
```

`LINUX_TAG=v6.12 make linux` builds a different version of the kernel, and
`LINUX_SRC` and `UROOT_SRC` point the build at trees which are already on the
disk.

## Testing

`make test` executes all tests. The interesting ones boot a guest and type a
command into its shell:

```bash
$ go test -run TestXv6 -v .
$ go test -run TestLinux -v .
```

`TestLinuxBoot` and `TestLinuxShell` skip themselves while `linux/bzImage` and
`linux/initramfs.cpio` are missing, so `make test` works without building a
kernel first.

The whole test suite which comes with xv6 runs as well. It takes a few
minutes, so it is opt in:

```bash
$ USERTESTS=1 go test -run TestXv6Usertests -v -timeout 30m .
```

## Contribution

Pull requests from anyone are welcome!
