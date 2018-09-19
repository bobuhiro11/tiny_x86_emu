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
	s := fmt.Sprintf(format, a...)
	t := js.Global().Get("document").Call("getElementById", "terminal")
	t.Call("insertAdjacentHTML", "beforeend", s)
	t.Set("scrollTop", t.Get("scrollHeight"))
	time.Sleep(5 * time.Millisecond)
}

type WasmWriter struct{}

func (w WasmWriter) Write(p []byte) (n int, err error) {
	printf("[foo]%v", p)
	return len(p), nil
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
	writer := WasmWriter{}
	e := NewEmulator(0x7c00+0x10240000, 0x7c00, 0x6f04, false, true, os.Stdin, writer, map[uint64]string{})
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

		// exit in scheduler()
		if e.eip == 0 || e.eip == 0x7c00 || e.eip == 0x80103bf0 {
			break
		}
		i++
	}
	e.dump(i)
	printf("End of program\n")
}
