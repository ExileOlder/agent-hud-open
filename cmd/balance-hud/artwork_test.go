package main

import (
	"image"
	"testing"
)

func TestAnimationAtlasesMatchNativeGrid(t *testing.T) {
	for _, tc := range []struct {
		name          string
		load          func() image.Image
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
			want := image.Pt(hoverColumns*atlasCell, tc.slots*tc.frames/hoverColumns*atlasCell)
			if got := tc.load().Bounds().Size(); got != want {
				t.Fatalf("atlas dimensions = %v, want %v", got, want)
			}
		})
	}
}
