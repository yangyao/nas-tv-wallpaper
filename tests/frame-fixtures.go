package main

import (
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
)

func main() {
	for i := 0; i < 32; i++ {
		w, h := 1200, 800
		if i%2 == 1 {
			w, h = 800, 1200
		}
		m := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				c := color.RGBA{uint8((50 + i*37) % 210), uint8((80 + i*53) % 210), uint8((110 + i*23) % 210), 255}
				dx, dy := x-w/2, y-h/2
				if dx*dx+dy*dy < (w/5)*(w/5) {
					c = color.RGBA{240, 220, 180, 255}
				}
				m.SetRGBA(x, y, c)
			}
		}
		dir := filepath.Join("/tmp/nas-frame-fixtures", fmt.Sprintf("set-%02d", i/2))
		os.MkdirAll(dir, 0700)
		f, _ := os.Create(filepath.Join(dir, fmt.Sprintf("photo-%02d.jpg", i)))
		jpeg.Encode(f, m, &jpeg.Options{Quality: 85})
		f.Close()
	}
}
