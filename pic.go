package main

// The two cascaded 8259A interrupt controllers of a PC. xv6 masks them and
// uses the I/O APIC instead, Linux (built without CONFIG_X86_LOCAL_APIC) uses
// them for every device interrupt.
//
// Index 0 is the master (ports 0x20/0x21, IRQ 0-7), index 1 is the slave
// (ports 0xA0/0xA1, IRQ 8-15) which is cascaded on IRQ 2 of the master.
type picChip struct {
	irr      uint8 // interrupt request register
	isr      uint8 // in service register
	imr      uint8 // interrupt mask register
	base     uint8 // vector of IRQ 0 of this chip
	initStep int   // 0: idle, 1: waiting for ICW2, 2: ICW3, 3: ICW4
	icw1     uint8
	readISR  bool // the next read of the command port returns the ISR
	autoEOI  bool
}

type picState struct {
	chip [2]picChip
}

func (p *picState) reset() {
	p.chip[0] = picChip{imr: 0xFF, base: 0x08}
	p.chip[1] = picChip{imr: 0xFF, base: 0x70}
}

// raise asserts one of the 16 interrupt request lines.
func (p *picState) raise(irq int) {
	if irq < 0 || irq > 15 {
		return
	}
	if irq < 8 {
		p.chip[0].irr |= 1 << uint(irq)
	} else {
		p.chip[1].irr |= 1 << uint(irq-8)
	}
}

// highest returns the number of the highest priority interrupt which is
// requested, not masked and not already being serviced.
func (c *picChip) highest() int {
	ready := c.irr &^ c.imr
	for i := 0; i < 8; i++ {
		bit := uint8(1) << uint(i)
		if c.isr&bit != 0 {
			// a lower priority interrupt cannot preempt this one
			return -1
		}
		if ready&bit != 0 {
			return i
		}
	}
	return -1
}

// pending returns the vector of the interrupt which should be delivered next,
// or -1 when the cpu has nothing to do.
func (p *picState) pending() int {
	irq := p.chip[0].highest()
	if irq < 0 {
		return -1
	}
	if irq == 2 {
		// the slave is cascaded on IRQ 2
		if s := p.chip[1].highest(); s >= 0 {
			return int(p.chip[1].base) + s
		}
		return -1
	}
	return int(p.chip[0].base) + irq
}

// acknowledge performs the interrupt acknowledge cycle for the vector
// previously returned by pending.
func (p *picState) acknowledge(vector int) {
	irq := p.chip[0].highest()
	if irq < 0 {
		return
	}
	if irq == 2 {
		s := p.chip[1].highest()
		if s < 0 {
			return
		}
		p.chip[1].irr &^= 1 << uint(s)
		if !p.chip[1].autoEOI {
			p.chip[1].isr |= 1 << uint(s)
		}
	}
	p.chip[0].irr &^= 1 << uint(irq)
	if !p.chip[0].autoEOI {
		p.chip[0].isr |= 1 << uint(irq)
	}
}

func (p *picState) out(port uint16, value uint8) {
	i := 0
	if port >= 0xA0 {
		i = 1
	}
	c := &p.chip[i]
	command := port&1 == 0

	if command {
		switch {
		case value&0x10 != 0: // ICW1
			c.icw1 = value
			c.imr = 0
			c.isr = 0
			c.irr = 0
			c.readISR = false
			c.autoEOI = false
			c.initStep = 1
		case value&0x08 != 0: // OCW3
			if value&0x02 != 0 {
				c.readISR = value&0x01 != 0
			}
		default: // OCW2
			switch value >> 5 {
			case 0x1: // non specific EOI
				for b := 0; b < 8; b++ {
					if c.isr&(1<<uint(b)) != 0 {
						c.isr &^= 1 << uint(b)
						break
					}
				}
			case 0x3: // specific EOI
				c.isr &^= 1 << (value & 0x7)
			}
		}
		return
	}

	switch c.initStep {
	case 1: // ICW2: the vector base
		c.base = value &^ 0x07
		switch {
		case c.icw1&0x02 == 0: // cascade mode: ICW3 comes next
			c.initStep = 2
		case c.icw1&0x01 != 0: // single mode, but with an ICW4
			c.initStep = 3
		default:
			c.initStep = 0
		}
	case 2: // ICW3: the cascade configuration, which is fixed here
		if c.icw1&0x01 != 0 {
			c.initStep = 3
		} else {
			c.initStep = 0
		}
	case 3: // ICW4
		c.autoEOI = value&0x02 != 0
		c.initStep = 0
	default: // OCW1
		c.imr = value
	}
}

func (p *picState) in(port uint16) uint8 {
	i := 0
	if port >= 0xA0 {
		i = 1
	}
	c := &p.chip[i]
	if port&1 == 0 {
		if c.readISR {
			return c.isr
		}
		return c.irr
	}
	return c.imr
}
