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
	hdds   [10]*os.File
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
	case 0x01f0: // Data Register (Read sector 32bit-chunk, 128 times)
	case 0x01f7: // 1st Hark Disk Status (4th bit means drive ready)
		io.memory[address] = 0x40
	case 0x03f8: // Reciever Buffer Register
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		io.memory[address] = input[0]
	}
	return io.memory[address]
}

func (io *IO) in32(address uint16) uint32 {
	var ret uint32
	for i := uint16(0); i < 4; i++ {
		ret |= uint32(io.in8(address)) << uint32(i*8)
	}
	return ret
}

func (io *IO) out8(address uint16, value uint8){
	fmt.Printf("io.out8 address=0x%x value=0x%x\n", address, value)
	io.memory[address] = value
	switch address {
	case 0x01f2: // Secter Count
		fmt.Printf("Secter Count=%d\n", io.memory[address])
		return
	case 0x01f3: // Secter Number
		fmt.Printf("Secter Number=%d\n", io.memory[address])
		return
	case 0x01f4: // Cylinder low
		fmt.Printf("Sylinder Low=%d\n", io.memory[address])
		return
	case 0x01f5: // Cylinder High
		fmt.Printf("Sylinder High=%d\n", io.memory[address])
		return
	case 0x01f6: // Drive/Head
		fmt.Printf("Drive Number=%d\n", (io.memory[address]&0x8) >> 4)
		return
	case 0x01f7: // Command Register
		fmt.Printf("Command Register=%d\n", io.memory[address])
		switch io.memory[address] {
		case 0x20:
			fmt.Printf("Read sectors with Retry command is sent.\n")
		}
		return
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
