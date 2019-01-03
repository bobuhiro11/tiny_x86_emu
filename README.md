# tiny_x86_emu [![wercker status](https://app.wercker.com/status/7ac504b68746c744dd7dc4b5e52e4735/s/master "wercker status")](https://app.wercker.com/project/byKey/7ac504b68746c744dd7dc4b5e52e4735)

This is an experimental x86 emulator. Currently, this project is targeted only to xv6 as guest OS.

## Demo

https://bobuhiro11.net/tiny_x86_emu/

![screenshot](https://raw.githubusercontent.com/nmi/tiny_x86_emu/master/screenshot.png)

## preparation

Please make sure that make, go (>1.11), gcc, objdump, nasm and ndisasm are installed.

## Usage

`make` command will build two version of emulators and xv6 image described as below:
- Emulator for Host OS: An emulator execution binary for the host OS is created. Its name is `tiny_x86_emu`.
- Emulator for wasm: A wasm version of the emulator is built to run in the browser. Its name is `wasm/tiny_x86_emu.wasm`.
- xv6 image: The makefile for xv6 is also executed recursively.

```bash
# Build two version of emulators and guest xv6 image
$ make

# Execute CLI version emulator in your terminal.
$ ./tiny_x86_emu -f xv6-public/xv6.img

# Start web server to host wasm file.
# Then, please open http://localhost:8000/wasm in your browser.
$ ./httpserv.py
```

## Testing

`make test` command will execute all tests.

## Contribution

Pull requests from anyone are welcome!
