package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
)

var (
	filename string
)

func main() {
	flag.StringVar(&filename, "f", "", "binary filename")
	flag.Parse()

	// load binary
	if filename == "" {
		fmt.Fprintln(os.Stderr, "Please set filename")
		os.Exit(1)
	}

	bytes, err := loadFile(filename)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	fmt.Printf("bytes =\n%s\n", hex.Dump(bytes))

	// setup emulator
	e := NewEmulator(10000, 0, 100)
	for i := 0; i < len(bytes); i++ {
		e.memory[i] = bytes[i]
	}

	// emulate
	for e.eip < 10000 {
		err := e.exec_inst()
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}

		if e.eip == 0 {
			fmt.Println("End of program")
			e.dump()
			break
		}
	}
}

func loadFile(filename string) ([]byte, error) {
	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}
