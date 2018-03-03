BITS 32
  org 0x7c00
start:
  sub esp, 32            ; esp -= 32
  mov ebp, esp           ; ebp = esp
  mov eax, 2             ; eax = 2
  mov dword [ebp+4], 5   ; *(ebp+4) = 0x00000005
  add dword [ebp+4], eax ; *(ebp+4) = 0x00000007
  mov esi, [ebp+4]       ; esi = 0x00000007
  inc dword [ebp+4]      ; *(ebp+4) = 0x00000008
  mov edi, [ebp+4]       ; edi = 0x00000008
  jmp short start
