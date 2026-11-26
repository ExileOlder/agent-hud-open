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
		q := quotaInks[r.quotaTone()]

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
