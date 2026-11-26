package main

import (
	"github.com/ExileOlder/agent-hud-open/internal/agentactivity"
	"testing"
	"time"
)

func TestAntigravityAnimationFollowsIdentityAcrossSlotSwap(t *testing.T) {
	now := time.Now()
	var status workingMarks
	status.updateAntigravity(map[string]agentactivity.Tool{"first@example.test": agentactivity.Read}, now)
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
		work.hook(hookEvent{agent: agent, phase: phase, session: session}, &inbox, now, false)
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

func TestNativeToolPoseTransitions(t *testing.T) {
	now := time.Now()
	rows := view(nil)
	p := placement{ShowClaude: true}
	var w workingMarks
	var inbox mailInbox
	w.hook(hookEvent{agent: 5, phase: hookCommand, session: 1}, &inbox, now, false)
	frame := func(at time.Duration) int {
		f, _ := w.frames(rows, now.Add(at), now, now, -1, p)
		return f[4]
	}
	if got := frame(0); got != toolBase+2*toolFrames {
		t.Fatal(got)
	}
	if got := frame(20 * time.Second); got >= toolBase+3*toolFrames {
		t.Fatalf("long tool still needs stabilization: %v", got)
	}
	if got := frame(22 * time.Second); got != toolBase+3*toolFrames {
		t.Fatalf("long command pose: %v", got)
	}
	w.hook(hookEvent{agent: 5, phase: hookComplete, session: 1}, &inbox, now.Add(23*time.Second), false)
	if got := frame(23 * time.Second); got >= workBase {
		t.Fatalf("finishing stops the working pose: %v", got)
	}
}

func TestAntigravityWaitingInboxTracksBothProfiles(t *testing.T) {
	now := time.Now()
	rows := view(nil)
	rows[1].Identity, rows[2].Identity = "one@example.test", "two@example.test"
	var inbox mailInbox
	var work workingMarks
	initial := []agentactivity.AntigravityWait{{Account: rows[1].Identity, Cascade: "one"}, {Account: rows[2].Identity, Cascade: "two", Permission: true}}
	work.updateAntigravityWaits(initial, &inbox, now, true)
	if len(inbox.queue) != 2 || inbox.sourceSlot(inbox.queue[0], rows) != 1 || inbox.sourceSlot(inbox.queue[1], rows) != 2 || !inbox.queue[1].permission {
		t.Fatal("native notices did not use their own marks")
	}
	work.updateAntigravityWaits(initial, &inbox, now, true)
	if len(inbox.queue) != 2 {
		t.Fatal("a repeated snapshot duplicated the notices")
	}
	work.updateAntigravityWaits(initial[1:], &inbox, now, true)
	rows[1], rows[2] = rows[2], rows[1]
	frames, _ := work.frames(rows, now, now, now, -1, placement{})
	if frames[1] < pendingBase || frames[2] >= pendingBase {
		t.Fatal("clearing one request or swapping positions moved the wrong state")
	}
	work.updateAntigravityWaits(nil, &inbox, now, true)
	frames, _ = work.frames(rows, now, now, now, -1, placement{})
	if frames[1] >= pendingBase || len(inbox.queue) != 2 {
		t.Fatal("resolved requests should lose the waiting pose but retain the notification")
	}
}

func TestAntigravityWaitingAttributionDoesNotResendNotice(t *testing.T) {
	now := time.Now()
	var inbox mailInbox
	var work workingMarks
	wait := agentactivity.AntigravityWait{Cascade: "fixture", Trajectory: "attempt"}
	work.updateAntigravityWaits([]agentactivity.AntigravityWait{wait}, &inbox, now, true)
	rows := view(nil)
	rows[2].Identity = "two@example.test"
	if len(inbox.queue) != 1 || inbox.sourceSlot(inbox.queue[0], rows) != -1 {
		t.Fatal("unknown source should not be assigned to the first account")
	}
	wait.Account = rows[2].Identity
	work.updateAntigravityWaits([]agentactivity.AntigravityWait{wait}, &inbox, now.Add(time.Second), true)
	frames, _ := work.frames(rows, now.Add(time.Second), now, now, -1, placement{})
	if len(inbox.queue) != 1 || frames[1] >= pendingBase || frames[2] < pendingBase {
		t.Fatal("verifying the account should move the waiting pose without adding a second notice")
	}
}

func TestClaudeCompactionIsImmediateAndReturnsToThePreviousWork(t *testing.T) {
	var w workingMarks
	var inbox mailInbox
	p := placement{ShowClaude: true}
	rows := view(nil)
	now := time.Now()
	post := func(phase hookPhase, offset time.Duration) {
		w.hook(hookEvent{agent: 5, phase: phase, session: 42}, &inbox, now.Add(offset), true)
	}
	frame := func(offset time.Duration) int {
		f, _ := w.frames(rows, now.Add(offset), now, now, -1, p)
		return f[4]
	}
	post(hookRead, 0)
	if got := frame(0); got != toolBase {
		t.Fatalf("expected tool look, got %d", got)
	}
	post(hookCompact, time.Second)
	if got := frame(time.Second); got != compactBase {
		t.Fatalf("compacting was held back by the tool guard: %d", got)
	}
	post(hookCompactEnd, 2*time.Second)
	if got := frame(2 * time.Second); got != toolBase {
		t.Fatalf("ending compaction did not restore prior work: %d", got)
	}
	post(hookEnd, 3*time.Second)
	post(hookCompact, 4*time.Second)
	if got := frame(4 * time.Second); got != compactBase {
		t.Fatalf("standalone compaction did not animate: %d", got)
	}
	post(hookCompactEnd, 5*time.Second)
	if got := frame(5 * time.Second); got >= workBase || len(inbox.queue) != 0 {
		t.Fatalf("ending manual compaction left work running or fabricated a message: %d", got)
	}
}
