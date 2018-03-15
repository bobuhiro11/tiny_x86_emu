package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"github.com/hajimehoshi/ebiten"
	"github.com/hajimehoshi/ebiten/ebitenutil"
	"image"
	"io/ioutil"
	"log"
	"math/rand"
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

func update(screen *ebiten.Image) error {
	for i := 0; i < width*height; i++ {
		vram.Pix[4*i] = uint8(rand.Int() & 0xFF)
		vram.Pix[4*i+1] = uint8(rand.Int() & 0xFF)
		vram.Pix[4*i+2] = uint8(rand.Int() & 0xFF)
		vram.Pix[4*i+3] = 0xff
	}
	if ebiten.IsRunningSlowly() {
	}
	screen.ReplacePixels(vram.Pix)
	ebitenutil.DebugPrint(screen, fmt.Sprintf("FPS: %f", ebiten.CurrentFPS()))
	return nil
}

func main() {
	runtime.LockOSThread()
	filename := flag.String("f", "", "binary filename (*.bin)")
	enableGUI := flag.Bool("gui", false, "gui mode")
	silent := flag.Bool("silent", false, "silent mode")
	flag.Parse()

	// load binary
	if *filename == "" {
		fmt.Fprintln(os.Stderr, "Please set filename")
		os.Exit(1)
	}
	// disasm binary
	b, err := exec.Command("ndisasm", "-b", "16", *filename, "-o", "0x7c00").CombinedOutput()
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

	bytes, err := loadFile(*filename)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	fmt.Printf("enable GUI = %#v\n", *enableGUI)
	fmt.Printf("bytes =\n%s", hex.Dump(bytes))

	// setup emulator
	e := NewEmulator(0x7c00+0x10000, 0x7c00, 0x8000, true, *silent, disasm)
	for i := 0; i < len(bytes); i++ {
		e.memory[i+0x7c00] = bytes[i]
	}

	// emulate
	chFinished := make(chan bool)
	go func(chFinished chan bool) {
		// time.Sleep(3000 * time.Millisecond)
		for e.eip < 0x7c00+0x10000 {
			if !*silent {
				e.dump()
			}
			err := e.execInst()
			if err != nil {
				fmt.Fprintln(os.Stderr, err.Error())
				os.Exit(1)
			}

			if e.eip == 0 || e.eip == 0x7c00 {
				break
			}
		}
		e.dump()
		fmt.Println("End of program")
		chFinished <- true
	}(chFinished)

	// setup gui
	if *enableGUI {
		err := ebiten.Run(update, width, height, 2, "x86 emulator")
		if err != nil {
			log.Fatal(err.Error())
		}
	}
	<-chFinished
}

func loadFile(filename string) ([]byte, error) {
	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}
