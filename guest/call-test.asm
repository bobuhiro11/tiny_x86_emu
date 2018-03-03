BITS 32
  org 0x7c00
start:
  mov eax, 0xf1
  mov ebx, 0x29
  call add_routine
  jmp short start
add_routine:
  mov ecx, eax
  add ecx, ebx # ecx = 0x011a
  ret
