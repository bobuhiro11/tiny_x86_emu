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
- Unix/Linuxでは、論理アドレス、仮想アドレス、リニアアドレスが等価。Segmentは実際には使っていない。
- `EIP=0x00007c2c (opecode=ea, EA317C0800 jmp 0x8:0x7c31)`の次の命令`EIP=0x00007c31 (opecode=66, 66B810008ED8 mov eax,0xd88e0010)`からは、32bit protected mode で動作が始まる。
- UnmarshalYamlを使う

![](https://image.slidesharecdn.com/linuxintroduction-130907015640-/95/linux-introduction-29-638.jpg)
![](http://slideplayer.com/slide/4865857/15/images/29/32bit+Mode:+4MB+Page+Mapping.jpg)


- xv6カーネルでは、以下の手順でカーネルメモリを初期化している
  - main() -> kinit2(P2V(4*1024*1024), P2V(PHYSTOP)); -> freerange(vstart, vend); -> kfree(p); -> memset(v, 1, PGSIZE); -> `stosl(dst, (c<<24)|(c<<16)|(c<<8)|c, n/4);`
  - memsetの引数は、dst=0x80400000 c=1 n=4096となっていた
  - また、stolsカンスの引数は、stosl start. addr=0x80400000 data=16843009 cnt=1024 だった
  - ここで、stosl命令、アセンブリではf3 ab rep stos %eax,%es:(%edi)に対応する
- このときに、エミュレータには、以下の出力が出ていた。本来stosd命令が1024回繰り返されるはずだが、実際には残り512回のところで実行エラーになってしまった。
- よく見ると、repeat 512のとき、0x80400800(0x800)番地をいじっているため、その次のeipに対するV2Pがおかしい
- pdtentry=0x800なので、上記と完全に一致してしまっている。。。
- pdtentryの場所がおかしいのか？？

```text 
repeat 514 times. eip=0x80104601 code=0xab
  stosd address=0x804007f8(0x7f8) value=0x1010101
  The exec of 514 loop finished.
  Next eip=0x80104600(0x104600) paddr of pdtentry=0x800 code=0xf3 ecx=0x201
repeat 513 times. eip=0x80104601 code=0xab
  stosd address=0x804007fc(0x7fc) value=0x1010101
  The exec of 513 loop finished.
  Next eip=0x80104600(0x104600) paddr of pdtentry=0x800 code=0xf3 ecx=0x200
repeat 512 times. eip=0x80104601 code=0xab
  stosd address=0x80400800(0x800) value=0x1010101
  The exec of 512 loop finished.
  Next eip=0x80104600(0x1114701) paddr of pdtentry=0x800 code=0x0 ecx=0x1ff
eip=80104600 opecode = 0 is not implemented at execInst().
```

- ということで、もうすこし調べてみる
- kinit1は、カーネルELFバイナリの最後のアドレス`end`から4MBを初期化する
  - 仮想アドレスでは、0x801154a8 ~ 0x80400000 の範囲になる
  - これは初期状態ではページテーブルを4MBしか用意していないため
- kinit2では、4MBからPHYSTOP 4GBを初期化する
  - 仮想アドレスでは、0x80400000 ~ 0x8e000000 の範囲になる
- vm.c のkmapのコメントが詳しい

```
// setupkvm() and exec() set up every page table like this:
//
//   0..KERNBASE: user memory (text+data+stack+heap), mapped to
//                phys memory allocated by the kernel
//   KERNBASE..KERNBASE+EXTMEM: mapped to 0..EXTMEM (for I/O space)
//   KERNBASE+EXTMEM..data: mapped to EXTMEM..V2P(data)
//                for the kernel's instructions and r/o data
//   data..KERNBASE+PHYSTOP: mapped to V2P(data)..PHYSTOP,
//                                  rw data + free physical memory
//   0xfe000000..0: mapped direct (devices such as ioapic)


// vm.cのL.29あたりのlgdtで設定されるべき内容が空になってしまっている
GDTEntry[0]={entryPhysAddr=0x0 segmentBaseAddr=0x0 segmentLimit=0x0 isCodeSegment=0x0}
GDTEntry[1]={entryPhysAddr=0x8 segmentBaseAddr=0x0 segmentLimit=0x0 isCodeSegment=0x0}
GDTEntry[2]={entryPhysAddr=0x10 segmentBaseAddr=0x0 segmentLimit=0x0 isCodeSegment=0x0}
GDTEntry[3]={entryPhysAddr=0x18 segmentBaseAddr=0x0 segmentLimit=0x0 isCodeSegment=0x0}
GDTEntry[4]={entryPhysAddr=0x20 segmentBaseAddr=0x0 segmentLimit=0x0 isCodeSegment=0x0}
GDTEntry[5]={entryPhysAddr=0x28 segmentBaseAddr=0x0 segmentLimit=0x0 isCodeSegment=0x0}

```
