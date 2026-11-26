package main

import (
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
