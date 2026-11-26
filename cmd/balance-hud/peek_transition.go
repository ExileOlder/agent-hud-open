package main

import "time"

// Original Agent HUD's HUDExpandedSurface geometry transition. The first
// coordinate of the cubic is elapsed time, so invert it before sampling y.
func peekSwitchEase(elapsed time.Duration) float64 {
	t := float64(elapsed) / float64(400*time.Millisecond)
	if t <= 0 {
		return 0
	}
	if t >= 1 {
		return 1
	}
	lo, hi := 0.0, 1.0
	for range 12 {
		m := (lo + hi) * .5
		u := 1 - m
		x := 3*u*u*m*.2 + 3*u*m*m*.2 + m*m*m
		if x < t {
			lo = m
		} else {
			hi = m
		}
	}
	m := (lo + hi) * .5
	u := 1 - m
	return 3*u*u*m*.8 + 3*u*m*m + m*m*m
}
