QEMU_XV6_LOG_URL=https://www.dropbox.com/s/i2zwrr40zkvdjh0/qemu_xv6.log

GO_BUILD_OPT=-gcflags '-N -l'
LD_OPT=-m elf_i386 --entry=start --oformat=binary -Ttext 0x7c00
CC= -nostdlib -fno-pie -fno-asynchronous-unwind-tables -g -fno-stack-protector

SRCS=$(shell find . -type f -name '*.go')
PKGS=$(go list ./... | grep -v /vendor/)
GUEST_BINARIES=guest/test.bin guest/test132.bin guest/test133.bin \
			   guest/test134.bin guest/test135.bin guest/addjmp.bin \
			   guest/modrm-test.bin guest/call-test.bin guest/test141.bin \
			   guest/test143.bin guest/mbr.bin
GO_GET_PKGS=github.com/mattn/goveralls \
			github.com/goreleaser/goreleaser \
			github.com/golang/lint/golint \
			github.com/jessevdk/go-assets \
			gopkg.in/yaml.v2 \
			github.com/jessevdk/go-assets-builder

.SUFFIX: .bin

.PHONY: all
all: tiny_x86_emu wasm/tiny_x86_emu.wasm

.PHONY: test
test: goget $(GUEST_BINARIES) xv6-public/xv6.img qemu_xv6.log
	go vet $(PKGS) && golint $(PKGS) && go test $(PKGS) -v --cover -timeout 5h

.PHONY: clean
clean:
	make --quiet -C xv6-public/ clean
	rm -f tiny_x86_emu wasm/tiny_x86_emu.wasm guest/*.bin guest/*.o
	go clean

.PHONY: goget
goget:
	go get $(GO_GET_PKGS)

.PHONY: qemu_xv6.log
qemu_xv6.log:
	wget --no-clobber $(QEMU_XV6_LOG_URL)

.PHONY: xv6-public/xv6.img
xv6-public/xv6.img:
	make --quiet -C ./xv6-public xv6.img

tiny_x86_emu: goget xv6-public/xv6.img assets.go $(SRCS) 
	go build $(GO_BUILD_OPT)

wasm/tiny_x86_emu.wasm: goget xv6-public/xv6.img assets.go $(SRCS) 
	GOOS=js GOARCH=wasm go build $(GO_BUILD_OPT) -o $@

assets.go: xv6-public/xv6.img
	go-assets-builder $< > $@

guest/inc.bin: guest/inc.c
	gcc -Wl,--entry=inc,--oformat=binary $(CC) -o $@ $<

guest/crt0.o: guest/crt0.asm
	nasm -f elf $<

%.bin: %.c guest/crt0.o
	gcc $(CC) -m32 -c $< -o $*.o
	ld $(LD_OPT) guest/crt0.o $*.o -o $@

%.bin: %.asm
	nasm -f bin $< -o $@
