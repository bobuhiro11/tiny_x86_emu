all: tiny_x86_emu
tiny_x86_emu:
	go build ./...
test: guest_bin
	go vet $(go list ./... | grep -v /vendor/)
	golint $(go list ./... | grep -v /vendor/)
	go test $(go list ./... | grep -v /vendor/) --cover
	# goveralls -repotoken $COVERALL_REPO_TOKEN
guest_bin:
	gcc -Wl,--entry=inc,--oformat=binary -nostdlib -fno-asynchronous-unwind-tables -o guest/inc.bin guest/inc.c
	gcc -c -g -o guest/inc.o guest/inc.c
	ndisasm -b 32 guest/inc.bin
	objdump -d -S -M intel guest/inc.o
