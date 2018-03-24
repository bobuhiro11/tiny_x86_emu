package main

// IO has I/O port and emulate I/O device
type IO struct {
	memory [2^16]uint8 // I/O port
}

// NewIO creates New IO
func NewIO() IO{
	return IO{}
}
