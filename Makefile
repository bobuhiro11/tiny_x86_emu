LD_OPT=-m elf_i386 --entry=start --oformat=binary -Ttext 0x7c00

.PHONY: rebuild build clean test goget xv6 guest_bin
rebuild: clean build
build: xv6
	go build -gcflags '-N -l'
	GOOS=js GOARCH=wasm go build -gcflags '-N -l' -o ./wasm/tiny_x86_emu.wasm
test: guest_bin xv6
	ls qemu_xv6.log || wget https://www.dropbox.com/s/i2zwrr40zkvdjh0/qemu_xv6.log # download from cache
	pkgs=$(go list ./... | grep -v /vendor/)
	go vet ${pkgs}
	golint ${pkgs} && go test ${pkgs} -v --cover -timeout 5h
clean:
	make -C xv6-public/ clean
	rm ./wasm/tiny_x86_emu.wasm || true
	go clean
goget:
	go get github.com/mattn/goveralls
	go get github.com/goreleaser/goreleaser
	go get github.com/golang/lint/golint
	go get github.com/jessevdk/go-assets
	go get gopkg.in/yaml.v2
	go get github.com/jessevdk/go-assets-builder
xv6:
	if [ ! -d xv6-public ]; then git clone --depth 1 https://github.com/mit-pdos/xv6-public.git; fi
	make -C ./xv6-public
	go-assets-builder ./xv6-public/xv6.img > assets.go
guest_bin:
	# binary from gcc
	gcc -Wl,--entry=inc,--oformat=binary -nostdlib -fno-asynchronous-unwind-tables \
		-o guest/inc.bin guest/inc.c
	# binary from nasm
	for name in addjmp modrm-test call-test test141 test143 mbr; do \
		nasm -f bin ./guest/$${name}.asm -o ./guest/$${name}.bin ; \
	done
	# elf from gcc
	for name in test test132 test133 test134 test135 ; do \
		gcc -nostdlib -fno-pie -fno-asynchronous-unwind-tables -g -fno-stack-protector -m32 \
		-c guest/$${name}.c -o guest/$${name}.o ; \
	done
	# elf from nasm
	nasm -f elf guest/crt0.asm
	# link
	ld ${LD_OPT} -o guest/test.bin guest/crt0.o    guest/test.o
	ld ${LD_OPT} -o guest/test132.bin guest/crt0.o guest/test132.o
	ld ${LD_OPT} -o guest/test133.bin guest/crt0.o guest/test133.o
	ld ${LD_OPT} -o guest/test134.bin guest/crt0.o guest/test134.o
	ld ${LD_OPT} -o guest/test135.bin guest/crt0.o guest/test135.o
	# disasm
	# objdump -D -b binary -m i386:x86-64 ./guest/addjmp.bin
	# ndisasm -b 32 guest/test143.bin
	# hexdump -C guest/inc.bin
	# ./dump_registers.sh
