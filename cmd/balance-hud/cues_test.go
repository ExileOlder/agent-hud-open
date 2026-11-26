package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ExileOlder/agent-hud-open/internal/codexnotice"
)

func TestNoticeCues(t *testing.T) {
	now := time.Now()
	rows := view(nil)
	place := placement{ShowClaude: true}
	var m mailInbox
	m.receive(mailItem{agent: deepSeekAgent, event: codexnotice.Event{Question: true}}, now, true)
	m.receive(mailItem{agent: 5, event: codexnotice.Event{Question: true}, permission: true}, now, true)
	m.receive(mailItem{agent: 1}, now, true)
	frames, playing := m.cueFrames(now.Add(500*time.Millisecond), place, rows, markFrames{workBase}, true)
	if !playing || frames[0] != workBase || frames[3] != cueBase+6 || frames[4] != cueBase+cueFrames+6 {
		t.Fatalf("cues over working state: %v", frames)
	}
	frames, _ = m.cueFrames(now.Add(time.Second), place, rows, markFrames{}, false)
	if frames[0] != cueBase+2*cueFrames+16 {
		t.Fatalf("completion cue: %v", frames)
	}
	frames, playing = m.cueFrames(now.Add(2500*time.Millisecond), place, rows, markFrames{}, false)
	if playing || frames != (markFrames{}) {
		t.Fatalf("expired cues: %v", frames)
	}
	place.ReduceMotion = true
	m.receive(mailItem{agent: 5, permission: true}, now, true)
	if frames, playing = m.cueFrames(now, place, rows, markFrames{}, false); playing || frames != (markFrames{}) {
		t.Fatalf("reduced motion: %v", frames)
	}
}

func TestAntigravityCueFollowsProfile(t *testing.T) {
	now := time.Now()
	folder := filepath.Join(t.TempDir(), ".gemini")
	token := antigravityCallbackProfile(filepath.Join(folder, "antigravity-ide", "brain", "task", ".system_generated", "logs", "transcript.jsonl"))
	if token != profileKey(folder) {
		t.Fatalf("callback key does not match its Gemini home")
	}
	m := mailInbox{profiles: map[string]uint32{"one@example.test": 123, "two@example.test": token}}
	rows := view(nil)
	rows[1].Identity, rows[2].Identity = "one@example.test", "TWO@example.test"
	item := mailItem{agent: 2, profile: token}
	m.receive(item, now, true)
	frames, _ := m.cueFrames(now, placement{}, rows, markFrames{}, false)
	if frames[1] != 0 || frames[2] != cueBase+2*cueFrames || m.sourceSlot(item, rows) != 2 {
		t.Fatalf("second account cue: %v", frames)
	}
	rows[1], rows[2] = rows[2], rows[1]
	frames, _ = m.cueFrames(now, placement{}, rows, markFrames{}, false)
	if frames[2] != 0 || frames[1] != cueBase+2*cueFrames || m.sourceSlot(item, rows) != 1 {
		t.Fatalf("swapped account cue: %v", frames)
	}
}
