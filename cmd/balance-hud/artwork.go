package main

import (
	_ "embed"
	"image"
	"image/color"
	"image/draw"
	"math"
	"sync"

	"github.com/ExileOlder/agent-hud-open/internal/markatlas"
)

//go:embed artwork/hover.atlas
var hoverData []byte

//go:embed artwork/claude-hover.atlas
var claudeHoverData []byte

//go:embed artwork/claude-idle.atlas
var claudeIdleData []byte

//go:embed artwork/idle.atlas
var idleData []byte

//go:embed artwork/work.atlas
var workData []byte

//go:embed artwork/claude-work.atlas
var claudeWorkData []byte

//go:embed artwork/tool.atlas
var toolData []byte

//go:embed artwork/cue-question.atlas
var cueQuestionData []byte

//go:embed artwork/cue-permission.atlas
var cuePermissionData []byte

//go:embed artwork/cue-done.atlas
var cueDoneData []byte

const (
	barPadding     = 4
	barItem        = 92
	barItemNumbers = 148
	hoverFrames    = 96
	markRadius     = markatlas.Cell / 2
)

var hoverAtlas = sync.OnceValue(func() markatlas.Atlas { return decodeAtlas(hoverData) })
var idleAtlas = sync.OnceValue(func() markatlas.Atlas { return decodeAtlas(idleData) })
var claudeHoverAtlas = sync.OnceValue(func() markatlas.Atlas { return decodeAtlas(claudeHoverData) })
var claudeIdleAtlas = sync.OnceValue(func() markatlas.Atlas { return decodeAtlas(claudeIdleData) })

var workAtlas = sync.OnceValue(func() markatlas.Atlas { return decodeAtlas(workData) })
var claudeWorkAtlas = sync.OnceValue(func() markatlas.Atlas { return decodeAtlas(claudeWorkData) })

// The upstream procedural performer shares this pose across tool kinds.
// Keep a single atlas, not one duplicate sheet per status.
var toolAtlas = sync.OnceValue(func() markatlas.Atlas { return decodeAtlas(toolData) })

var cueAtlas = [...]func() markatlas.Atlas{
	sync.OnceValue(func() markatlas.Atlas { return decodeAtlas(cueQuestionData) }),
	sync.OnceValue(func() markatlas.Atlas { return decodeAtlas(cuePermissionData) }),
	sync.OnceValue(func() markatlas.Atlas { return decodeAtlas(cueDoneData) }),
}

func decodeAtlas(data []byte) markatlas.Atlas {
	img, err := markatlas.Open(data)
	if err != nil {
		panic(err)
	}
	return img
}

// Shared with the Xft percentage palette. The missing ring is dimmer than
// its label; Windows also uses this table for GDI quota figures.
var quotaInks = [...]color.RGBA{
	{55, 66, 76, 255}, {135, 145, 154, 255}, {255, 104, 120, 255},
	{255, 197, 94, 255}, {45, 217, 128, 255},
}

func (r row) quotaTone() int {
	switch {
	case r.Percent < 0:
		return 0
	case r.Stale:
		return 1
	case r.Percent <= 15:
		return 2
	case r.Percent <= 30:
		return 3
	default:
		return 4
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

type paintedMark struct{ percent, tone, frame int }

type barPainter struct {
	bitmap   *image.RGBA
	geometry [3]int
	vertical bool
	marks    [5]paintedMark
}

var dockRGBA = image.NewUniform(color.RGBA{5, 7, 9, 255})
var dockBGRA = image.NewUniform(color.RGBA{9, 7, 5, 255})

// Both native windows consume the same BGRX bitmap. Repaint the mark that
// changed in place; the surrounding background and other marks are unchanged.
func (b *barPainter) paint(rows []row, p placement, frames markFrames) *image.RGBA {
	w, h, step := barGeometry(p)
	geometry := [3]int{w, h, step}
	vertical := p.Edge%2 != 0
	full := b.bitmap == nil || b.geometry != geometry || b.vertical != vertical
	if full {
		if b.bitmap == nil || b.bitmap.Rect.Dx() != w || b.bitmap.Rect.Dy() != h {
			b.bitmap = image.NewRGBA(image.Rect(0, 0, w, h))
		}
		b.geometry, b.vertical = geometry, vertical
		draw.Draw(b.bitmap, b.bitmap.Bounds(), dockBGRA, image.Point{}, draw.Src)
	}
	dst := b.bitmap
	for i, r := range rows[:p.slotCount()] {
		mark := paintedMark{r.Percent, r.quotaTone(), frames[i]}
		if !full && mark == b.marks[i] {
			continue
		}
		b.marks[i] = mark
		left, top := barPadding+i*step, barPadding
		if vertical {
			left, top = 0, barPadding+i*barItem
		}
		cx, cy := left+barItem/2, top+barItem/2
		const patchRadius = 33
		patch := image.Rect(cx-patchRadius, cy-patchRadius, cx+patchRadius+1, cy+patchRadius+1)
		draw.Draw(dst, patch, dockRGBA, image.Point{}, draw.Src)
		q := quotaInks[mark.tone]

		const radius, halfStroke = 28.5, 3.3
		for yy := patch.Min.Y; yy < patch.Max.Y; yy++ {
			for xx := patch.Min.X; xx < patch.Max.X; xx++ {
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
		frame, count := frames[i], hoverFrames
		var atlas markatlas.Atlas
		if frame >= pendingBase {
			look := (frame - pendingBase) / waitAtlasFrames
			frame, count, atlas = (frame-pendingBase)%waitAtlasFrames, waitAtlasFrames, cueAtlas[look]()
			if i == 4 {
				slot = 3
			}
		} else if frame >= cueBase {
			look := (frame - cueBase) / cueFrames
			frame, count, atlas = (frame-cueBase)%cueFrames, cueFrames, cueAtlas[look]()
			if look != int(cueDone) {
				count = waitAtlasFrames
			}
			if i == 4 {
				slot = 3
			}
		} else if frame >= toolBase {
			frame, count, atlas = (frame-toolBase)%toolFrames, toolFrames, toolAtlas()
			if i == 4 {
				slot = 3
			}
		} else if frame >= workBase {
			frame, count = frame-workBase, workFrames
			if i == 4 {
				atlas = claudeWorkAtlas()
			} else {
				atlas = workAtlas()
			}
		} else if i == 4 {
			atlas = claudeHoverAtlas()
			if frame < 0 {
				frame, count, atlas = -frame-1, idleFrames, claudeIdleAtlas()
			}
		} else if frame < 0 {
			frame, count, atlas = -frame-1, idleFrames, idleAtlas()
		}
		if atlas.Len() == 0 {
			atlas = hoverAtlas()
		}
		atlas.Draw(dst, image.Pt(cx-markRadius, cy-markRadius), slot*count+frame)
		for y := patch.Min.Y; y < patch.Max.Y; y++ {
			start := dst.PixOffset(patch.Min.X, y)
			pixels := dst.Pix[start : start+4*patch.Dx()]
			for k := 0; k < len(pixels); k += 4 {
				pixels[k], pixels[k+2] = pixels[k+2], pixels[k]
			}
		}
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
