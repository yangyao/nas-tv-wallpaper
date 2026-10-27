package main

import (
	"context"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type orientedImage struct {
	image.Image
	orientation int
}

func (o orientedImage) Bounds() image.Rectangle {
	b := o.Image.Bounds()
	if o.orientation >= 5 {
		return image.Rect(0, 0, b.Dy(), b.Dx())
	}
	return image.Rect(0, 0, b.Dx(), b.Dy())
}
func (o orientedImage) At(x, y int) color.Color {
	b := o.Image.Bounds()
	w, h := b.Dx(), b.Dy()
	switch o.orientation {
	case 2:
		x = w - 1 - x
	case 3:
		x, y = w-1-x, h-1-y
	case 4:
		y = h - 1 - y
	case 5:
		x, y = y, x
	case 6:
		x, y = y, h-1-x
	case 7:
		x, y = w-1-y, h-1-x
	case 8:
		x, y = w-1-y, x
	}
	return o.Image.At(b.Min.X+x, b.Min.Y+y)
}
func scaled(src image.Image, w, h int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	bounds := src.Bounds()
	for y := 0; y < h; y++ {
		fy := (float64(y)+.5)*float64(bounds.Dy())/float64(h) - .5
		y0 := int(math.Floor(fy))
		dy := fy - float64(y0)
		if y0 < 0 {
			y0 = 0
			dy = 0
		}
		y1 := y0 + 1
		if y1 >= bounds.Dy() {
			y1 = bounds.Dy() - 1
		}
		for x := 0; x < w; x++ {
			fx := (float64(x)+.5)*float64(bounds.Dx())/float64(w) - .5
			x0 := int(math.Floor(fx))
			dx := fx - float64(x0)
			if x0 < 0 {
				x0 = 0
				dx = 0
			}
			x1 := x0 + 1
			if x1 >= bounds.Dx() {
				x1 = bounds.Dx() - 1
			}
			a := color.NRGBAModel.Convert(src.At(bounds.Min.X+x0, bounds.Min.Y+y0)).(color.NRGBA)
			b := color.NRGBAModel.Convert(src.At(bounds.Min.X+x1, bounds.Min.Y+y0)).(color.NRGBA)
			c := color.NRGBAModel.Convert(src.At(bounds.Min.X+x0, bounds.Min.Y+y1)).(color.NRGBA)
			d := color.NRGBAModel.Convert(src.At(bounds.Min.X+x1, bounds.Min.Y+y1)).(color.NRGBA)
			mix := func(a, b, c, d uint8) uint8 {
				return uint8((float64(a)*(1-dx)+float64(b)*dx)*(1-dy) + (float64(c)*(1-dx)+float64(d)*dx)*dy)
			}
			dst.SetNRGBA(x, y, color.NRGBA{mix(a.R, b.R, c.R, d.R), mix(a.G, b.G, c.G, d.G), mix(a.B, b.B, c.B, d.B), 255})
		}
	}
	return dst
}
func renderPhoto(ctx context.Context, path string, orientation int) (*image.NRGBA, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	source, e := jpeg.Decode(f)
	if e != nil {
		return nil, e
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	src := orientedImage{source, orientation}
	bounds := src.Bounds()
	const width, height = 1920, 1080
	// A tiny stretched, softened backdrop fills the sides; the full photo is never cropped.
	small := scaled(src, 32, 18)
	dst := scaled(small, width, height)
	for i := 0; i < len(dst.Pix); i += 4 {
		dst.Pix[i] /= 2
		dst.Pix[i+1] /= 2
		dst.Pix[i+2] /= 2
	}
	factor := math.Min(float64(width)/float64(bounds.Dx()), float64(height)/float64(bounds.Dy()))
	w, h := int(float64(bounds.Dx())*factor), int(float64(bounds.Dy())*factor)
	if w < 1 || h < 1 {
		return nil, errors.New("invalid image size")
	}
	foreground := scaled(src, w, h)
	draw.Draw(dst, image.Rect((width-w)/2, (height-h)/2, (width+w)/2, (height+h)/2), foreground, image.Point{}, draw.Src)
	return dst, nil
}
func (s *Sampler) imageFile(ctx context.Context, p Photo, cache string) (string, error) {
	path, e := s.safePath(p.Path)
	if e != nil {
		return "", e
	}
	info, e := os.Stat(path)
	if e != nil || info.Size() != p.Size || info.ModTime().UnixNano() != p.Modified {
		return "", errors.New("source changed")
	}
	out := filepath.Join(cache, p.ID+".jpg")
	if info, e := os.Stat(out); e == nil && info.Size() > 0 {
		return out, nil
	}
	if e = os.MkdirAll(cache, 0700); e != nil {
		return "", e
	}
	img, e := renderPhoto(ctx, path, p.Meta.Orientation)
	if e != nil {
		return "", e
	}
	tmp, e := os.CreateTemp(cache, ".photo-*")
	if e != nil {
		return "", e
	}
	defer os.Remove(tmp.Name())
	if e = jpeg.Encode(tmp, img, &jpeg.Options{Quality: 82}); e == nil {
		e = tmp.Sync()
	}
	tmp.Close()
	if e != nil {
		return "", e
	}
	if e = os.Rename(tmp.Name(), out); e != nil {
		return "", e
	}
	pruneImages(cache, 200<<20)
	return out, nil
}
func pruneImages(dir string, limit int64) {
	entries, e := os.ReadDir(dir)
	if e != nil {
		return
	}
	type entry struct {
		path     string
		size     int64
		modified int64
	}
	var files []entry
	var total int64
	for _, d := range entries {
		if !strings.HasSuffix(d.Name(), ".jpg") {
			continue
		}
		if i, e := d.Info(); e == nil {
			files = append(files, entry{filepath.Join(dir, d.Name()), i.Size(), i.ModTime().UnixNano()})
			total += i.Size()
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].modified < files[j].modified })
	for _, f := range files {
		if total <= limit {
			break
		}
		if os.Remove(f.path) == nil {
			total -= f.size
		}
	}
}
