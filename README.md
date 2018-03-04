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
- 16ビットモードに対応
  - 実機起動時には、16bitモードで起動する（つまり16bitmodeのアセンブリを実行できる）
  - 今作っているエミュレータはデフォルトが32bitモードなので、それを16bitにする
  - 0x66（Operand-size override prefix）や0x67（Adddress-size override prefix）がオペコードの先頭についている場合には、レジスタサイズを変更して実行する（P.157）
- BIOSがHDDからデータを読み出すときは、物理セクタサイズ（512あるいか4096KB）のかわりに、統一して512B（Big Sector）を使う
  - HDDの先頭のセクタ512バイトの最後2バイトが、0x55、0xaaの場合には、このセクタはブートセクタと呼ばれ、メモリの0x7c00以降に読み込まれ、ジャンプする。デバイスによっては、以下のように用語を区別する
  - PBR（Partition Boot Record）：フロッピーやUSBメモリなど1パーティションしか持たないもの。OSとフォーマットに依存したローダプログラムを書く必要がある。PBRの512バイトに収めるの困難な場合には、2段のロード方式（多段ブート）を行う。例えば、Windows NT系で使われていたNTLDRは2段。
  - MBR（Master Boot Record）：複数のパーティションをもつHDDなど。先頭446Bは機械語を下記、その後16B x 4パーティション = 64Bに、パーティションテーブルを書く。パーティションテーブルのエントリには、CHS方式あるいはLBA方式でパーティションの開始セクタ（PBR）を指示する。LBA方式はいずれのトラックにおいてもセクタ数が同じとう制約があるので、現在ではふつうLBAで位置決めをする。こちらは、OSやフォーマット形式に依存しない汎用的なもので、起動フラグが0x80であるパーティションを探してそのパーティションの先頭セクタを0x7c00に読み込んでジャンプするだけで良い。ここで、MBR自身も0x7c00にあるのでは？
  - PBRかMBRの判定プログラムなどを実際に組んで、さらに中身を解析すると面白そう。
