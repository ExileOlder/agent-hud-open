package main

import (
	"testing"
	"time"

	"github.com/ExileOlder/agent-hud-open/internal/agentactivity"
	"github.com/ExileOlder/agent-hud-open/internal/codexnotice"
)

func TestPendingCadenceHoldsAndRepeats(t *testing.T) {
	for _, tc := range []struct {
		at     time.Duration
		frame  int
		moving bool
	}{
		{0, 0, true}, {3 * time.Second, 23, false}, {3800 * time.Millisecond, 24, true},
		{5 * time.Second, 29, false}, {6400 * time.Millisecond, 32, true},
		{120800 * time.Millisecond, 32, true}, {125 * time.Second, 37, false},
		{128800 * time.Millisecond, 24, true},
	} {
		f, m := pendingFrame(tc.at, cueQuestion, false)
		if f != pendingBase+tc.frame || m != tc.moving {
			t.Errorf("%s: %d %v", tc.at, f, m)
		}
	}
	if f, m := pendingFrame(6500*time.Millisecond, cuePermission, true); f != pendingBase+waitAtlasFrames+23 || m {
		t.Fatalf("reduced motion: %d %t", f, m)
	}
}

func TestWaitingStateIsIndependentOfInboxAndSessions(t *testing.T) {
	var w workingMarks
	var mail mailInbox
	now := time.Now()
	w.updateCodex(codexnotice.Event{Pending: true}, &mail, now)
	w.updateDeepSeek(agentactivity.DeepSeekStatus{Pending: true, Questions: 1}, &mail, now, false)
	w.hook(hookEvent{agent: 5, phase: hookWait, session: 10}, &mail, now, false)
	w.hook(hookEvent{agent: 5, phase: hookPermission, session: 20}, &mail, now, false)
	mail.action(92)
	p := placement{ShowClaude: true}
	frames := func() markFrames {
		until := now.Add(7 * time.Second)
		got, _ := w.frames(view(nil), until, now, now, -1, p)
		return got
	}
	if got := frames(); got[0] < pendingBase || got[3] < pendingBase || got[4] < pendingBase+waitAtlasFrames {
		t.Fatalf("clear removed pending state: %v", got)
	}
	w.hook(hookEvent{agent: 5, phase: hookWork, session: 20}, &mail, now.Add(time.Second), false)
	if f := frames()[4]; f < pendingBase || f >= pendingBase+waitAtlasFrames {
		t.Fatalf("remaining question hidden: %d", f)
	}
	w.hook(hookEvent{agent: 5, phase: hookEnd, session: 10}, &mail, now.Add(time.Second), false)
	w.updateCodex(codexnotice.Event{Working: true}, &mail, now.Add(time.Second))
	w.updateDeepSeek(agentactivity.DeepSeekStatus{}, &mail, now.Add(time.Second), false)
	for i, f := range frames() {
		if f >= pendingBase {
			t.Errorf("resolved slot %d still waiting: %d", i, f)
		}
	}
}
