package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"math"
	"os"
)

func main() {
	inPath := flag.String("in", "salute.jpg", "source image path")
	outPath := flag.String("out", "app.ico", "output icon path")
	flag.Parse()

	file, err := os.Open(*inPath)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	src, err := jpeg.Decode(file)
	if err != nil {
		panic(err)
	}

	icon, err := buildICO(src, []int{16, 32, 48, 256})
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(*outPath, icon, 0644); err != nil {
		panic(err)
	}
}

func buildICO(src image.Image, sizes []int) ([]byte, error) {
	images := make([][]byte, len(sizes))
	for i, size := range sizes {
		resized := resizeCover(src, size)
		var pngData bytes.Buffer
		if err := png.Encode(&pngData, resized); err != nil {
			return nil, err
		}
		images[i] = pngData.Bytes()
	}

	var out bytes.Buffer
	writeLE(&out, uint16(0))
	writeLE(&out, uint16(1))
	writeLE(&out, uint16(len(sizes)))

	offset := 6 + len(sizes)*16
	for i, size := range sizes {
		entrySize := byte(size)
		if size == 256 {
			entrySize = 0
		}
		out.WriteByte(entrySize)
		out.WriteByte(entrySize)
		out.WriteByte(0)
		out.WriteByte(0)
		writeLE(&out, uint16(1))
		writeLE(&out, uint16(32))
		writeLE(&out, uint32(len(images[i])))
		writeLE(&out, uint32(offset))
		offset += len(images[i])
	}

	for _, img := range images {
		out.Write(img)
	}
	return out.Bytes(), nil
}

func resizeCover(src image.Image, size int) *image.NRGBA {
	crop := squareCrop(src)
	out := image.NewNRGBA(image.Rect(0, 0, size, size))
	cw := crop.Bounds().Dx()
	ch := crop.Bounds().Dy()

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			sx := (float64(x)+0.5)*float64(cw)/float64(size) - 0.5
			sy := (float64(y)+0.5)*float64(ch)/float64(size) - 0.5
			out.SetNRGBA(x, y, sample(crop, sx, sy))
		}
	}
	return out
}

func squareCrop(src image.Image) *image.NRGBA {
	b := src.Bounds()
	size := minInt(b.Dx(), b.Dy())
	x0 := b.Min.X + (b.Dx()-size)/2
	y0 := b.Min.Y + (b.Dy()-size)/2
	crop := image.NewNRGBA(image.Rect(0, 0, size, size))
	draw.Draw(crop, crop.Bounds(), src, image.Point{X: x0, Y: y0}, draw.Src)
	return crop
}

func sample(img *image.NRGBA, x, y float64) color.NRGBA {
	b := img.Bounds()
	w := b.Dx()
	h := b.Dy()
	x = clamp(x, 0, float64(w-1))
	y = clamp(y, 0, float64(h-1))
	x0 := int(math.Floor(x))
	y0 := int(math.Floor(y))
	x1 := minInt(x0+1, w-1)
	y1 := minInt(y0+1, h-1)
	tx := x - float64(x0)
	ty := y - float64(y0)

	c00 := img.NRGBAAt(x0, y0)
	c10 := img.NRGBAAt(x1, y0)
	c01 := img.NRGBAAt(x0, y1)
	c11 := img.NRGBAAt(x1, y1)

	return color.NRGBA{
		R: byte(bilerp(float64(c00.R), float64(c10.R), float64(c01.R), float64(c11.R), tx, ty)),
		G: byte(bilerp(float64(c00.G), float64(c10.G), float64(c01.G), float64(c11.G), tx, ty)),
		B: byte(bilerp(float64(c00.B), float64(c10.B), float64(c01.B), float64(c11.B), tx, ty)),
		A: byte(bilerp(float64(c00.A), float64(c10.A), float64(c01.A), float64(c11.A), tx, ty)),
	}
}

func bilerp(c00, c10, c01, c11, tx, ty float64) int {
	top := c00 + (c10-c00)*tx
	bottom := c01 + (c11-c01)*tx
	return int(math.Round(clamp(top+(bottom-top)*ty, 0, 255)))
}

func clamp(v, low, high float64) float64 {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func writeLE(buf *bytes.Buffer, value any) {
	if err := binary.Write(buf, binary.LittleEndian, value); err != nil {
		panic(err)
	}
}
