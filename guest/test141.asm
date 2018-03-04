BITS 32
  org 0x7c00
start:
  mov edx, 0x03f8
  mov eax, 0x41 ; 'A'
  out dx, al
  mov eax, 0x0A ; '\n'
  out dx, al
  jmp 0
