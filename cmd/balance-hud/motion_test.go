package main

import (
	"testing"
	"time"
)

func TestIdleVisitsRepeatWithoutPointer(t *testing.T) {
	if got, active := motionFrames(15*time.Second, -1, 0, false); got != (markFrames{}) || active {
		t.Fatal(got, active)
	}
	if got, active := motionFrames(firstVisit[0]+time.Second, -1, 0, false); got[0] >= 0 || !active || got[1] != 0 {
		t.Fatal(got, active)
	}
	if got, _ := motionFrames(firstVisit[0]+6*time.Second, -1, 0, false); got[0] != 0 {
		t.Fatal(got)
	}
	if got, _ := motionFrames(firstVisit[0]+visitPeriod[0]+time.Second, -1, 0, false); got[0] >= 0 {
		t.Fatal(got)
	}
	if got, active := motionFrames(firstVisit[0]+time.Second, 0, time.Second, true); got != (markFrames{}) || active {
		t.Fatal(got, active)
	}
	if got, _ := motionFrames(firstVisit[0]+time.Second, 0, time.Second, false); got[0] != 25 {
		t.Fatal(got)
	}
}
