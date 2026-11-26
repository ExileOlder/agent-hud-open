package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ExileOlder/agent-hud-open/internal/agentactivity"
	"github.com/ExileOlder/agent-hud-open/internal/codexnotice"
)

func loadAntigravityProfiles() []agentactivity.AntigravityProfile {
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
	return profiles
}

func antigravityProfileKeys() map[string]uint32 {
	keys := map[string]uint32{}
	for _, p := range loadAntigravityProfiles() {
		if p.Account != "" && filepath.IsAbs(p.GeminiHome) {
			keys[strings.ToLower(p.Account)] = profileKey(p.GeminiHome)
		}
	}
	return keys
}

type workingMarks struct {
	rest                      [5]markRest
	looks                     [5]workLook
	codex                     workClock
	deepSeek, deepSeekCompact time.Time
	agy                       map[string]workClock
	agyWaits                  map[agentactivity.AntigravityWait]markCue
	hooks                     [2]map[uint32]workClock
	pendingHooks              [2]map[uint32]markCue
	codexWait, deepSeekWait   markCue
}

func setProjectionWaiting(m *markCue, active bool, now time.Time, inbox *mailInbox, slot int) {
	if !active {
		if !m.at.IsZero() {
			inbox.clearWaitCue(slot)
		}
		*m = markCue{}
	} else if m.at.IsZero() {
		*m = markCue{cueQuestion, now}
	}
}

