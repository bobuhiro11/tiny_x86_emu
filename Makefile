# add -gcflags '-N -l' when you want to debug the emulator itself: it makes
# the emulator about four times slower
GO_BUILD_OPT=
LD_OPT=-m elf_i386 --entry=start --oformat=binary -Ttext 0x7c00
CC= -nostdlib -fno-pie -fno-asynchronous-unwind-tables -g -fno-stack-protector

SRCS=$(shell find . -maxdepth 1 -type f -name '*.go')
PKGS=$(shell go list ./... | grep -v /vendor/)
GUEST_BINARIES=guest/test.bin guest/test132.bin guest/test133.bin \
			   guest/test134.bin guest/test135.bin guest/addjmp.bin \
			   guest/modrm-test.bin guest/call-test.bin guest/test141.bin \
			   guest/test143.bin guest/mbr.bin
GOROOT=$(shell go env GOROOT)

.SUFFIX: .bin

.PHONY: all
all: tiny_x86_emu wasm/tiny_x86_emu.wasm httpserv

.PHONY: test
test: $(GUEST_BINARIES) xv6-public/xv6.img xv6-public/fs.img
	go vet $(PKGS) && go test $(PKGS) -v --cover -timeout 30m

# The Linux guest is not part of "make all": building a kernel takes a few
# minutes and needs the sources, so it is opt in. The tests which boot Linux
# skip themselves while the images are missing.
.PHONY: linux
linux: linux/bzImage linux/initramfs.cpio

linux/bzImage linux/initramfs.cpio: linux/tiny_x86_emu.config script/build-linux.sh
	./script/build-linux.sh

.PHONY: clean
clean:
	make --quiet -C xv6-public/ clean
	rm -f linux/bzImage linux/initramfs.cpio
	rm -f tiny_x86_emu wasm/tiny_x86_emu.wasm wasm/wasm_exec.js \
		wasm/xv6.img.gz wasm/fs.img.gz wasm/bzImage.gz \
		wasm/initramfs.cpio.gz httpserv guest/*.bin guest/*.o
	go clean

.PHONY: xv6-public/xv6.img
xv6-public/xv6.img:
	make --quiet -C ./xv6-public xv6.img

.PHONY: xv6-public/fs.img
xv6-public/fs.img:
	make --quiet -C ./xv6-public fs.img

tiny_x86_emu: $(SRCS)
	go build $(GO_BUILD_OPT) -o $@

# The wasm build has no file system, so both disk images are embedded into the
# binary (compressed: xv6.img is mostly empty).
wasm/xv6.img.gz: xv6-public/xv6.img
	gzip -9 -c xv6-public/xv6.img > $@

wasm/fs.img.gz: xv6-public/fs.img
	gzip -9 -c xv6-public/fs.img > $@

# wasm_exec.js has to match the go toolchain which built the wasm binary
.PHONY: wasm/wasm_exec.js
wasm/wasm_exec.js:
	cp $(GOROOT)/lib/wasm/wasm_exec.js $@ 2>/dev/null || cp $(GOROOT)/misc/wasm/wasm_exec.js $@

wasm/tiny_x86_emu.wasm: $(SRCS) wasm/xv6.img.gz wasm/fs.img.gz wasm/wasm_exec.js
	GOOS=js GOARCH=wasm go build $(GO_BUILD_OPT) -o $@

# The Linux images are far too big to embed, so the page downloads them from
# next to the wasm binary when the guest is switched to Linux.
.PHONY: wasm-linux
wasm-linux: wasm/bzImage.gz wasm/initramfs.cpio.gz

wasm/bzImage.gz: linux/bzImage
	gzip -9 -c $< > $@

wasm/initramfs.cpio.gz: linux/initramfs.cpio
	gzip -9 -c $< > $@

httpserv: script/httpserv.go
	go build -o httpserv ./script/httpserv.go

guest/inc.bin: guest/inc.c
	gcc -Wl,--entry=inc,--oformat=binary $(CC) -o $@ $<

guest/crt0.o: guest/crt0.asm
	nasm -f elf $<

%.bin: %.c guest/crt0.o
	gcc $(CC) -m32 -c $< -o $*.o
	ld $(LD_OPT) guest/crt0.o $*.o -o $@

%.bin: %.asm
	nasm -f bin $< -o $@
