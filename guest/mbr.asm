BITS 16
  org 0x7c00
  jmp short entry
  nop
  db "MSDOS5.0"
  dw 512
  db 4
  dw 4
  db 2
  dw 512
  dw 0
  db 248
  dw 250
  dw 63
  dw 255
  dd 0
  dd 256000
  db 128
  db 0
  db 41
  dd 2963790959
  db "NO NAME  "
  db "FAT 16  "
  times 0x3e - ($ - $$) db 0
entry:
  mov ax, 0  ; init registers
  mov ss, ax
  mov sp, 0x7c00
  mov ds, ax
  mov es, ax

  mov ax, 0x0013 ; video mode
  int 0x10

  mov bl, 14
  mov si, msg1
  call puts
  mov bl, 15
  mov si, msg2
  call puts
  mov bl, 13
  mov si, msg3
  call puts
  mov bl, 15
  mov si, msg4
  call puts
fin:
  hlt
  jmp fin
puts:
  mov al, [si]
  inc si
  cmp al,0
  je puts_end
  mov ah, 0x0e
  mov bh, 0
  int 0x10
  jmp puts
puts_end:
  ret
msg1:
  db "Congratulations!", 0x0d, 0x0a, 0
msg2:
  db "You are on a way to ", 0
msg3:
  db "hacker",0
msg4:
  db "!!", 0x0d, 0x0a, 0
  times 510 - ($-$$) db 0
  db 0x55, 0xaa
