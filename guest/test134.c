int f(int a) {
    int r = 0;
    if (a  > 10) { r += 1; }
    if (a >= 50) { r += 2; }
    if (a  < 5)  { r += 4; }
    if (a <= 4)  { r += 8; }
    if (a != 5)  { r += 16; }
    return r;
}

int fib(int n) {
    if (n < 2) {
        return 1;
    } else {
        return 2;
    }
}

int main(void) {
    return f(5);
}
