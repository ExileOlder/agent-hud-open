package main

import (
	"testing"
	"time"
)

func TestIdleVisitsRepeatWithoutPointer(t *testing.T) {
	if got, active := motionFrames(15*time.Second, -1, 0, false, 4); got != (markFrames{}) || active {
		t.Fatal(got, active)
	}
	if got, active := motionFrames(firstVisit[0]+time.Second, -1, 0, false, 4); got[0] >= 0 || !active || got[1] != 0 {
		t.Fatal(got, active)
	}
	if got, _ := motionFrames(firstVisit[0]+6*time.Second, -1, 0, false, 4); got[0] != 0 {
		t.Fatal(got)
	}
	if got, _ := motionFrames(firstVisit[0]+visitPeriod[0]+time.Second, -1, 0, false, 4); got[0] >= 0 {
		t.Fatal(got)
	}
	if got, active := motionFrames(firstVisit[0]+time.Second, 0, time.Second, true, 4); got != (markFrames{}) || active {
		t.Fatal(got, active)
	}
	if got, _ := motionFrames(firstVisit[0]+time.Second, 0, time.Second, false, 4); got[0] != 25 {
		t.Fatal(got)
	}
}

func TestWorkingAnimationPreemptsHoverAndIdle(t *testing.T) {
	frames, active := motionFrames(time.Second, 0, 200*time.Millisecond, false, 4, 500*time.Millisecond, -1, 250*time.Millisecond)
	if !active || frames != (markFrames{workBase + 6, 0, workBase + 3}) {
		t.Fatalf("unexpected work frames: %v, active: %v", frames, active)
	}
	if frames, active = motionFrames(time.Second, 0, time.Second, true, 4, time.Second); active || frames != (markFrames{}) {
		t.Fatalf("reduced motion: %v, active: %v", frames, active)
	}
}
