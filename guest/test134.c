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
        return fib(n-1) + fib(n-2);
    }
}

int sum1(int n) {
    if (n <= 0){
        return 0;
    } else {
        return n + sum1(n-1);
    }
}

int sum2(int n) {
    int res=0;
    while (n > 0) {
        res += n--;
    }
    return res;
}

int main(void) {
    int res = 0;
    
    res += f(5) - 0;
    res += fib(0) - 1;
    res += fib(1) - 1;
    res += fib(2) - 2;
    // res += fib(3) - 3;
    // res += fib(4) - 5;
    // res += fib(5) - 8;
 
    res += sum1(10) + sum2(10) - 55 - 55;

    return res;
}
