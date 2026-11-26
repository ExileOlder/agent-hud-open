package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

var agentNames = []string{"", "Codex", "Antigravity", "Mistral Vibe", "Agent", "Claude Code"}

func completionID(name string) int {
	switch strings.ToLower(name) {
	case "codex":
		return 1
	case "antigravity":
		return 2
	case "mistral", "mistral-vibe":
		return 3
	case "agent":
		return 4
	case "claude", "claude-code":
		return 5
	}
	return 0
}

// A generic Agent hook can show a notice but has no known destination.
func completionSlot(agent int) int {
	switch agent {
	case 1:
		return 0
	case 2:
		return 1
	case 3:
		return 3
	case 5:
		return 4
	default:
		return -1
	}
}

// The hook payload can contain conversation text. Read only completion markers
// and send only a fixed agent label to the desktop notifier.
func completed(id int, r io.Reader) bool {
	var event struct {
		Event        string     `json:"hook_event_name"`
		Notification string     `json:"notification_type"`
		StopReason   string     `json:"terminationReason"`
		Idle         bool       `json:"fullyIdle"`
		Error        string     `json:"error"`
		Background   []struct{} `json:"background_tasks"`
		Cron         []struct{} `json:"session_crons"`
	}
	if json.NewDecoder(io.LimitReader(r, 1<<20)).Decode(&event) != nil {
		return false
	}
	switch id {
	case 1:
		return event.Event == "Stop"
	case 2:
		return event.Idle && event.Error == "" && (event.StopReason == "NO_TOOL_CALL" || event.StopReason == "model_stop")
	case 3:
		return event.Notification == "turn_completed"
	case 5:
		return event.Event == "Stop" && len(event.Background) == 0 && len(event.Cron) == 0
	}
	return false
}

func runCompletion(name string, hook bool, input io.Reader, output io.Writer) error {
	id := completionID(name)
	if id == 0 {
		return fmt.Errorf("unknown completion source: %s", name)
	}
	if hook {
		// A failed notification must never stop a client turn or make it continue.
		if id == 2 {
			defer fmt.Fprintln(output, `{"decision":"stop"}`)
		}
		if !completed(id, input) {
			return nil
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return notifyCompletion(ctx, id)
}
