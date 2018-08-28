// +build wasm

package main

import (
	"fmt"
	"syscall/js"
)

func main() {
	fmt.Printf("Hello, wasm!\n")
	for i := 0; i < 10; i++ {
		t := js.Global().Get("document").Call("getElementById", "terminal")
		t.Set("innerHTML", t.Get("innerHTML").String()+"hello, wasm2\n")
	}
}
