package main

import (
	"bytes"
	"embed"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"

	imagedraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

//go:embed artwork/*.png
var artwork embed.FS

const (
	barPadding     = 7
	barItem        = 70
	barItemNumbers = 120
)

var logos [4]image.Image
var barFont font.Face

func init() {
	for i, name := range []string{"chatgpt", "antigravity", "antigravity", "deepseek"} {
		if i == 2 {
			logos[i] = logos[1]
			continue
		}
		data, err := artwork.ReadFile("artwork/" + name + ".png")
		if err != nil {
			panic(err)
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			panic(err)
		}
		// Remove transparent borders so visibly sized marks match the reference.
		b := img.Bounds()
		minX, minY, maxX, maxY := b.Max.X, b.Max.Y, b.Min.X, b.Min.Y
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				_, _, _, a := img.At(x, y).RGBA()
				if a <= 256 {
					continue
				}
				minX, minY, maxX, maxY = min(x, minX), min(y, minY), max(x+1, maxX), max(y+1, maxY)
			}
		}
		w, h := maxX-minX, maxY-minY
		extent := max(w, h)
		trimmed := image.NewRGBA(image.Rect(0, 0, max(1, 128*w/extent), max(1, 128*h/extent)))
		imagedraw.CatmullRom.Scale(trimmed, trimmed.Bounds(), img, image.Rect(minX, minY, maxX, maxY), draw.Src, nil)
		if i == 0 { // upstream renders the OpenAI template in teal
			for y := 0; y < trimmed.Bounds().Dy(); y++ {
				for x := 0; x < trimmed.Bounds().Dx(); x++ {
					a := trimmed.RGBAAt(x, y).A
					trimmed.Set(x, y, color.NRGBA{0x10, 0xa3, 0x7f, a})
				}
			}
		}
		logos[i] = trimmed
	}
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
		return 2*barPadding + 4*step, 2*barPadding + barItem, step
	}
	w = barItem
	if p.Numbers {
		w = barItemNumbers
	}
	return w + 2*barPadding, 2*barPadding + 4*barItem, step
}

// Native display draw calls consume an opaque BGRA image; window shapes
// clip the pill's corners. Rings are drawn with fractional pixel coverage.
func paintBar(rows []row, p placement, hover int, phase float64, dst *image.RGBA) *image.RGBA {
	w, h, step := barGeometry(p)
	if dst == nil || dst.Bounds().Dx() != w || dst.Bounds().Dy() != h {
		dst = image.NewRGBA(image.Rect(0, 0, w, h))
	}
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.RGBA{5, 7, 9, 255}), image.Point{}, draw.Src)
	for i, r := range rows {
		left, top := barPadding+i*step, barPadding
		if p.Edge%2 != 0 {
			left, top = barPadding, barPadding+i*barItem
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
		glow := 0.0
		if hover == i {
			glow = phase
		}
		angles := []float64{0}
		if i == 1 || i == 2 {
			angles = []float64{0, 90, 180, 270}
		}
		for yy := cy - 33; yy <= cy+33; yy++ {
			for xx := cx - 33; xx <= cx+33; xx++ {
				dx, dy := float64(xx-cx)+.5, float64(yy-cy)+.5
				rad := math.Hypot(dx, dy)
				px := dst.RGBAAt(xx, yy)
				if glow > 0 {
					px = blend(px, color.RGBA{33, 41, 45, 255}, glow*clamp01((34-rad)/4))
				}
				ring := clamp01(2.7 - math.Abs(rad-27.5))
				if ring > 0 {
					a := math.Atan2(dx, -dy) * 180 / math.Pi
					if a < 0 {
						a += 360
					}
					segment := min(len(angles)-1, int(a/360*float64(len(angles))))
					offset := a - angles[segment]
					percent := r.Percent
					if len(angles) > 1 {
						percent = -1
						if segment < len(r.Segments) {
							percent = r.Segments[segment]
						}
					}
					span := 360 / float64(len(angles))
					gap := 0.0
					if len(angles) > 1 {
						gap = 5
					}
					track := 1.0
					if gap > 0 {
						track = clamp01((math.Min(offset, span-offset)-gap)/2 + .5)
					}
					px = blend(px, color.RGBA{33, 41, 45, 255}, ring*track)
					if percent > 0 {
						active := 1.0
						if percent < 100 {
							cutoff := gap + float64(percent)*(span-2*gap)/100
							active = clamp01((cutoff-offset)/2 + .5)
						}
						px = blend(px, q, ring*track*active)
					}
				}
				dst.SetRGBA(xx, yy, px)
			}
		}
		size := int(math.Round(34 + 4*glow))
		src := logos[i]
		if src.Bounds().Dx() > 0 && src.Bounds().Dy() > 0 {
			iw, ih := size, size
			if src.Bounds().Dx() > src.Bounds().Dy() {
				ih = size * src.Bounds().Dy() / src.Bounds().Dx()
			} else {
				iw = size * src.Bounds().Dx() / src.Bounds().Dy()
			}
			x, y := cx-iw/2, cy-ih/2
			imagedraw.CatmullRom.Scale(dst, image.Rect(x, y, x+iw, y+ih), src, src.Bounds(), draw.Over, nil)
		}
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
func clamp01(a float64) float64 { return max(0, min(1, a)) }
func blend(a, b color.RGBA, amount float64) color.RGBA {
	ch := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*amount + .5) }
	return color.RGBA{ch(a.R, b.R), ch(a.G, b.G), ch(a.B, b.B), 255}
}