func (w *workingMarks) updateCodex(e codexnotice.Event, inbox *mailInbox, now time.Time) {
	setProjectionWaiting(&w.codexWait, e.Pending, now, inbox, 0)
	if !e.Working {
		w.codex = workClock{}
		return
	}
	if w.codex.started.IsZero() {
		w.codex.started = now
	}
	w.codex.holds = e.Holding
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
	was := w.deepSeekWait
	switch {
	case state.ApprovalPending:
		if was.at.IsZero() || was.look != cuePermission {
			w.deepSeekWait = markCue{cuePermission, now}
		}
	case state.Pending:
		if was.at.IsZero() || was.look != cueQuestion {
			w.deepSeekWait = markCue{cueQuestion, now}
		}
	default:
		setProjectionWaiting(&w.deepSeekWait, false, now, mail, 3)
	}
	if !state.Compacting {
		w.deepSeekCompact = time.Time{}
	} else if w.deepSeekCompact.IsZero() {
		w.deepSeekCompact = now
	}
	if state.Running {
		if w.deepSeek.IsZero() {
			w.deepSeek = now
		}
	} else {
		w.deepSeek = time.Time{}
	}
	for _, key := range state.Questions {
		mail.receive(mailItem{agent: deepSeekAgent, key: key, event: codexnotice.Event{Question: true}}, now, motion)
	}
	for _, key := range state.Approvals {
		mail.receive(mailItem{agent: deepSeekAgent, key: key, permission: true, event: codexnotice.Event{Question: true}}, now, motion)
	}
	for range state.Completed {
		mail.receive(mailItem{agent: deepSeekAgent}, now, motion)
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

func (w *workingMarks) updateAntigravityWaits(waits []agentactivity.AntigravityWait, inbox *mailInbox, now time.Time, motion bool) {
	next := make(map[agentactivity.AntigravityWait]markCue, len(waits))
	known := make(map[agentactivity.AntigravityWait]markCue, len(w.agyWaits))
	for old, cue := range w.agyWaits {
		old.Account = ""
		known[old] = cue
	}
	for _, item := range waits {
		id := item
		id.Account = ""
		if existing, ok := known[id]; ok {
			next[item] = existing
			continue
		}
		look := cueQuestion
		if item.Permission {
			look = cuePermission
		}
		next[item] = markCue{look, now}
		known[id] = next[item]
		key := fmt.Sprintf("%s/%s/%s/%d", item.Cascade, item.Trajectory, item.Execution, item.Step)
		inbox.receive(mailItem{agent: 2, account: item.Account, key: key, permission: item.Permission, event: codexnotice.Event{Question: true}}, now, motion)
	}
	w.agyWaits = next
}

type workClock struct {
	started, last, poseAt, compactingAt time.Time
	tool                                agentactivity.Tool
	holds                               bool
}

func (w *workingMarks) hook(e hookEvent, inbox *mailInbox, now time.Time, motion bool) {
	if e.agent == 0 {
		return
	}
	var work map[uint32]workClock
	var waits map[uint32]markCue
	if e.session != 0 && (e.agent == 3 || e.agent == 5) {
		i := (e.agent - 3) / 2
		work = w.hooks[i]
		waits = w.pendingHooks[i]
		if (e.phase == hookWait || e.phase == hookPermission) && waits == nil {
			waits = make(map[uint32]markCue)
			w.pendingHooks[i] = waits
		}
		if (e.phase == hookWork || e.phase == hookCompact || e.phase >= hookRead && e.phase <= hookCommand) && work == nil {
			work = make(map[uint32]workClock)
			w.hooks[i] = work
		}
	}
	switch e.phase {
	case hookCompact:
		clock := work[e.session]
		if clock.compactingAt.IsZero() {
			clock.compactingAt = now
		}
		clock.last = now
		work[e.session] = clock
	case hookCompactEnd:
		if clock, ok := work[e.session]; ok && !clock.compactingAt.IsZero() {
			clock.compactingAt, clock.last = time.Time{}, now
			if clock.started.IsZero() {
				delete(work, e.session)
			} else {
				work[e.session] = clock
			}
		}
	case hookWork, hookRead, hookEdit, hookCommand:
		if _, ok := waits[e.session]; ok {
			delete(waits, e.session)
			inbox.clearWaitCue(completionSlot(e.agent))
		}
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
	case hookComplete, hookWait, hookPermission:
		delete(work, e.session)
		if e.phase == hookWait || e.phase == hookPermission {
			look := cueQuestion
			if e.phase == hookPermission {
				look = cuePermission
			}
			if prev := waits[e.session]; prev.at.IsZero() || prev.look != look {
				waits[e.session] = markCue{look, now}
			}
		} else {
			delete(waits, e.session)
		}
		var profile uint32
		if e.agent == 2 {
			profile = e.session
		}
		inbox.receive(mailItem{agent: e.agent, event: codexnotice.Event{Question: e.phase == hookWait || e.phase == hookPermission}, permission: e.phase == hookPermission, profile: profile}, now, motion)
	case hookEnd:
		delete(work, e.session)
		if _, ok := waits[e.session]; ok {
			delete(waits, e.session)
			inbox.clearWaitCue(completionSlot(e.agent))
		}
	}
}

func hookAge(now time.Time, work map[uint32]workClock) time.Duration {
	var first time.Time
	for session, c := range work {
		if now.Sub(c.last) >= 30*time.Minute {
			delete(work, session)
			continue
		}
		if !c.compactingAt.IsZero() && now.Sub(c.compactingAt) >= 5*time.Minute {
			c.compactingAt = time.Time{}
			if c.started.IsZero() {
				delete(work, session)
				continue
			}
			work[session] = c
		}
		started := c.started
		if started.IsZero() {
			started = c.compactingAt
		}
		if first.IsZero() || started.Before(first) {
			first = started
		}
	}
	return workAge(now, first)
}

func (w *workingMarks) ages(rows []row, now time.Time) [5]time.Duration {
	return [5]time.Duration{workAge(now, w.codex.started),
		workAge(now, w.agy[strings.ToLower(rows[1].Identity)].started),
		workAge(now, w.agy[strings.ToLower(rows[2].Identity)].started), max(workAge(now, w.deepSeek), workAge(now, w.deepSeekCompact), hookAge(now, w.hooks[0])), hookAge(now, w.hooks[1])}
}

func (w *workingMarks) waking(p placement) bool {
	for _, rest := range w.rest[:p.slotCount()] {
		if !rest.wakeAt.IsZero() {
			return true
		}
	}
	return false
}

func (w *workingMarks) frames(rows []row, now, start, hoverStart time.Time, hover int, p placement) (markFrames, bool) {
	var frames markFrames
	ages := w.ages(rows, now)
	for slot, age := range ages[:p.slotCount()] {
		r := &w.rest[slot]
		if (slot == 1 || slot == 2) && !strings.EqualFold(r.identity, rows[slot].Identity) {
			*r = markRest{identity: rows[slot].Identity, lastActivity: now}
			w.looks[slot] = workLook{}
		}
		r.observeWork(now, start, age)
		if p.ReduceMotion {
			r.wakeAt = time.Time{}
		}
	}
	if p.ReduceMotion {
		return w.applyWaiting(frames, false, p, now, rows)
	}
	tool := [5]workClock{
		0: w.codex,
		1: w.agy[strings.ToLower(rows[1].Identity)],
		2: w.agy[strings.ToLower(rows[2].Identity)],
	}
	for i, sessions := range w.hooks {
		for _, clock := range sessions {
			before := tool[i+3]
			if !clock.compactingAt.IsZero() && before.compactingAt.IsZero() ||
				clock.compactingAt.IsZero() == before.compactingAt.IsZero() && clock.last.After(before.last) {
				tool[i+3] = clock
			}
		}
	}
	if !w.deepSeekCompact.IsZero() {
		tool[3].compactingAt = w.deepSeekCompact
	}
	active := false
	for slot := range p.slotCount() {
		age := ages[slot]
		r := &w.rest[slot]
		frame, moving := r.frame(now, slot == hover, now.Sub(hoverStart), age >= 0, age, r.asleep(now, start))
		var owner time.Time
		if age >= 0 {
			owner = tool[slot].started
			if owner.IsZero() {
				owner = now.Add(-age)
			}
		}
		want := tool[slot].tool
		if !tool[slot].compactingAt.IsZero() {
			want = agentactivity.Compact
			owner = tool[slot].compactingAt
		}
		if slot >= 3 && want == agentactivity.Command && now.Sub(tool[slot].poseAt) >= 20*time.Second {
			want = agentactivity.LongCommand
		}
		kind, phase := w.looks[slot].follow(now, owner, want)
		if kind == agentactivity.Compact {
			frame = compactFrame(phase)
		} else if kind != agentactivity.Think {
			frame = toolFrame(kind, phase)
		}
		if tool[slot].holds && kind == agentactivity.Think && slot != hover {
			frame, moving = workBase+workFrames-1, false
		}
		if kind == agentactivity.Think && !tool[slot].holds {
			if first, ok := r.wakeFrame(now, slot == 4); ok {
				frame, moving = first, true
			}
		} else {
			r.wakeAt = time.Time{}
		}
		frames[slot], active = frame, active || moving
	}
	return w.applyWaiting(frames, active, p, now, rows)
}

func (w *workingMarks) applyWaiting(frames markFrames, active bool, p placement, now time.Time, rows []row) (markFrames, bool) {
	cues := [5]markCue{0: w.codexWait, 3: w.deepSeekWait}
	for wait, cue := range w.agyWaits {
		if wait.Account == "" {
			continue
		}
		for slot := 1; slot <= 2; slot++ {
			if strings.EqualFold(rows[slot].Identity, wait.Account) && takesWaitingCue(cues[slot], cue) {
				cues[slot] = cue
			}
		}
	}
	for i, sessions := range w.pendingHooks {
		slot := i + 3
		for session, cue := range sessions {
			if now.Sub(cue.at) >= 30*time.Minute {
				delete(sessions, session)
				continue
			}
			if takesWaitingCue(cues[slot], cue) {
				cues[slot] = cue
			}
		}
	}
	for i, cue := range cues[:p.slotCount()] {
		if cue.at.IsZero() {
			continue
		}
		w.rest[i].lastActivity, w.rest[i].nextVisit, w.rest[i].wakeAt = now, time.Time{}, time.Time{}
		w.looks[i] = workLook{} // attention interrupts the previous working look
		frame, anim := pendingFrame(now.Sub(cue.at), cue.look, p.ReduceMotion)
		frames[i] = frame
		active = active || anim
	}
	return frames, active
}

func takesWaitingCue(current, cue markCue) bool {
	return current.at.IsZero() || cue.look > current.look || cue.look == current.look && cue.at.Before(current.at)
}
