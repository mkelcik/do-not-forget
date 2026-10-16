//go:build ignore

// Draws the macOS app icon: the tray calendar (icon.go) on a dark rounded square, in every
// size an .iconset needs, and packs it with iconutil.
//
//	go run scripts/appicon.go   # writes assets/AppIcon.icns
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
)

// The calendar on its 22-unit grid, as in icon.go and docs/logo.svg: {x, y, w, h}.
var (
	glyph = [][4]float64{
		{6, 2, 2, 2}, {14, 2, 2, 2}, // binder rings
		{3, 4, 16, 3},                                 // header
		{3, 7, 1, 12}, {18, 7, 1, 12}, {3, 18, 16, 1}, // frame
		{6, 9, 2, 2}, {10, 9, 2, 2}, {14, 9, 2, 2}, {6, 13, 2, 2}, {14, 13, 2, 2}, // days
	}
	today = [4]float64{10, 13, 2, 2}

	bg     = color.NRGBA{0x1c, 0x1c, 0x1e, 0xff}
	fg     = color.NRGBA{0xfc, 0xfc, 0xfc, 0xff}
	accent = color.NRGBA{0x3d, 0xae, 0xe9, 0xff}
)

// render draws one size. Like Apple's template, the tile is 824/1024 of the canvas with
// rounded corners; each pixel is supersampled for smooth edges.
func render(size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	s := float64(size) / 1024
	tile, inset, radius := 824*s, 100*s, 185*s
	unit := tile / 22
	inRect := func(x, y float64, r [4]float64) bool {
		return x >= inset+r[0]*unit && x < inset+(r[0]+r[2])*unit && y >= inset+r[1]*unit && y < inset+(r[1]+r[3])*unit
	}
	inTile := func(x, y float64) bool {
		dx := math.Max(math.Max(inset+radius-x, x-(inset+tile-radius)), 0)
		dy := math.Max(math.Max(inset+radius-y, y-(inset+tile-radius)), 0)
		return x >= inset && x <= inset+tile && y >= inset && y <= inset+tile && dx*dx+dy*dy <= radius*radius
	}
	const n = 4 // samples per axis
	for py := range size {
		for px := range size {
			var r, g, b, a float64
			for sy := range n {
				for sx := range n {
					x, y := float64(px)+(float64(sx)+.5)/n, float64(py)+(float64(sy)+.5)/n
					if !inTile(x, y) {
						continue
					}
					c := bg
					if inRect(x, y, today) {
						c = accent
					} else {
						for _, rc := range glyph {
							if inRect(x, y, rc) {
								c = fg
								break
							}
						}
					}
					r, g, b, a = r+float64(c.R), g+float64(c.G), b+float64(c.B), a+1
				}
			}
			if a > 0 {
				img.SetNRGBA(px, py, color.NRGBA{uint8(r / a), uint8(g / a), uint8(b / a), uint8(255 * a / (n * n))})
			}
		}
	}
	return img
}

func main() {
	tmp, err := os.MkdirTemp("", "appicon-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmp)
	iconset := filepath.Join(tmp, "AppIcon.iconset") // iconutil wants the .iconset suffix
	if err := os.Mkdir(iconset, 0o755); err != nil {
		log.Fatal(err)
	}

	for _, pt := range []int{16, 32, 128, 256, 512} {
		for _, scale := range []int{1, 2} {
			name := fmt.Sprintf("icon_%dx%d.png", pt, pt)
			if scale == 2 {
				name = fmt.Sprintf("icon_%dx%d@2x.png", pt, pt)
			}
			f, err := os.Create(filepath.Join(iconset, name))
			if err != nil {
				log.Fatal(err)
			}
			if err := png.Encode(f, render(pt*scale)); err != nil {
				log.Fatal(err)
			}
			_ = f.Close()
		}
	}
	if err := os.MkdirAll("assets", 0o755); err != nil {
		log.Fatal(err)
	}
	out, err := exec.Command("iconutil", "-c", "icns", iconset, "-o", "assets/AppIcon.icns").CombinedOutput()
	if err != nil {
		log.Fatalf("iconutil: %v\n%s", err, out)
	}
	log.Print("wrote assets/AppIcon.icns")
}
