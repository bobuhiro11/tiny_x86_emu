// +build wasm

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"time"
	// "runtime"
	"syscall/js"
)

func printf(format string, a ...interface{}) {
	// fmt.Printf(format, a)
	s := fmt.Sprintf(format, a...)
	// fmt.Println(s)
	t := js.Global().Get("document").Call("getElementById", "terminal")
	t.Call("insertAdjacentHTML", "beforeend", s)
	time.Sleep(10 * time.Millisecond)
}

func main() {
	printf("hello, world!!!!")
	// runtime.LockOSThread()
	f, err := Assets.Open("/xv6-public/xv6.img")
	if err != nil {
		panic(err)
	}
	bytes, err := ioutil.ReadAll(f)
	if err != nil {
		panic(err)
	}

	// setup emulator
	e := NewEmulator(0x7c00+0x10240000, 0x7c00, 0x6f04, false, true, os.Stdin, os.Stdout, map[uint64]string{})
	for i := 0; i < len(bytes); i++ {
		e.memory[uint32(i+0x7c00)] = bytes[i]
	}
	e.io.hdds[0], _ = Assets.Open("/xv6-public/xv6.img")
	f, err = Assets.Open("/xv6-public/xv6.img")

	// emulate
	i := 0
	for {
		// if !*silent && 0x8010376c < e.eip && e.eip < 0x801037d1 {
		if false {
			// if !*silent && i > 3635000 {
			e.dump(i)
		}
		err := e.execInst()
		if err != nil {
			printf(err.Error())
			os.Exit(1)
		}

		if e.eip == 0 || e.eip == 0x7c00 {
			break
		}
		i++
	}
	e.dump(i)
	printf("End of program\n")
}
