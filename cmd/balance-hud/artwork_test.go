package main

import (
	"bytes"
	"image"
	"testing"
	"time"
)

func TestAnimationAtlasesMatchNativeGrid(t *testing.T) {
	for _, tc := range []struct {
		name          string
		sheet         avatarSheet
		slots, frames int
	}{
		{"hover", sheetHover, 3, hoverFrames},
		{"idle", sheetIdle, 3, idleFrames},
		{"work", sheetWork, 3, workFrames},
		{"Claude hover", sheetClaudeHover, 1, hoverFrames},
		{"Claude idle", sheetClaudeIdle, 1, idleFrames},
		{"Claude work", sheetClaudeWork, 1, workFrames},
		{"tools", sheetTool, 4, toolFrames},
		{"question", sheetQuestion, 4, waitAtlasFrames},
		{"permission", sheetPermission, 4, waitAtlasFrames},
		{"completion", sheetDone, 4, cueFrames},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for slot := range tc.slots {
				if got := decodeAtlas(avatarData[tc.sheet], slot).Len(); got != tc.frames {
					t.Fatalf("provider %d frame count = %d, want %d", slot, got, tc.frames)
				}
			}
		})
	}
}

func TestDockIncrementalPaintMatchesFreshPaint(t *testing.T) {
	var painter barPainter
	rows := view(nil)
	p := placement{Edge: 3, Numbers: true}
	frames := markFrames{}
	for i := range rows {
		rows[i].Percent = 70 - 6*i
	}
	check := func(label string) {
		t.Helper()
		var fresh barPainter
		want := fresh.paint(rows, p, frames)
		got := painter.paint(rows, p, frames)
		if got.Bounds() != want.Bounds() || !bytes.Equal(got.Pix, want.Pix) {
			t.Fatalf("%s: reused surface differs from a fresh render", label)
		}
	}
	check("initial")
	frames[1] = workBase + 18
	check("one icon moved")
	rows[1].Stale = true
	check("same percentage turns stale")
	rows[2].Percent = -1
	check("another reading unavailable")
	frames[1] = 0
	check("icon returned to rest")
	p.Edge, p.ShowClaude = 0, true
	check("dock moved to top and gained a mark")
	p.Numbers = false
	check("percentages hidden")
	p.Edge, p.ShowClaude = 3, false
	check("side dock restored")
}

func TestColdAtlasesCanExpireWithoutChangingLogosOrReloadedFrames(t *testing.T) {
	var cache avatarCache
	now := time.Now()
	render := func(provider, frame int, at time.Time) []byte {
		dst := image.NewRGBA(image.Rect(0, 0, markRadius*2, markRadius*2))
		cache.draw(dst, image.Point{}, provider, frame, at)
		return dst.Pix
	}
	logo := render(0, 0, now)
	claudeLogo := render(4, 0, now)
	if cache.sheets[sheetHover][0].atlas.Len() != 0 || cache.sheets[sheetClaudeHover][0].atlas.Len() != 0 {
		t.Fatal("rest logos retained the hover atlases")
	}
	active := render(0, workBase+8, now)
	for _, slot := range []int{1, 2} {
		if cache.sheets[sheetWork][slot].atlas.Len() != 0 {
			t.Fatalf("Codex work also decoded provider %d", slot)
		}
	}
	// An idle visit refreshed its own sheet shortly before the sweep.
	render(1, workBase+8, now)
	recent := now.Add(avatarIdleRetention - time.Second)
	render(2, workBase+8, recent) // The second Antigravity account shares the first account's frames.
	render(1, -8, recent)
	after := now.Add(avatarIdleRetention)
	cache.trim(after)
	if cache.sheets[sheetWork][0].atlas.Len() != 0 || cache.sheets[sheetWork][1].atlas.Len() == 0 || cache.sheets[sheetIdle][1].atlas.Len() == 0 {
		t.Fatal("sweep did not separate expired and recently used atlases")
	}
	if !bytes.Equal(logo, render(0, 0, after)) ||
		!bytes.Equal(claudeLogo, render(4, 0, after)) ||
		!bytes.Equal(active, render(0, workBase+8, after)) {
		t.Fatal("expiry or reloading changed frame pixels")
	}
}

func TestAllMarksLeaveHoverFromDisplayedPose(t *testing.T) {
	now := time.Now()
	for slot := range (markFrames{}) {
		var rest markRest
		var cache avatarCache
		const hoverAge = 3300 * time.Millisecond
		frame, _ := rest.frame(now, true, hoverAge, false, -1)
		render := func(frame int) []byte {
			dst := image.NewRGBA(image.Rect(0, 0, markRadius*2, markRadius*2))
			cache.draw(dst, image.Point{}, slot, frame, now)
			return dst.Pix
		}
		actor, logo := render(frame), render(0)
		first, _ := rest.frame(now, false, 0, false, -1)
		mid, _ := rest.frame(now.Add(hoverExit/2), false, 0, false, -1)
		last, active := rest.frame(now.Add(hoverExit), false, 0, false, -1)
		if !bytes.Equal(render(first), actor) || active || last != 0 {
			t.Fatalf("slot %d: exit did not connect the last drawn pose to the logo", slot)
		}
		for i, got := range render(mid) {
			if want := uint8((int(actor[i]) + int(logo[i]) + 1) / 2); got != want {
				t.Fatalf("slot %d: midpoint replayed a pose at pixel component %d", slot, i)
			}
		}
	}
}

func TestDockFrameDamageReportsOnlyChangedMarkPatches(t *testing.T) {
	for _, edge := range []int{0, 3} {
		var painter barPainter
		rows := view(nil)
		p := placement{Edge: edge, Numbers: true}
		painter.paint(rows, p, markFrames{})
		changed := markFrames{workBase + 8, 0, toolBase + 1}
		frame := painter.paint(rows, p, changed)
		for i, rect := range painter.damage {
			if i != 0 && i != 2 {
				if !rect.Empty() {
					t.Fatalf("stationary mark %d was damaged at edge %d", i, edge)
				}
				continue
			}
			if rect.Empty() || rect.Dx() != 67 || rect.Dy() != 67 || !rect.In(frame.Bounds()) {
				t.Fatalf("edge %d mark %d: unexpected patch %v", edge, i, rect)
			}
		}
		painter.paint(rows, p, changed)
		if painter.damage != [5]image.Rectangle{} {
			t.Fatalf("unchanged frame damaged the bar at edge %d", edge)
		}
	}
}
