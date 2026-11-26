package main

import (
	"time"

	"github.com/ExileOlder/agent-hud-open/internal/agentactivity"
)

const (
	idleFrames      = 88
	idleFPS         = 16
	workFPS         = 12
	workFrames      = 48
	workBase        = hoverFrames
	toolBase        = workBase + workFrames
	toolFrames      = 32
	cueBase         = toolBase + 4*toolFrames
	cueFrames       = 24
	pendingBase     = cueBase + 3*cueFrames
	waitAtlasFrames = 40
)

// An idle visit wakes after about 20–35 seconds like the original HUD.
// Give each mark its own clock so they do not animate in lockstep.
var firstVisit = [...]time.Duration{21 * time.Second, 24 * time.Second, 27 * time.Second, 30 * time.Second, 33 * time.Second}
var visitPeriod = [...]time.Duration{32 * time.Second, 36 * time.Second, 40 * time.Second, 34 * time.Second, 31 * time.Second}

type markFrames [5]int // zero: logo; negative: idle; workBase and higher: working atlas

func motionFrames(elapsed time.Duration, hover int, sinceHover time.Duration, reduced bool, count int, working ...time.Duration) (markFrames, bool) {
	var frames markFrames
	if reduced {
		return frames, false
	}
	playing := false
	for i := range count {
		if i < len(working) && working[i] >= 0 {
			age := working[i]
			frames[i] = workBase + int(age*workFPS/time.Second)%workFrames
			playing = true
			continue
		}
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

func workAge(now, started time.Time) time.Duration {
	if started.IsZero() {
		return -1
	}
	return max(0, now.Sub(started))
}

// Original director switches a shell call to the long-command look at 20s.
func toolFrame(tool agentactivity.Tool, age time.Duration) int {
	if tool == agentactivity.Command && age >= 20*time.Second {
		tool = agentactivity.LongCommand
		age -= 20 * time.Second
	}
	frame := int(max(0, age)*workFPS/time.Second) % toolFrames
	return toolBase + (int(tool)-1)*toolFrames + frame
}

// The original BotMoves repeats after its first movement at 1.2s, every
// 2.6s, then every 8s from 120.8s. The two atlas groups capture alternating
// movements. A settled pose is drawn once and held until the next movement.
func pendingFrame(age time.Duration, look cueLook, reduce bool) (int, bool) {
	const firstRepeat = 3800 * time.Millisecond
	const frequent = 2600 * time.Millisecond
	const slowStart = 120800 * time.Millisecond
	const slow = 8 * time.Second
	base := pendingBase + int(look)*waitAtlasFrames
	if reduce {
		return base + 23, false
	}
	if age < 2*time.Second {
		return base + int(max(0, age)*12/time.Second), true
	}
	if age < firstRepeat {
		return base + 23, false
	}
	var n int
	var since time.Duration
	if age < slowStart {
		n = 1 + int((age-firstRepeat)/frequent)
		since = (age - firstRepeat) % frequent
	} else {
		n = 46 + int((age-slowStart)/slow)
		since = (age - slowStart) % slow
	}
	frame := int(since * 12 / time.Second)
	return base + 24 + ((n+1)%2)*8 + min(7, frame), frame < 7
}
