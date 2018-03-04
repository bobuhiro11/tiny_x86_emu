BITS 32
extern main ; test.c
global start
start:
  call main
  jmp 0
