package main

import (
	// "bufio"
	// "fmt"
	"io"
	// "os"
)

const (
	// SectorSize is 512 Byte
	SectorSize = 512
)

type ReaderSeeker interface {
	Seek(offset int64, whence int) (int64, error)
	Read(p []byte) (n int, err error)
}

// IO has I/O port and emulate I/O device
type IO struct {
	memory [65536]uint8 // I/O port
	reader *io.Reader
	writer *io.Writer
	hdds   [10]ReaderSeeker
}

// NewIO creates New IO
func NewIO(reader *io.Reader, writer *io.Writer) IO {
	return IO{
		reader: reader,
		writer: writer,
	}
}

var times0x01f7 = 0

func (io *IO) in8(address uint16) uint8 {
	//printf("io.in8 from 0x%x\n", address)
	switch address {
	case 0x0064: // Keyboard Controller Read Status
		io.memory[address] = 0x1c
	case 0x01f0: // Data Register (Read sector 32bit-chunk, 128 times)
		b := make([]byte, 1)
		io.hdds[0].Read(b)
		io.memory[address] = b[0]
	case 0x01f7: // 1st Hark Disk Status (4th bit means drive ready)
		if times0x01f7&0x01 == 0 {
			io.memory[address] = 0x50
		} else {
			io.memory[address] = 0x58
		}
		times0x01f7++
	case 0x03f8: // COM1+0: Reciever Buffer Register
		// reader := bufio.NewReader(os.Stdin)
		// input, _ := reader.ReadString('\n') // TODO: fixme
		input := "\n"
		io.memory[address] = input[0]
	case 0x03f8 + 5: // COM1+5: Line Status Register
		io.memory[address] = 0x20
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

func (io *IO) out16(address, value uint16) {
	io.out8(address, uint8(value&0xFF))
	io.out8(address+1, uint8((value&0xFF00)>>8))
}

func (io *IO) out8(address uint16, value uint8) {
	// printf("io.out8 address=0x%x value=0x%x\n", address, value)
	io.memory[address] = value
	switch address {
	case 0x01f2: // Secter Count
		printf("Secter Count=%d\n", io.memory[address])
		return
	case 0x01f3: // Secter Number
		io.hdds[0].Seek(0, 0) // Read a entire sector (TODO: check)
		offset, _ := io.hdds[0].Seek(int64(value)*SectorSize, 1)
		printf("Secter Number=%d offset=%d\n", io.memory[address], offset)
		return
	case 0x01f4: // Cylinder low
		offset, _ := io.hdds[0].Seek(int64(uint32(value)<<8)*SectorSize, 1)
		printf("Sylinder Low=%d offset=%d\n", io.memory[address], offset)
		return
	case 0x01f5: // Cylinder High
		offset, _ := io.hdds[0].Seek(int64(uint32(value)<<16)*SectorSize, 1)
		printf("Sylinder High=%d offset=%d\n", io.memory[address], offset)
		return
	case 0x01f6: // Drive/Head
		offset, _ := io.hdds[0].Seek(int64((uint32(value)&0x1F)<<24)*SectorSize, 1)
		printf("Drive Number=%d offset=%d\n", (io.memory[address]&0x8)>>4, offset)
		return
	case 0x01f7: // Command Register
		printf("Command Register=%d\n", io.memory[address])
		switch io.memory[address] {
		case 0x20:
			printf("Read sectors with Retry command is sent.\n")
		}
		return
	case 0x0060: // Keyboard Input Register
		return
	case 0x0064: // Keyboard Input Buffer
		return
	case 0x03f8: // COM1+0: Transmitter Holding Register
		// fmt.Fprint(*io.writer, string(io.memory[address]))
		printf("%s", string(io.memory[address]))
	default:
		return
	}
}
