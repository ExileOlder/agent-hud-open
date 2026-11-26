package main

import (
	"bytes"
	"testing"

	"github.com/ExileOlder/agent-hud-open/internal/markatlas"
)

func TestAnimationAtlasesMatchNativeGrid(t *testing.T) {
	for _, tc := range []struct {
		name          string
		load          func() markatlas.Atlas
		slots, frames int
	}{
		{"hover", hoverAtlas, 3, hoverFrames},
		{"idle", idleAtlas, 3, idleFrames},
		{"work", workAtlas, 3, workFrames},
		{"Claude hover", claudeHoverAtlas, 1, hoverFrames},
		{"Claude idle", claudeIdleAtlas, 1, idleFrames},
		{"Claude work", claudeWorkAtlas, 1, workFrames},
		{"tools", toolAtlas, 4, toolFrames},
		{"question", cueAtlas[cueQuestion], 4, waitAtlasFrames},
		{"permission", cueAtlas[cuePermission], 4, waitAtlasFrames},
		{"completion", cueAtlas[cueDone], 4, cueFrames},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := tc.slots * tc.frames
			if got := tc.load().Len(); got != want {
				t.Fatalf("atlas frame count = %d, want %d", got, want)
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
