# tiny_x86_emu [![wercker status](https://app.wercker.com/status/7ac504b68746c744dd7dc4b5e52e4735/s/master "wercker status")](https://app.wercker.com/project/byKey/7ac504b68746c744dd7dc4b5e52e4735)

Tiny x86 Emulator

## How to use

## メモ

- Intel VT-xを使わないデメリット
  - パイプラインがないことによるIPC (Instruction Per Clock)の低下
  - Emulator自体の命令実行
  - TLBを利用していないことによるアドレス変換速度の低下
  - レジスタのアクセスが、すべてメモリへのアクセスになってしまう
