package main

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"sync"

	imagedraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

//go:embed artwork/hover.png
var hoverPNG []byte

//go:embed artwork/claude-hover.png
var claudeHoverPNG []byte

//go:embed artwork/claude-idle.png
var claudeIdlePNG []byte

//go:embed artwork/idle.png
var idlePNG []byte

const (
	barPadding     = 4
	barItem        = 92
	barItemNumbers = 148
	hoverFrames    = 40
	hoverCell      = 80
	hoverColumns   = 8
	markRadius     = 29
)

var hoverAtlas image.Image
var barFont font.Face

// Every provider mark uses the same cell and target size. Reuse the
// high-quality filter weights and its scratch buffer between animation frames.
var markScaler = imagedraw.CatmullRom.NewScaler(2*markRadius, 2*markRadius, hoverCell, hoverCell)

var idleAtlas = sync.OnceValue(func() image.Image { return decodeAtlas(idlePNG) })
var claudeHoverAtlas = sync.OnceValue(func() image.Image { return decodeAtlas(claudeHoverPNG) })
var claudeIdleAtlas = sync.OnceValue(func() image.Image { return decodeAtlas(claudeIdlePNG) })

func decodeAtlas(data []byte) image.Image {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		panic(err)
	}
	return img
}

func init() {
	hoverAtlas = decodeAtlas(hoverPNG)
	parsed, e := opentype.Parse(goregular.TTF)
	if e != nil {
		panic(e)
	}
	barFont, e = opentype.NewFace(parsed, &opentype.FaceOptions{Size: 16, DPI: 72, Hinting: font.HintingFull})
	if e != nil {
		panic(e)
	}
}

func barGeometry(p placement) (w, h, step int) {
	step = barItem
	if p.Numbers && p.Edge%2 == 0 {
		step = barItemNumbers
	}
	if p.Edge%2 == 0 {
		return 2*barPadding + p.slotCount()*step, 2*barPadding + barItem, step
	}
	w = barItem
	if p.Numbers {
		w = barItemNumbers
	}
	return w, 2*barPadding + p.slotCount()*barItem, step
}

// Native display draw calls consume an opaque BGRA image; window shapes
// clip the pill's corners. Rings are drawn with fractional pixel coverage.
func paintBar(rows []row, p placement, frames markFrames, dst *image.RGBA) *image.RGBA {
	w, h, step := barGeometry(p)
	if dst == nil || dst.Bounds().Dx() != w || dst.Bounds().Dy() != h {
		dst = image.NewRGBA(image.Rect(0, 0, w, h))
	}
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.RGBA{5, 7, 9, 255}), image.Point{}, draw.Src)
	for i, r := range rows[:p.slotCount()] {
		left, top := barPadding+i*step, barPadding
		if p.Edge%2 != 0 {
			left, top = 0, barPadding+i*barItem
		}
		cx, cy := left+barItem/2, top+barItem/2
		q := color.RGBA{55, 66, 76, 255}
		if r.Percent >= 0 {
			switch {
			case r.Stale:
				q = color.RGBA{135, 145, 154, 255}
			case r.Percent <= 15:
				q = color.RGBA{255, 104, 120, 255}
			case r.Percent <= 30:
				q = color.RGBA{255, 197, 94, 255}
			default:
				q = color.RGBA{45, 217, 128, 255}
			}
		}

		const radius, halfStroke = 28.5, 3.3
		for yy := cy - 33; yy <= cy+33; yy++ {
			for xx := cx - 33; xx <= cx+33; xx++ {
				dx, dy := float64(xx-cx)+.5, float64(yy-cy)+.5
				rad := math.Hypot(dx, dy)
				d := math.Abs(rad - radius)
				track := clamp01(halfStroke + .5 - d)
				if track == 0 {
					continue
				}
				px := blend(dst.RGBAAt(xx, yy), color.RGBA{30, 32, 33, 255}, track)
				if r.Percent > 0 {
					delta := ringGap(math.Atan2(dx, -dy), r.Percent)
					if delta > 0 {
						d = math.Sqrt(d*d + 2*rad*radius*(1-math.Cos(delta)))
					}
					px = blend(px, q, clamp01(halfStroke+.5-d))
				}
				dst.SetRGBA(xx, yy, px)
			}
		}
		slot := [...]int{0, 1, 1, 2, 0}[i]
		frame, count, atlas := frames[i], hoverFrames, hoverAtlas
		if i == 4 {
			atlas = claudeHoverAtlas()
			if frame < 0 {
				frame, count, atlas = -frame-1, idleFrames, claudeIdleAtlas()
			}
		} else if frame < 0 {
			frame, count, atlas = -frame-1, idleFrames, idleAtlas()
		}
		x := frame % hoverColumns * hoverCell
		y := (slot*count/hoverColumns + frame/hoverColumns) * hoverCell
		markScaler.Scale(dst, image.Rect(cx-markRadius, cy-markRadius, cx+markRadius, cy+markRadius),
			atlas, image.Rect(x, y, x+hoverCell, y+hoverCell), draw.Over, nil)
		if p.Numbers {
			ink := q
			if r.Percent < 0 {
				ink = color.RGBA{165, 174, 181, 255}
			}
			d := font.Drawer{Dst: dst, Src: image.NewUniform(ink), Face: barFont, Dot: fixed.P(left+barItem-2, cy+6)}
			d.DrawString(r.Number)
		}
	}
	// The targets use little-endian 32-bit BGRX pixels.
	for i := 0; i < len(dst.Pix); i += 4 {
		dst.Pix[i], dst.Pix[i+2] = dst.Pix[i+2], dst.Pix[i]
	}
	return dst
}

// Remaining quota fills clockwise from twelve o'clock. As it drops the
// rounded end retreats counterclockwise, opening a gap from the upper-left.
func ringGap(angle float64, percent int) float64 {
	if angle < 0 {
		angle += 2 * math.Pi
	}
	end := 2 * math.Pi * float64(percent) / 100
	return max(0, min(angle-end, 2*math.Pi-angle))
}
func clamp01(a float64) float64 { return max(0, min(1, a)) }
func blend(a, b color.RGBA, amount float64) color.RGBA {
	ch := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*amount + .5) }
	return color.RGBA{ch(a.R, b.R), ch(a.G, b.G), ch(a.B, b.B), 255}
}
