package main

// The 8254 programmable interval timer. Channel 0 drives IRQ 0 (the timer
// interrupt of a PC without a local APIC) and channel 2 is what Linux uses to
// calibrate the time stamp counter against, so both of them have to count at
// the right speed.
//
// Time is measured in executed instructions: the emulated cpu runs at one
// instruction per cycle, so instPerPITTick instructions pass between two ticks
// of the 1193182Hz timer crystal. That makes the cpu about 100MHz fast.
const (
	instPerPITTick = 84
	pitFrequency   = 1193182
	cpuFrequency   = instPerPITTick * pitFrequency
)

type pitChannel struct {
	mode      uint8  // counter mode 0-5
	rw        uint8  // access mode: 1 lsb, 2 msb, 3 lsb then msb
	initial   uint32 // reload value, 65536 when the guest wrote 0
	startTick uint64 // the tick at which the counter was loaded
	gate      bool   // the gate input (always high for channel 0 and 1)
	armed     bool   // a count has been written
	writeMSB  bool   // the next write to the data port is the high byte
	readMSB   bool
	lsb       uint8 // low byte of a count which is being written
	latched   bool
	latchVal  uint16
}

type pitState struct {
	ch      [3]pitChannel
	next    uint64 // tick at which channel 0 fires next
	speaker uint8  // port 0x61
}

func newPIT() pitState {
	var p pitState
	p.ch[0].gate = true
	p.ch[1].gate = true
	return p
}

// tick converts an instruction count into a tick of the timer crystal.
func pitTick(instCount uint64) uint64 { return instCount / instPerPITTick }

// count returns the current value of a counter.
func (c *pitChannel) count(now uint64) uint16 {
	if c.latched {
		return c.latchVal
	}
	if !c.armed {
		return 0
	}
	n := uint64(c.initial)
	elapsed := now - c.startTick
	if !c.gate {
		elapsed = 0
	}
	switch c.mode {
	case 2, 3:
		if n == 0 {
			return 0
		}
		return uint16(n - (elapsed % n))
	default:
		// modes 0, 1, 4 and 5 keep counting down after they expired
		return uint16((n - elapsed) & 0xFFFF)
	}
}

// out returns the state of the OUT pin of a counter.
func (c *pitChannel) out(now uint64) bool {
	if !c.armed {
		return false
	}
	if !c.gate {
		return c.mode == 0
	}
	elapsed := now - c.startTick
	switch c.mode {
	case 0, 1:
		return elapsed >= uint64(c.initial)
	case 2:
		return c.count(now) != 1
	case 3:
		n := uint64(c.initial)
		if n == 0 {
			return true
		}
		return (elapsed % n) < (n+1)/2
	}
	return elapsed >= uint64(c.initial)
}

func (c *pitChannel) load(now uint64, value uint32) {
	if value == 0 {
		value = 0x10000
	}
	c.initial = value
	c.startTick = now
	c.armed = true
}

// rearm computes the tick at which channel 0 raises the next interrupt.
func (p *pitState) rearm(now uint64) {
	c := &p.ch[0]
	if !c.armed {
		p.next = 0
		return
	}
	p.next = c.startTick + uint64(c.initial)
}

// step raises IRQ 0 when channel 0 expired. It returns true when an interrupt
// was raised.
func (p *pitState) step(now uint64) bool {
	c := &p.ch[0]
	if !c.armed || p.next == 0 || now < p.next {
		return false
	}
	switch c.mode {
	case 0, 1, 4, 5: // one shot
		p.next = 0
	default:
		p.next += uint64(c.initial)
		if p.next <= now {
			// the guest was not running for a long time: do not try to
			// catch up with a burst of interrupts
			p.next = now + uint64(c.initial)
		}
	}
	return true
}

// deadline returns the instruction count at which the next timer interrupt is
// due, or 0 when no interrupt is scheduled.
func (p *pitState) deadline() uint64 {
	if !p.ch[0].armed || p.next == 0 {
		return 0
	}
	return (p.next + 1) * instPerPITTick
}

func (p *pitState) out(port uint16, value uint8, now uint64) {
	switch port {
	case 0x43: // mode/command register
		sel := value >> 6
		if sel == 3 {
			// read back command, not used by the guests we care about
			return
		}
		c := &p.ch[sel]
		if value&0x30 == 0 { // counter latch command
			if !c.latched {
				c.latchVal = c.count(now)
				c.latched = true
			}
			return
		}
		c.rw = (value >> 4) & 0x3
		c.mode = (value >> 1) & 0x7
		if c.mode > 5 {
			c.mode -= 4
		}
		c.armed = false
		c.latched = false
		c.writeMSB = false
		c.readMSB = false
		if sel == 0 {
			p.next = 0
		}

	case 0x40, 0x41, 0x42:
		c := &p.ch[port-0x40]
		switch c.rw {
		case 1: // lsb only
			c.load(now, uint32(value))
		case 2: // msb only
			c.load(now, uint32(value)<<8)
		default: // lsb then msb
			if !c.writeMSB {
				c.lsb = value
				c.writeMSB = true
				return
			}
			c.writeMSB = false
			c.load(now, uint32(c.lsb)|uint32(value)<<8)
		}
		c.latched = false
		if port == 0x40 {
			p.rearm(now)
		}
	}
}

func (p *pitState) in(port uint16, now uint64) uint8 {
	if port < 0x40 || port > 0x42 {
		return 0xFF
	}
	c := &p.ch[port-0x40]
	value := c.count(now)
	switch c.rw {
	case 1:
		c.latched = false
		return uint8(value)
	case 2:
		c.latched = false
		return uint8(value >> 8)
	default:
		if !c.readMSB {
			c.readMSB = true
			return uint8(value)
		}
		c.readMSB = false
		c.latched = false
		return uint8(value >> 8)
	}
}

// speakerIn implements port 0x61: bit 0 is the gate of channel 2, bit 4 is the
// refresh bit which toggles at 66kHz and bit 5 is the OUT pin of channel 2.
func (p *pitState) speakerIn(now uint64) uint8 {
	v := p.speaker & 0x0F
	if now&0x8 != 0 {
		v |= 0x10
	}
	if p.ch[2].out(now) {
		v |= 0x20
	}
	return v
}

func (p *pitState) speakerOut(value uint8, now uint64) {
	gate := value&0x01 != 0
	if gate != p.ch[2].gate {
		p.ch[2].gate = gate
		if gate {
			p.ch[2].startTick = now
		}
	}
	p.speaker = value & 0x0F
}
