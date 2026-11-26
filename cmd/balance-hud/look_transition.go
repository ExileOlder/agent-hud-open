package main

import "time"

type markVisual struct {
	frame, previous int
	oldOpacity      uint8
}
type visualFrames [5]markVisual

type lookTransition struct {
	last  markFrames
	moves [5]struct {
		start time.Time
		from  int
		rest  bool
	}
}

// The original BotMotion uses an ease-out pose handoff for 300 ms. The
// lightweight backend retains the frame that was visible when the look
// changed, and interpolates it with the continuously advancing new clip.
// Notification, hover and rest entrances keep their own motion.
const lookHandoff = 300 * time.Millisecond

func workLookForFrame(frame int) int {
	switch {
	case frame >= workBase && frame < toolBase || frame >= wakeBase:
		return 1
	case frame >= compactBase && frame < cueBase:
		return 6
	case frame >= toolBase && frame < compactBase:
		return 2 + (frame-toolBase)/toolFrames
	default:
		return 0
	}
}

func (t *lookTransition) visual(now time.Time, frames markFrames, reduced bool) (visualFrames, bool) {
	if reduced {
		*t = lookTransition{}
		return plainVisuals(frames), false
	}
	var out visualFrames
	moving := false
	for i, frame := range frames {
		out[i].frame = frame
		before, after := workLookForFrame(t.last[i]), workLookForFrame(frame)
		move := &t.moves[i]
		switch {
		case frame == 0 && t.last[i] >= workBase:
			// BotMotion returns from a live pose to the logo over its regular
			// 500 ms morph stride. Hold the outgoing frame while morphing.
			move.start, move.from, move.rest = now, t.last[i], true
		case frame == 0 && move.rest:
			// Continue the return without restarting the clock.
		case after == 0:
			move.start = time.Time{}
		case before != 0 && before != after:
			move.start, move.from, move.rest = now, t.last[i], false
		case move.rest:
			move.start = time.Time{}
		}
		if !move.start.IsZero() {
			duration := lookHandoff
			if move.rest {
				duration = hoverExit
			}
			elapsed := now.Sub(move.start)
			if elapsed < duration {
				left := 1 - float64(elapsed)/float64(duration)
				if !move.rest { // 1 - easeOut(t)
					left *= left * left
				}
				out[i].previous = move.from
				out[i].oldOpacity = uint8(255*left + .5)
				moving = moving || out[i].oldOpacity != 0
			} else {
				move.start = time.Time{}
			}
		}
	}
	t.last = frames
	return out, moving
}

func plainVisuals(frames markFrames) (out visualFrames) {
	for i, frame := range frames {
		out[i].frame = frame
	}
	return
}
