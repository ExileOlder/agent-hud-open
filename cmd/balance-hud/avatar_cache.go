package main

import (
	"image"
	"image/draw"
	"time"

	"github.com/ExileOlder/agent-hud-open/internal/markatlas"
)

type avatarSheet uint8

const (
	sheetHover avatarSheet = iota
	sheetIdle
	sheetWork
	sheetClaudeHover
	sheetClaudeIdle
	sheetClaudeWork
	sheetTool
	sheetQuestion
	sheetPermission
	sheetDone
	sheetCount
)

var avatarData = [sheetCount][]byte{
	hoverData, idleData, workData,
	claudeHoverData, claudeIdleData, claudeWorkData,
	toolData, cueQuestionData, cuePermissionData, cueDoneData,
}

const avatarIdleRetention = 2 * time.Minute

type cachedSheet struct {
	atlas markatlas.Atlas
	used  time.Time
}

// Owned by the native paint loop; the rest logos never keep a whole
// animation decoded. No timer or goroutine is attached to an atlas.
type avatarCache struct {
	sheets     [sheetCount]cachedSheet
	logos      [4]markatlas.Tile
	ordinary   bool
	claude     bool
	nextSweep  time.Time
	exitPixels [2]*image.RGBA
}

func (c *avatarCache) load(sheet avatarSheet, now time.Time) markatlas.Atlas {
	s := &c.sheets[sheet]
	if s.atlas.Len() == 0 {
		s.atlas = decodeAtlas(avatarData[sheet])
	}
	s.used = now
	return s.atlas
}

func (c *avatarCache) trim(now time.Time) {
	if now.Before(c.nextSweep) {
		return
	}
	c.nextSweep = now.Add(10 * time.Second)
	for i := range c.sheets {
		s := &c.sheets[i]
		if s.atlas.Len() != 0 && now.Sub(s.used) >= avatarIdleRetention {
			*s = cachedSheet{}
		}
	}
}

func (c *avatarCache) logo(provider, slot int) markatlas.Tile {
	if provider == 4 {
		if !c.claude {
			atlas := c.sheets[sheetClaudeHover].atlas
			if atlas.Len() == 0 {
				atlas = decodeAtlas(claudeHoverData)
			}
			c.logos[3], c.claude = atlas.CopyTile(0), true
		}
		return c.logos[3]
	}
	if !c.ordinary {
		atlas := c.sheets[sheetHover].atlas
		if atlas.Len() == 0 {
			atlas = decodeAtlas(hoverData)
		}
		for i := 0; i < 3; i++ {
			c.logos[i] = atlas.CopyTile(i * hoverFrames)
		}
		c.ordinary = true
	}
	return c.logos[slot]
}

func (c *avatarCache) draw(dst *image.RGBA, origin image.Point, provider, frame int, now time.Time) {
	slot := [...]int{0, 1, 1, 2, 0}[provider]
	if frame < -idleFrames {
		index := -frame - idleFrames - 1
		if step := index % exitSteps; step == 0 {
			c.draw(dst, origin, provider, index/exitSteps, now)
		} else {
			c.drawExit(dst, origin, provider, index/exitSteps, step, now)
		}
		return
	}
	if frame == 0 {
		c.logo(provider, slot).Draw(dst, origin)
		return
	}
	count, sheet := hoverFrames, sheetHover
	switch {
	case frame >= pendingBase:
		sheet = sheetQuestion + avatarSheet((frame-pendingBase)/waitAtlasFrames)
		frame, count = (frame-pendingBase)%waitAtlasFrames, waitAtlasFrames
	case frame >= cueBase:
		sheet = sheetQuestion + avatarSheet((frame-cueBase)/cueFrames)
		frame, count = (frame-cueBase)%cueFrames, waitAtlasFrames
		if sheet == sheetDone {
			count = cueFrames
		}
	case frame >= toolBase:
		frame, count, sheet = (frame-toolBase)%toolFrames, toolFrames, sheetTool
	case frame >= workBase:
		frame, count, sheet = frame-workBase, workFrames, sheetWork
	case frame < 0:
		frame, count, sheet = -frame-1, idleFrames, sheetIdle
	}
	if provider == 4 {
		if sheet <= sheetWork {
			sheet += sheetClaudeHover
		} else {
			slot = 3
		}
	}
	c.load(sheet, now).Draw(dst, origin, slot*count+frame)
}

// Keep the crop and its alpha premultiplied while the actor becomes its logo.
// Only the active hover exit needs these two reusable mark-sized surfaces.
func (c *avatarCache) drawExit(dst *image.RGBA, origin image.Point, provider, from, step int, now time.Time) {
	for i, frame := range [2]int{0, from} {
		if c.exitPixels[i] == nil {
			c.exitPixels[i] = image.NewRGBA(image.Rect(0, 0, markatlas.Cell, markatlas.Cell))
		}
		clear(c.exitPixels[i].Pix)
		c.draw(c.exitPixels[i], image.Point{}, provider, frame, now)
	}
	logo, actor := c.exitPixels[0], c.exitPixels[1]
	for i, v := range actor.Pix {
		actor.Pix[i] = uint8((int(v)*(exitSteps-step) + int(logo.Pix[i])*step + exitSteps/2) / exitSteps)
	}
	draw.Draw(dst, actor.Bounds().Add(origin), actor, image.Point{}, draw.Over)
}
