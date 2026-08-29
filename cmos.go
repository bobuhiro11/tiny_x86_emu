package main

import "time"

// The CMOS/RTC chip behind ports 0x70 and 0x71. Linux reads the wall clock
// from it while booting, everything else it may ask for reads back as zero.
type cmosState struct {
	index uint8
	ram   [128]uint8
}

func bcd(v int) uint8 {
	return uint8((v/10)<<4 | (v % 10))
}

func (c *cmosState) out(port uint16, value uint8) {
	if port == 0x70 {
		c.index = value & 0x7F // the high bit only disables NMI
		return
	}
	switch c.index {
	case 0x0A, 0x0B, 0x0C, 0x0D:
		// the status registers are read only here
	default:
		c.ram[c.index] = value
	}
}

func (c *cmosState) in(port uint16) uint8 {
	if port == 0x70 {
		return 0xFF
	}
	now := time.Now().UTC()
	switch c.index {
	case 0x00:
		return bcd(now.Second())
	case 0x02:
		return bcd(now.Minute())
	case 0x04:
		return bcd(now.Hour())
	case 0x06:
		return bcd(int(now.Weekday()) + 1)
	case 0x07:
		return bcd(now.Day())
	case 0x08:
		return bcd(int(now.Month()))
	case 0x09:
		return bcd(now.Year() % 100)
	case 0x0A:
		return 0x26 // the update in progress bit is never set
	case 0x0B:
		return 0x02 // 24 hour format, BCD values
	case 0x0C:
		return 0x00 // no interrupt pending
	case 0x0D:
		return 0x80 // the battery is good
	case 0x32:
		return bcd(now.Year() / 100)
	}
	return c.ram[c.index]
}
