package main

import (
	// "encoding/hex"
	"flag"
	"fmt"
	// "github.com/hajimehoshi/ebiten"
	// "github.com/hajimehoshi/ebiten/ebitenutil"
	"image"
	"io/ioutil"
	// "log"
	// "math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	// "path/filepath"
	"runtime"
	// "time"
)

const (
	height = 120
	width  = 160
)

var (
	vram = image.NewRGBA(image.Rect(0, 0, width, height))
)

// func update(screen *ebiten.Image) error {
// 	for i := 0; i < width*height; i++ {
// 		vram.Pix[4*i] = uint8(rand.Int() & 0xFF)
// 		vram.Pix[4*i+1] = uint8(rand.Int() & 0xFF)
// 		vram.Pix[4*i+2] = uint8(rand.Int() & 0xFF)
// 		vram.Pix[4*i+3] = 0xff
// 	}
// 	if ebiten.IsRunningSlowly() {
// 	}
// 	screen.ReplacePixels(vram.Pix)
// 	ebitenutil.DebugPrint(screen, fmt.Sprintf("FPS: %f", ebiten.CurrentFPS()))
// 	return nil
// }

func main() {
	runtime.LockOSThread()
	filename := flag.String("f", "", "binary filename (*.bin)")
	// enableGUI := flag.Bool("gui", false, "gui mode")
	silent := flag.Bool("silent", false, "silent mode")
	flag.Parse()

	// load binary
	if *filename == "" {
		fmt.Fprintln(os.Stderr, "Please set filename")
		os.Exit(1)
	}
	// disasm binary
	exec.Command("sh", "-c", "head -c 49  "+*filename+" | ndisasm -b 16 -o 0x7c00 - |  tee disasm16.txt").Run()               // 16 bit mode
	exec.Command("sh", "-c", "tail -c +50 "+*filename+" | ndisasm -b 32 -o 0x7c31 - | head -n 5000 | tee disasm32.txt").Run() // 32 bit mode
	b, err := exec.Command("sh", "-c", "cat disasm16.txt disasm32.txt").CombinedOutput()
	disasm := map[uint64]string{}
	if err != nil {
		panic(err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		row := strings.Fields(line)
		if len(row) < 3 {
			continue
		}
		ix, _ := strconv.ParseUint(row[0][2:], 16, 64)
		disasm[ix] = strings.Join(row[1:], " ")
	}
	// disasm binary of ./xv6-public/kernel
	b, err = exec.Command("sh", "-c", "objdump -d ./xv6-public/kernel | tail -n +7 | grep -E \"[0-9a-f]{8}:\"").CombinedOutput()
	if err != nil {
		panic(err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		if len(line) < 7 {
			continue
		}
		ix, err := strconv.ParseUint(line[:8], 16, 64)
		if err != nil {
			panic(err)
		}
		disasm[ix] = line[10:]
		disasm[ix-0x80000000] = line[10:]
	}

	bytes, err := LoadFile(*filename)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	// fmt.Printf("enable GUI = %#v\n", *enableGUI)
	fmt.Printf("len(bytes) = %d\n", len(bytes))
	// fmt.Printf("bytes =\n%s", hex.Dump(bytes))

	// setup emulator
	e := NewEmulator(0x7c00+0x10240000, 0x7c00, 0x6f04, false, *silent, os.Stdin, os.Stdout, disasm)
	for i := 0; i < len(bytes); i++ {
		e.memory[uint32(i+0x7c00)] = bytes[i]
	}
	e.io.hdds[0], _ = os.Open(*filename)

	// emulate
	// chFinished := make(chan bool)
	// go func(chFinished chan bool) {
	// time.Sleep(3000 * time.Millisecond)
	// for e.eip < 0x7c00+0x200000 {
	i := 0
	for {
		// if !*silent && 0x80100500 < e.eip && e.eip < 0x801005ff {
		if !*silent && i > 3635000 {
			e.dump(i)
		}
		err := e.execInst()
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}

		if e.eip == 0 || e.eip == 0x7c00 {
			break
		}
		i++
	}
	if !*silent {
		e.dump(i)
	}
	fmt.Println("End of program")
	// chFinished <- true
	// }(chFinished)

	// setup gui
	// if *enableGUI {
	// 	err := ebiten.Run(update, width, height, 2, "x86 emulator")
	// 	if err != nil {
	// 		log.Fatal(err.Error())
	// 	}
	// }
	// <-chFinished
}

// LoadFile by filename
func LoadFile(filename string) ([]byte, error) {
	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}
