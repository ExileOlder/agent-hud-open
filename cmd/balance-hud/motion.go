package main

import (
	"math/rand/v2"
	"time"

	"github.com/ExileOlder/agent-hud-open/internal/agentactivity"
)

const (
	idleFrames      = 88
	idleFPS         = 16
	hoverFPS        = 24
	workFPS         = 6
	workCycle       = 15 * time.Second
	workFrames      = 104
	workEntry       = 8
	workBase        = hoverFrames
	toolBase        = workBase + workFrames
	toolFrames      = 40
	toolFPS         = 8
	toolCycle       = 4500 * time.Millisecond
	cueBase         = toolBase + 4*toolFrames
	cueFrames       = 40
	pendingBase     = cueBase + 3*cueFrames
	waitAtlasFrames = 40
)

type markFrames [5]int // zero: logo; negative: idle, asleep or hover exit; workBase and higher: work/cue atlases

const (
	hoverExit     = 500 * time.Millisecond // upstream BotMotion's morph stride
	exitSteps     = 12
	hudSleepAfter = time.Hour
)

func exitFrame(from, step int) int { return -2*idleFrames - 1 - from*exitSteps - step }

// Each mark has its own idle visit clock. The original delays visits by
// 20–35s and starts a new wait after the visit, not from the process clock.
type markRest struct {
	nextVisit, leftHover time.Time
	lastActivity         time.Time
	identity             string
	hovered, sleepVisit  bool
	hoverFrame           int
}

func (r *markRest) frame(now time.Time, hovered bool, hoverAge time.Duration, working bool, age time.Duration, asleep bool) (int, bool) {
	if working || hovered {
		r.nextVisit, r.leftHover = time.Time{}, time.Time{}
		r.hovered = hovered && !working
		if working {
			n := int(age * workFPS / time.Second)
			loop := int(workCycle * workFPS / time.Second)
			if n >= workEntry+loop {
				n = workEntry + (n-workEntry)%loop
			}
			return workBase + n, true
		}
		// Play the arrival and greeting once, then stay in the awake portion.
		n := int(max(0, hoverAge) * hoverFPS / time.Second)
		const awake = 36
		if n >= hoverFrames {
			n = awake + (n-awake)%(hoverFrames-awake)
		}
		r.hoverFrame = n
		return n, true
	}
	if r.hovered {
		r.hovered, r.leftHover = false, now
	}
	if !r.leftHover.IsZero() {
		step := int(now.Sub(r.leftHover) * exitSteps / hoverExit)
		if step < exitSteps && r.hoverFrame != 0 {
			// Start at the pose actually on screen. Reversing the opening
			// clip can replay the greeting hop when the pointer leaves.
			return exitFrame(r.hoverFrame, step), true
		}
		r.leftHover = time.Time{}
	}
	if r.nextVisit.IsZero() || r.sleepVisit != asleep && now.Before(r.nextVisit) {
		r.scheduleVisit(now, asleep)
	}
	since := now.Sub(r.nextVisit)
	if since < 0 {
		return 0, false
	}
	if n := int(since * idleFPS / time.Second); n < idleFrames {
		if r.sleepVisit {
			n += idleFrames
		}
		return -1 - n, true
	}
	r.scheduleVisit(now, asleep)
	return 0, false
}

// Mirrors the idle/asleep visit cadence in BotMotion.scheduleVisit.
func (r *markRest) scheduleVisit(now time.Time, asleep bool) {
	r.sleepVisit = asleep
	delay := 20*time.Second + rand.N(15*time.Second)
	if asleep {
		delay = 180*time.Second + rand.N(120*time.Second)
	}
	r.nextVisit = now.Add(delay)
}

func (r *markRest) asleep(now, start time.Time) bool {
	last := r.lastActivity
	if last.IsZero() {
		last = start
	}
	return now.Sub(last) >= hudSleepAfter
}

// Apply the upstream working-look grace period. Flipping between thinking and
// tools during a single turn should not repeatedly restart the actor.
type workLook struct {
	owner, shownAt, wantedAt time.Time
	shown, wanted            agentactivity.Tool
	changes                  [4]time.Time
	guarded                  bool
}

func (m *workLook) follow(now, owner time.Time, want agentactivity.Tool) (agentactivity.Tool, time.Duration) {
	if owner.IsZero() {
		*m = workLook{}
		return agentactivity.Think, 0
	}
	if !owner.Equal(m.owner) {
		*m = workLook{owner: owner, shownAt: now, shown: want, wanted: want,
			changes: [4]time.Time{3: now}}
		return want, 0
	}
	if m.guarded && now.Sub(m.shownAt) >= 30*time.Second {
		m.guarded = false
		m.changes = [4]time.Time{}
	}
	if want == m.shown {
		m.wantedAt = time.Time{}
	} else {
		if m.wantedAt.IsZero() || m.wanted != want {
			m.wantedAt, m.wanted = now, want
		}
		guard := 6 * time.Second
		if m.guarded {
			guard = 12 * time.Second
		}
		if now.Sub(m.wantedAt) >= 2*time.Second && now.Sub(m.shownAt) >= guard {
			m.shown, m.shownAt, m.wantedAt = want, now, time.Time{}
			copy(m.changes[:], m.changes[1:])
			m.changes[3] = now
			m.guarded = m.guarded || !m.changes[0].IsZero() && now.Sub(m.changes[0]) < 30*time.Second
		}
	}
	return m.shown, now.Sub(m.shownAt)
}

func workAge(now, started time.Time) time.Duration {
	if started.IsZero() {
		return -1
	}
	return max(0, now.Sub(started))
}

func toolFrame(tool agentactivity.Tool, age time.Duration) int {
	frame := int((max(0, age) % toolCycle) * toolFPS / time.Second)
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
	settled := 7
	if look == cueQuestion {
		settled = 5 // 0.4s in BotMoves
	}
	return base + 24 + ((n+1)%2)*8 + min(settled, frame), frame < settled
}
