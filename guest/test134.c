int f(int a, int b) {
    return a+b;
}

int fib(int n) {
    if (n < 2) {
        return 1;
    } else {
        return 2;
    }
}

int main(void) {
    return fib(1) - 1;
}
