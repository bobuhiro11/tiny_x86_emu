# tiny_x86_emu [![wercker status](https://app.wercker.com/status/7ac504b68746c744dd7dc4b5e52e4735/s/master "wercker status")](https://app.wercker.com/project/byKey/7ac504b68746c744dd7dc4b5e52e4735)

Tiny x86 Emulator

## How to use

```bash
# ubuntu
$ sudo apt install libgl1-mesa-dev xorg-dev
$ make

# compile for windows on ubuntu
$ sudo apt install mingw-w64
$ CGO_ENABLED=1 CXX=x86_64-w64-mingw32-g++ CC=x86_64-w64-mingw32-gcc GOOS=windows GOARCH=amd64 go build
```


## メモ

- Intel VT-xを使わないデメリット
  - パイプラインがないことによるIPC (Instruction Per Clock)の低下
  - Emulator自体の命令実行
  - TLBを利用していないことによるアドレス変換速度の低下
  - レジスタのアクセスが、すべてメモリへのアクセスになってしまう
