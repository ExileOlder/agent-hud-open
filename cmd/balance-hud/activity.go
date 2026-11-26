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

func watchAntigravity(ctx context.Context) <-chan map[string]agentactivity.Tool {
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
	codex    workClock
	deepSeek time.Time
	agy      map[string]workClock
	hooks    [2]map[uint32]workClock
}

func (w *workingMarks) updateCodex(e codexnotice.Event, now time.Time) {
	if !e.Working {
		w.codex = workClock{}
		return
	}
	if w.codex.started.IsZero() {
		w.codex.started = now
	}
	if w.codex.poseAt.IsZero() || w.codex.tool != e.Tool {
		w.codex.tool, w.codex.poseAt = e.Tool, now
	}
}

func deepSeekHome() string {
	if dir := os.Getenv("DSH_HOME"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".dsh")
}

func (w *workingMarks) updateDeepSeek(state agentactivity.DeepSeekStatus, mail *mailInbox, now time.Time, motion bool) {
	if state.Running {
		if w.deepSeek.IsZero() {
			w.deepSeek = now
		}
	} else {
		w.deepSeek = time.Time{}
	}
	for range state.Questions {
		mail.receive(mailItem{agent: deepSeekAgent, event: codexnotice.Event{Question: true}}, now, motion)
	}
}

func (w *workingMarks) updateAntigravity(active map[string]agentactivity.Tool, now time.Time) {
	next := make(map[string]workClock, len(active))
	for name, kind := range active {
		clock := w.agy[name]
		if clock.started.IsZero() {
			clock.started = now
		}
		if clock.tool != kind || clock.poseAt.IsZero() {
			clock.tool, clock.poseAt = kind, now
		}
		next[name] = clock
	}
	w.agy = next
}

type workClock struct {
	started, last, poseAt time.Time
	tool                  agentactivity.Tool
}

func (w *workingMarks) hook(e hookEvent, inbox *mailInbox, now time.Time, motion bool) {
	if e.agent == 0 {
		return
	}
	var work map[uint32]workClock
	if e.session != 0 && (e.agent == 3 || e.agent == 5) {
		i := (e.agent - 3) / 2
		work = w.hooks[i]
		if (e.phase == hookWork || e.phase >= hookRead) && work == nil {
			work = make(map[uint32]workClock)
			w.hooks[i] = work
		}
	}
	switch e.phase {
	case hookWork, hookRead, hookEdit, hookCommand:
		clock := work[e.session]
		if clock.started.IsZero() {
			clock.started = now
		}
		clock.last = now
		tool := agentactivity.Think
		if e.phase >= hookRead {
			tool = agentactivity.Tool(e.phase-hookRead) + agentactivity.Read
		}
		if clock.tool != tool || clock.poseAt.IsZero() {
			clock.tool, clock.poseAt = tool, now
		}
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
	return [5]time.Duration{workAge(now, w.codex.started),
		workAge(now, w.agy[strings.ToLower(rows[1].Identity)].started),
		workAge(now, w.agy[strings.ToLower(rows[2].Identity)].started), max(workAge(now, w.deepSeek), hookAge(now, w.hooks[0])), hookAge(now, w.hooks[1])}
}

func (w *workingMarks) frames(rows []row, now, start, hoverStart time.Time, hover int, p placement) (markFrames, bool) {
	ages := w.ages(rows, now)
	frames, active := motionFrames(now.Sub(start), hover, now.Sub(hoverStart), p.ReduceMotion, p.slotCount(), ages[:]...)
	if p.ReduceMotion {
		return frames, active
	}
	if w.codex.tool != agentactivity.Think {
		frames[0] = toolFrame(w.codex.tool, now.Sub(w.codex.poseAt))
	}
	for slot := 1; slot <= 2; slot++ {
		if clock := w.agy[strings.ToLower(rows[slot].Identity)]; clock.tool != agentactivity.Think {
			frames[slot] = toolFrame(clock.tool, now.Sub(clock.poseAt))
		}
	}
	for i, sessions := range w.hooks {
		slot := i + 3
		if slot >= p.slotCount() {
			break
		}
		var selected workClock
		for _, clock := range sessions {
			if clock.tool != agentactivity.Think && clock.last.After(selected.last) {
				selected = clock
			}
		}
		if selected.tool != agentactivity.Think {
			frames[slot] = toolFrame(selected.tool, now.Sub(selected.poseAt))
		}
	}
	return frames, active
}
