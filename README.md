# tiny_x86_emu [![wercker status](https://app.wercker.com/status/7ac504b68746c744dd7dc4b5e52e4735/s/master "wercker status")](https://app.wercker.com/project/byKey/7ac504b68746c744dd7dc4b5e52e4735) [![GoDoc](https://godoc.org/github.com/nmi/tiny_x86_emu?status.svg)](https://godoc.org/github.com/nmi/tiny_x86_emu) [![Go Report Card](https://goreportcard.com/badge/github.com/nmi/tiny_x86_emu)](https://goreportcard.com/report/github.com/nmi/tiny_x86_emu) ![](https://img.shields.io/github/license/nmi/tiny_x86_emu.svg)

This is an experimental x86 emulator written in Go. It boots
[xv6](https://github.com/mit-pdos/xv6-public) from an emulated IDE disk, all the
way to an interactive shell. The same emulator also runs in the browser as
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

![screenshot](https://raw.githubusercontent.com/nmi/tiny_x86_emu/master/screenshot.png)

## What is emulated

- 32bit protected mode with segmentation (including the per-cpu `%gs` segment
  xv6 relies on) and 4KB/4MB paging, plus enough real mode to run the boot
  sector.
- The i386 instruction set, ring 0/ring 3 privilege levels, interrupt and trap
  gates, stack switching through the TSS and `iret`, and cpu exceptions such as
  page faults.
- Devices: IDE disks (boot disk and file system disk), 16550 UART, local APIC
  (including its timer), I/O APIC, the CGA cursor registers and the keyboard
  controller ports.
- The MP tables the guest needs to find its (single) cpu and its I/O APIC.

The console of the guest is the serial port: what xv6 writes to COM1 shows up
on the terminal, and what you type is delivered to the guest as a COM1
interrupt.

The machine has 16MB of RAM (see `PHYSTOP` in `xv6-public/memlayout.h` and
`emulator.go`, both have to agree). That is plenty for the shell and the user
programs, but not for the part of `usertests` which grows a process to 100MB.

## Preparation

Please make sure that make, go (>=1.16), gcc, objdump, nasm and ndisasm are
installed. For example, if you are using ubuntu, you can install them using the
following command.

```bash
$ sudo apt-get install -y nasm gcc git tar wget make bsdmainutils golang
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
| `-f`      | boot disk image (default `xv6-public/xv6.img`) |
| `-fs`     | file system disk image (default `xv6-public/fs.img`) |
| `-trace`  | dump every executed instruction, annotated with `objdump` output |
| `-max`    | stop after the given number of instructions |

## Testing

`make test` executes all tests. The interesting ones boot xv6 and type a
command into its shell:

```bash
$ go test -run TestXv6 -v .
```

## Contribution

Pull requests from anyone are welcome!
