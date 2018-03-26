package main

import (
	"fmt"
	"bufio"
	"os"
	"io"
)

// IO has I/O port and emulate I/O device
type IO struct {
	memory [65536]uint8 // I/O port
	reader *io.Reader
	writer *io.Writer
}

// NewIO creates New IO
func NewIO(reader *io.Reader, writer *io.Writer) IO{
	return IO{
		reader: reader,
		writer: writer,
	}
}

func (io *IO) in8(address uint16) uint8 {
	fmt.Printf("io.in8 from 0x%x\n", address)
	switch address {
	case 0x0064: // Keyboard Controller Read Status
		io.memory[address] = 0
	case 0x01f7: // 1st Hark Disk Status (4th bit means drive ready)
		io.memory[address] = 0x40
	case 0x03f8: // Reciever Buffer Register
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		io.memory[address] = input[0]
	}
	return io.memory[address]
}

func (io *IO) out8(address uint16, value uint8){
	fmt.Printf("io.out8 address=0x%x value=0x%x\n", address, value)
	io.memory[address] = value
	switch address {
	case 0x0060: // Keyboard Input Register
		return
	case 0x0064: // Keyboard Input Buffer
		return
	case 0x03f8: // Transmitter Holding Register
		fmt.Fprint(*io.writer, string(io.memory[address]))
	default:
		return
	}
}
