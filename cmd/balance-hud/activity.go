package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ExileOlder/agent-hud-open/internal/agentactivity"
	"github.com/ExileOlder/agent-hud-open/internal/codexnotice"
)

func watchAntigravity(ctx context.Context) <-chan map[string]bool {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(dir, "BalanceHUD", "activity-antigravity.json"))
	if err != nil {
		return nil
	}
	var profiles []agentactivity.AntigravityProfile
	if json.Unmarshal(data, &profiles) != nil {
		return nil
	}
	return agentactivity.WatchAntigravity(ctx, profiles)
}

type workingMarks struct {
	codex time.Time
	agy   map[string]time.Time
	hooks [2]map[uint32]workClock
}

func (w *workingMarks) updateAntigravity(active map[string]bool, now time.Time) {
	next := make(map[string]time.Time, len(active))
	for name, on := range active {
		if !on {
			continue
		}
		since := w.agy[name]
		if since.IsZero() {
			since = now
		}
		next[name] = since
	}
	w.agy = next
}

type workClock struct{ started, last time.Time }

func (w *workingMarks) hook(e hookEvent, inbox *mailInbox, now time.Time, motion bool) {
	if e.agent == 0 {
		return
	}
	var work map[uint32]workClock
	if e.session != 0 && (e.agent == 3 || e.agent == 5) {
		i := (e.agent - 3) / 2
		work = w.hooks[i]
		if e.phase == hookWork && work == nil {
			work = make(map[uint32]workClock)
			w.hooks[i] = work
		}
	}
	switch e.phase {
	case hookWork:
		clock := work[e.session]
		if clock.started.IsZero() {
			clock.started = now
		}
		clock.last = now
		work[e.session] = clock
	case hookComplete, hookWait:
		delete(work, e.session)
		inbox.receive(mailItem{agent: e.agent, event: codexnotice.Event{Question: e.phase == hookWait}}, now, motion)
	case hookEnd:
		delete(work, e.session)
	}
}

func hookAge(now time.Time, work map[uint32]workClock) time.Duration {
	var first time.Time
	for session, c := range work {
		if now.Sub(c.last) >= 30*time.Minute {
			delete(work, session)
			continue
		}
		if first.IsZero() || c.started.Before(first) {
			first = c.started
		}
	}
	return workAge(now, first)
}

func (w *workingMarks) ages(rows []row, now time.Time) [5]time.Duration {
	return [5]time.Duration{workAge(now, w.codex),
		workAge(now, w.agy[strings.ToLower(rows[1].Identity)]),
		workAge(now, w.agy[strings.ToLower(rows[2].Identity)]), hookAge(now, w.hooks[0]), hookAge(now, w.hooks[1])}
}

func (w *workingMarks) frames(rows []row, now, start, hoverStart time.Time, hover int, p placement) (markFrames, bool) {
	ages := w.ages(rows, now)
	return motionFrames(now.Sub(start), hover, now.Sub(hoverStart), p.ReduceMotion, p.slotCount(), ages[:]...)
}
