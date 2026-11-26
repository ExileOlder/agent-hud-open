package main

import (
	"testing"
	"time"
)

func TestAntigravityAnimationFollowsIdentityAcrossSlotSwap(t *testing.T) {
	now := time.Now()
	var status workingMarks
	status.updateAntigravity(map[string]bool{"first@example.test": true}, now)
	rows := view(nil)
	rows[1].Identity, rows[2].Identity = "FIRST@example.test", "second@example.test"
	want := [5]time.Duration{-1, time.Second, -1, -1, -1}
	if got := status.ages(rows, now.Add(time.Second)); got != want {
		t.Fatalf("activity: %v; want %v", got, want)
	}
	rows[1], rows[2] = rows[2], rows[1]
	want = [5]time.Duration{-1, -1, time.Second, -1, -1}
	if got := status.ages(rows, now.Add(time.Second)); got != want {
		t.Fatalf("swapped activity: %v; want %v", got, want)
	}
	status.updateAntigravity(nil, now.Add(2*time.Second))
	if got := status.ages(rows, now.Add(2*time.Second)); got != ([5]time.Duration{-1, -1, -1, -1, -1}) {
		t.Fatalf("stopped activity: %v", got)
	}
}

func TestHookTracksOverlappingSessionsAndWaits(t *testing.T) {
	now := time.Now()
	var work workingMarks
	var inbox mailInbox
	send := func(agent int, phase hookPhase, session uint32) {
		work.hook(hookEvent{agent, phase, session}, &inbox, now, false)
	}
	send(5, hookWork, 12)
	send(5, hookWork, 24)
	send(3, hookWork, 12)
	send(5, hookWait, 12)
	if hookAge(now, work.hooks[0]) < 0 || hookAge(now, work.hooks[1]) < 0 {
		t.Fatal("a pending Claude session hid another active session")
	}
	if len(inbox.items) != 1 || !inbox.items[0].event.Question || inbox.items[0].agent != 5 {
		t.Fatalf("wrong question notification: %+v", inbox.items)
	}
	send(5, hookComplete, 24)
	if hookAge(now, work.hooks[1]) >= 0 || hookAge(now, work.hooks[0]) < 0 {
		t.Fatal("completing Claude did not end only its activity")
	}
	send(3, hookEnd, 12)
	if len(inbox.items) != 2 || inbox.items[0].event.Question || hookAge(now, work.hooks[0]) >= 0 {
		t.Fatal("session-end must stop without a fake completion")
	}
}
