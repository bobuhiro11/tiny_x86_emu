int main (void) {
    __asm__("mov    $0x100000, %ecx;");
    __asm__("mov    $0x80000000, %edx;");
    __asm__("lea    -0x1(%edx,%ecx,1), %eax"); // 8d 44 0a ff
    register int eax asm("eax");
    return eax;
}
