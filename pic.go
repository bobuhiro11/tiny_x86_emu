package main

// Refs: http://oswiki.osask.jp/?%28PIC%298259A
type PIC8259A struct {
	commandReg uint8 // Command Register (Write Only)
	statusReg  uint8 // Status Register (Read Only)
	IRR        uint8 // Interrupt Request Register
	ISR        uint8 // In-Service Register
	IMR        uint8 // Interrupt Mask Register
}
