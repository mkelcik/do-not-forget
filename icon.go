package main

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
)

// trayIcon draws a Breeze-style calendar on the 22px icon grid, rendered at 2x.
// Muted: dimmed calendar crossed by a red slash, like Breeze's muted-audio icons.
// ponytail: white only (dark panels); add a dark variant when someone runs a light theme.
func trayIcon(muted bool) []byte {
	const scale = 2
	fg := color.RGBA{0xfc, 0xfc, 0xfc, 0xff}
	accent := color.RGBA{0x3d, 0xae, 0xe9, 0xff} // Breeze highlight blue
	if muted {
		fg = color.RGBA{0x7e, 0x7e, 0x7e, 0x7e} // premultiplied: white at ~50%
		accent = fg
	}

	img := image.NewRGBA(image.Rect(0, 0, 22*scale, 22*scale))
	fill := func(x0, y0, x1, y1 int, c color.Color) {
		r := image.Rect(x0*scale, y0*scale, x1*scale, y1*scale)
		draw.Draw(img, r, image.NewUniform(c), image.Point{}, draw.Src)
	}

	fill(6, 2, 8, 4, fg) // binder rings
	fill(14, 2, 16, 4, fg)
	fill(3, 4, 19, 7, fg)   // header
	fill(3, 7, 4, 19, fg)   // left
	fill(18, 7, 19, 19, fg) // right
	fill(3, 18, 19, 19, fg) // bottom
	for _, y := range []int{9, 13} {
		for _, x := range []int{6, 10, 14} {
			fill(x, y, x+2, y+2, fg)
		}
	}
	fill(10, 13, 12, 15, accent) // "next meeting" day

	if muted {
		// Slash from top-left to bottom-right, with a transparent gap so it reads on any panel.
		red := color.RGBA{0xda, 0x44, 0x53, 0xff} // Breeze negative
		size := 22 * scale
		for y := range size {
			for x := range size {
				d := math.Abs(float64(x-y)) / math.Sqrt2
				inside := x+y >= 4*scale && x+y <= 2*size-4*scale
				switch {
				case inside && d <= 1.6*scale:
					img.Set(x, y, red)
				case inside && d <= 2.6*scale:
					img.Set(x, y, color.Transparent)
				}
			}
		}
	}

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
