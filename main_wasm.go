//go:build wasm
// +build wasm

package main

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"fmt"
	"io"
	"syscall/js"
	"time"
)

//go:embed wasm/xv6.img.gz
var xv6Image []byte

//go:embed wasm/fs.img.gz
var fsImage []byte

// outBuf collects the guest output between two screen updates.
var outBuf []byte

func printf(format string, a ...interface{}) {
	outBuf = append(outBuf, []byte(fmt.Sprintf(format, a...))...)
}

func flush() {
	if len(outBuf) == 0 {
		return
	}
	js.Global().Call("emuWrite", string(outBuf))
	outBuf = outBuf[:0]
}

// wasmWriter forwards everything the guest writes to the terminal.
type wasmWriter struct{}

func (w wasmWriter) Write(p []byte) (int, error) {
	outBuf = append(outBuf, p...)
	return len(p), nil
}

func gunzip(data []byte) []byte {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		panic(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		panic(err)
	}
	return out
}

func main() {
	input := make(chan byte, 1024)

	// The page calls emuKey() for every key the user presses.
	js.Global().Set("emuKey", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) == 0 {
			return nil
		}
		for _, b := range []byte(args[0].String()) {
			select {
			case input <- b:
			default:
			}
		}
		return nil
	}))

	boot := gunzip(xv6Image)
	fs := gunzip(fsImage)

	e := NewEmulator(0x7c00, 0x7c00, false, wasmWriter{}, map[uint64]string{})
	copy(e.memory[0x7c00:], boot[:SectorSize])
	e.io.hdds[0] = NewMemDisk(boot)
	e.io.hdds[1] = NewMemDisk(fs)

	js.Global().Call("emuReady")

	for {
		// Feed the console with everything typed since the last round.
		for {
			select {
			case b := <-input:
				e.io.PushInput(b)
				continue
			default:
			}
			break
		}

		// Run a batch of instructions, then hand control back to the
		// browser so that the screen is updated and key events arrive.
		batch := 200000
		if e.halted {
			// the guest is idle: one timer tick per round is enough
			batch = 1
		}
		for i := 0; i < batch; i++ {
			if err := e.execInst(); err != nil {
				printf("\n%s\n", err.Error())
				flush()
				return
			}
			if e.shutdown {
				printf("\nEnd of program\n")
				flush()
				return
			}
			if e.halted {
				break
			}
		}
		flush()
		if e.halted {
			time.Sleep(8 * time.Millisecond)
		} else {
			time.Sleep(time.Millisecond)
		}
	}
}
