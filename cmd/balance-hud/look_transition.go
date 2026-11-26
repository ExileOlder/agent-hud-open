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
	}
}

// The original BotMotion uses an ease-out pose handoff for 300 ms. The
// lightweight backend retains the frame that was visible when the look
// changed, and interpolates it with the continuously advancing new clip.
// Notification, hover and rest entrances keep their own motion.
const lookHandoff = 300 * time.Millisecond

func workLookForFrame(frame int) int {
	switch {
	case frame >= workBase && frame < toolBase:
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
		case after == 0:
			move.start = time.Time{}
		case before != 0 && before != after:
			move.start, move.from = now, t.last[i]
		}
		if !move.start.IsZero() {
			elapsed := now.Sub(move.start)
			if elapsed < lookHandoff {
				// 1 - easeOut(t) is the outgoing mark's contribution.
				left := 1 - float64(elapsed)/float64(lookHandoff)
				out[i].previous = move.from
				out[i].oldOpacity = uint8(255*left*left*left + .5)
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
