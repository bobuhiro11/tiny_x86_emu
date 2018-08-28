// +build wasm

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"syscall/js"
)

func main() {
	fmt.Printf("Hello, wasm!\n")
	for i := 0; i < 10; i++ {
		t := js.Global().Get("document").Call("getElementById", "terminal")
		t.Set("innerHTML", t.Get("innerHTML").String()+"hello, wasm2\n")
	}
	runtime.LockOSThread()
	f, err := Assets.Open("/xv6-public/xv6.img")
	if err != nil {
		panic(err)
	}
	bytes, err := ioutil.ReadAll(f)
	if err != nil {
		panic(err)
	}
	fmt.Println(len(bytes))

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
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}

		if e.eip == 0 || e.eip == 0x7c00 {
			break
		}
		i++
	}
	e.dump(i)
	fmt.Println("End of program")
}
