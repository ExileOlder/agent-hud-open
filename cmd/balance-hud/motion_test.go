package main

import (
	"testing"
	"time"

	"github.com/ExileOlder/agent-hud-open/internal/agentactivity"
)

func TestIdleAndHoverHaveSeparateEntrances(t *testing.T) {
	now := time.Now()
	var mark markRest
	if f, active := mark.frame(now, false, 0, false, -1); f != 0 || active {
		t.Fatal(f, active)
	}
	if delay := mark.nextVisit.Sub(now); delay < 20*time.Second || delay >= 35*time.Second {
		t.Fatal(delay)
	}
	visit := mark.nextVisit
	if f, active := mark.frame(visit.Add(time.Second), false, 0, false, -1); f != -17 || !active {
		t.Fatal(f, active)
	}
	if f, active := mark.frame(visit.Add(6*time.Second), false, 0, false, -1); f != 0 || active {
		t.Fatal(f, active)
	}
	at := visit.Add(8 * time.Second)
	if f, _ := mark.frame(at, true, time.Second, false, -1); f != 24 {
		t.Fatal(f)
	}
	if f, active := mark.frame(at.Add(time.Second), true, 8*time.Second, false, -1); f < 36 || !active {
		t.Fatalf("long hover replayed its greeting: %v %v", f, active)
	}
	if f, active := mark.frame(at.Add(2*time.Second), false, 0, false, -1); f != exitFrame(mark.hoverFrame, 0) || !active {
		t.Fatalf("hover should ease back: %v %v", f, active)
	}
}

func TestWorkingCadenceMatchesOriginalPhases(t *testing.T) {
	now := time.Now()
	var mark markRest
	for _, age := range []time.Duration{0, 4 * time.Second, 8 * time.Second, 12*time.Second + 500*time.Millisecond} {
		got, active := mark.frame(now.Add(age), true, 0, true, age)
		n := int(age * workFPS / time.Second)
		if !active || got != workBase+n {
			t.Fatal(age, got, active)
		}
	}
	// The first steady cycle continues straight through the entrance.
	// After it, the cycle resumes at its steady phase instead of frame zero.
	for _, tc := range []struct{ index, want int }{
		{workEntry - 1, workEntry - 1},
		{workEntry, workEntry},
		{90, 90},
		{97, 97},
		{98, workEntry},
		{99, workEntry + 1},
		{188, workEntry},
	} {
		age := (time.Duration(tc.index)*time.Second + workFPS - 1) / workFPS
		if got, _ := mark.frame(now.Add(age), false, 0, true, age); got != workBase+tc.want {
			t.Errorf("working tick %d drew %d, want %d", tc.index, got-workBase, tc.want)
		}
	}
	if last, restart := toolFrame(agentactivity.Read, toolCycle-time.Millisecond), toolFrame(agentactivity.Read, toolCycle); last != toolBase+35 || restart != toolBase {
		t.Fatal(last, restart)
	}
}

func TestWorkingLookIgnoresTransientToolTransitions(t *testing.T) {
	now := time.Now()
	var look workLook
	verify := func(at time.Duration, owner time.Time, target, want agentactivity.Tool) {
		t.Helper()
		if got, _ := look.follow(now.Add(at), owner, target); got != want {
			t.Errorf("%s: got %v, want %v", at, got, want)
		}
	}
	verify(0, now, agentactivity.Think, agentactivity.Think)
	verify(time.Second, now, agentactivity.Read, agentactivity.Think)
	verify(3*time.Second, now, agentactivity.Read, agentactivity.Think)
	verify(6*time.Second, now, agentactivity.Read, agentactivity.Read)
	verify(7*time.Second, now, agentactivity.Edit, agentactivity.Read)
	verify(8*time.Second, now, agentactivity.Read, agentactivity.Read)
	verify(9*time.Second, now.Add(9*time.Second), agentactivity.Command, agentactivity.Command)
}
