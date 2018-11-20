package main

// Interrupts on xv6が詳しい

// PIC(8259)は、IOAPIC（またはcpu0のlocal APIC）に接続されている
// xv6では、レガシーデバイス（PS/2 keyboard, IDE, COM0）のために、PIC->IOAPICの順で割り込み設定をする

// LocalAPIC is ..
// in CPU
// - Select Interrupt Vector Number
// - End of Interrupt
// - Timer, Thermo sensor, Performance counter
// - API IDs are unique in CPUs.
type LocalAPIC struct {
	IRR uint8 // Interrupt Request Register: CPUが未処理のベクタ番号にビットが立つ
	ISR uint8 // In-Service Register：次に割り込む候補、EOIへ書き込まれると、IRR->ISRへビットが更新
	IMR uint8 // Interrupt Mask Register, the bit is 0 only when the IRQ is enabled.
}

// IOAPIC is ...
// at ICH (South Bridge)
type IOAPIC struct {
}

// PIC is ...
type PIC struct {
}
