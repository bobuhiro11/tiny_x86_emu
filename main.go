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
	"runtime"
	"time"
)

const (
	height = 120
	width  = 160
)

var (
	vram = image.NewRGBA(image.Rect(0, 0, width, height))
)

func update(screen *ebiten.Image) error {
	time.Sleep(30 * time.Millisecond)
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
	filename := flag.String("f", "", "binary filename")
	enableGUI := flag.Bool("gui", false, "gui mode")
	flag.Parse()

	// load binary
	if *filename == "" {
		fmt.Fprintln(os.Stderr, "Please set filename")
		os.Exit(1)
	}
	bytes, err := loadFile(*filename)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	fmt.Printf("bytes =\n%s\n", hex.Dump(bytes))

	// setup emulator
	e := NewEmulator(10000, 0, 100)
	for i := 0; i < len(bytes); i++ {
		e.memory[i] = bytes[i]
	}

	// setup gui
	fmt.Printf("enable GUI = %#v\n", *enableGUI)
	chFinished := make(chan bool)
	if *enableGUI {
		go func(chFinished chan bool) {
			err := ebiten.Run(update, width, height, 2, "x86 emulator")
			if err != nil {
				log.Fatal(err.Error())
			}
			chFinished <- true
		}(chFinished)
	}

	// emulate
	for e.eip < 10000 {
		err := e.exec_inst()
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}

		if e.eip == 0 {
			fmt.Println("End of program")
			e.dump()
			break
		}
	}

	if *enableGUI {
		// wait for window closed
		<-chFinished
	}
}

func loadFile(filename string) ([]byte, error) {
	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}
