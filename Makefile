rebuild: clean build
build:
	go build
test: guest_bin
	pkgs=$(go list ./... | grep -v /vendor/)
	go vet ${pkgs}
	golint ${pkgs}
	go test ${pkgs} -v --cover
clean:
	go clean
guest_bin:
	# binary from gcc
	gcc -Wl,--entry=inc,--oformat=binary -nostdlib -fno-asynchronous-unwind-tables \
		-o guest/inc.bin guest/inc.c
	# binary from nasm
	for name in addjmp modrm-test call-test test141 test143 mbr; do \
		nasm -f bin ./guest/$${name}.asm -o ./guest/$${name}.bin ; \
	done
	# elf from gcc
	for name in test test132 test133 ; do \
		gcc -nostdlib -fno-pie -fno-asynchronous-unwind-tables -g -fno-stack-protector -m32 \
		-c guest/$${name}.c -o guest/$${name}.o ; \
	done
	# elf from nasm
	nasm -f elf guest/crt0.asm
	# link
	ld -m elf_i386 --entry=start --oformat=binary -Ttext 0x7c00 \
		-o guest/test.bin guest/crt0.o guest/test.o
	ld -m elf_i386 --entry=start --oformat=binary -Ttext 0x7c00 \
		-o guest/test132.bin guest/crt0.o guest/test132.o
	ld -m elf_i386 --entry=start --oformat=binary -Ttext 0x7c00 \
		-o guest/test133.bin guest/crt0.o guest/test133.o
	# disasm
	# objdump -D -b binary -m i386:x86-64 ./guest/addjmp.bin
	# ndisasm -b 32 guest/test143.bin
	# hexdump -C guest/inc.bin
	./dump_registers.sh
