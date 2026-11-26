package main

import (
	"time"
)

const (
	idleFrames = 88
	idleFPS    = 16
)

// An idle visit wakes after about 20–35 seconds like the original HUD.
// Give each mark its own clock so they do not animate in lockstep.
var firstVisit = [...]time.Duration{21 * time.Second, 24 * time.Second, 27 * time.Second, 30 * time.Second, 33 * time.Second}
var visitPeriod = [...]time.Duration{32 * time.Second, 36 * time.Second, 40 * time.Second, 34 * time.Second, 31 * time.Second}

type markFrames [5]int // zero: logo; positive: hover; negative: idle atlas

func motionFrames(elapsed time.Duration, hover int, sinceHover time.Duration, reduced bool, count int) (markFrames, bool) {
	var frames markFrames
	if reduced {
		return frames, false
	}
	playing := false
	for i := range count {
		if hover == i {
			frame := max(0, int(sinceHover*25/time.Second))
			frames[i] = min(hoverFrames-1, frame)
			playing = playing || frame < hoverFrames
			continue
		}
		if elapsed < firstVisit[i] {
			continue
		}
		frame := int(((elapsed - firstVisit[i]) % visitPeriod[i]) * idleFPS / time.Second)
		if frame < idleFrames {
			frames[i] = -frame - 1
			playing = true
		}
	}
	return frames, playing
}
