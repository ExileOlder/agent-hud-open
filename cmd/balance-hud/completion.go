package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/ExileOlder/agent-hud-open/internal/agentactivity"
	"hash/fnv"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const deepSeekAgent = 6
const deepSeekDisplayName = "Deepseek"

var agentNames = []string{"", "Codex", "Antigravity", deepSeekDisplayName, "Agent", "Claude Code", deepSeekDisplayName}

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
	case 3, deepSeekAgent:
		return 3
	case 5:
		return 4
	default:
		return -1
	}
}

// The local IPC carries only the provider, phase and an opaque session key.
type hookPhase int

const (
	hookComplete hookPhase = iota
	hookWork
	hookWait
	hookEnd
	hookRead
	hookEdit
	hookCommand
	hookPermission
	hookCompact
	hookCompactEnd
)
const hookPhaseStep = 8
const codexCompletionCode = 1
const codexApprovalCode = codexCompletionCode + hookPhaseStep*int(hookPermission)

type hookEvent struct {
	agent   int
	phase   hookPhase
	session uint32
	thread  codexHookThread
}

func (e hookEvent) code() int { return e.agent + hookPhaseStep*int(e.phase) }
func decodeHook(code int, session uint32) hookEvent {
	agent, phase := code%hookPhaseStep, hookPhase(code/hookPhaseStep)
	if agent <= 0 || agent >= len(agentNames) || phase < hookComplete || phase > hookCompactEnd {
		return hookEvent{}
	}
	if phase >= hookCompact && agent != 5 {
		return hookEvent{}
	}
	if phase != hookComplete && ((agent != 3 && agent != 5) || session == 0) {
		return hookEvent{}
	}
	return hookEvent{agent: agent, phase: phase, session: session}
}

func parseHook(id int, r io.Reader) hookEvent {
	var event struct {
		Event        string     `json:"hook_event_name"`
		Notification string     `json:"notification_type"`
		Session      string     `json:"session_id"`
		Parent       string     `json:"parent_session_id"`
		Tool         string     `json:"tool_name"`
		Transcript   string     `json:"transcriptPath"`
		StopReason   string     `json:"terminationReason"`
		Idle         bool       `json:"fullyIdle"`
		Error        string     `json:"error"`
		Background   []struct{} `json:"background_tasks"`
		Cron         []struct{} `json:"session_crons"`
	}
	if json.NewDecoder(io.LimitReader(r, 1<<20)).Decode(&event) != nil {
		return hookEvent{}
	}
	var session uint32
	if event.Session != "" && (id == 3 || id == 5) {
		session = opaqueKey(event.Session)
	} else if id == 2 {
		session = antigravityCallbackProfile(event.Transcript)
	}
	phase := hookComplete
	switch id {
	case 1:
		if event.Event == "PermissionRequest" {
			return hookEvent{agent: id, phase: hookPermission, thread: parseCodexHookThread(event.Session)}
		}
		if event.Event != "Stop" {
			return hookEvent{}
		}
		return hookEvent{agent: id, phase: hookComplete, thread: parseCodexHookThread(event.Session)}
	case 2:
		if !event.Idle || event.Error != "" || (event.StopReason != "NO_TOOL_CALL" && event.StopReason != "model_stop") {
			return hookEvent{}
		}
	case 3:
		if event.Parent != "" {
			return hookEvent{}
		}
		switch event.Notification {
		case "turn_completed":
		case "turn_failed":
			phase = hookEnd
		case "approval_requested":
			phase = hookPermission
		case "question_asked":
			phase = hookWait
		default:
			switch event.Event {
			case "pre_tool":
				phase = toolPhase(event.Tool)
			case "pre_agent", "pre_llm", "post_tool":
				phase = hookWork
			case "session_end":
				phase = hookEnd
			default:
				return hookEvent{}
			}
		}
	case 5:
		switch event.Event {
		case "Stop":
			if len(event.Background) > 0 || len(event.Cron) > 0 {
				return hookEvent{}
			}
		case "UserPromptSubmit", "PostToolUse", "PostToolUseFailure":
			phase = hookWork
		case "PermissionRequest":
			phase = hookPermission
		case "PreToolUse":
			if event.Tool == "AskUserQuestion" {
				phase = hookWait
			} else {
				phase = toolPhase(event.Tool)
			}
		case "PreCompact":
			phase = hookCompact
		case "PostCompact":
			phase = hookCompactEnd
		case "SessionEnd", "StopFailure":
			phase = hookEnd
		default:
			return hookEvent{}
		}
	default:
		return hookEvent{}
	}
	return decodeHook(id+int(phase)*hookPhaseStep, session)
}

func opaqueKey(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	if key := h.Sum32(); key != 0 {
		return key
	}
	return 1
}
func profileKey(path string) uint32 {
	normalized := filepath.Clean(path)
	if runtime.GOOS == "windows" {
		normalized = strings.ToLower(normalized)
	}
	return opaqueKey(normalized)
}
func antigravityCallbackProfile(transcript string) uint32 {
	if filepath.IsAbs(transcript) {
		for parent := filepath.Dir(transcript); filepath.Dir(parent) != parent; parent = filepath.Dir(parent) {
			switch filepath.Base(parent) {
			case "antigravity", "antigravity-cli", "antigravity-ide":
				return profileKey(filepath.Dir(parent))
			}
		}
	}
	if home := os.Getenv("GEMINI_CLI_HOME"); filepath.IsAbs(home) {
		return profileKey(home)
	}
	if home, err := os.UserHomeDir(); err == nil {
		return profileKey(filepath.Join(home, ".gemini"))
	}
	return 0
}

func toolPhase(name string) hookPhase {
	switch agentactivity.ClassifyTool(name) {
	case agentactivity.Read:
		return hookRead
	case agentactivity.Edit:
		return hookEdit
	case agentactivity.Command:
		return hookCommand
	default:
		return hookWork
	}
}

func runCompletion(name string, hook bool, input io.Reader, output io.Writer) error {
	id := completionID(name)
	if id == 0 {
		return fmt.Errorf("unknown completion source: %s", name)
	}
	event := hookEvent{agent: id}
	if hook {
		// A failed notification must never stop a client turn or make it continue.
		if id == 2 {
			defer fmt.Fprintln(output, `{"decision":"stop"}`)
		}
		event = parseHook(id, input)
		if event.agent == 0 {
			return nil
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if event.agent == 1 && (event.phase == hookPermission || event.thread != (codexHookThread{})) {
		return notifyCodexHook(ctx, event.thread, event.phase == hookPermission)
	}
	return notifyHook(ctx, event.code(), event.session)
}

// Only the UUID reaches the GUI. Tool names, approval reasons and payloads
// stay in the client; an unfamiliar session ID opens Codex without a route.
type codexHookThread [16]byte

func parseCodexHookThread(id string) (thread codexHookThread) {
	if codexThreadID.MatchString(id) {
		_, _ = hex.Decode(thread[:], []byte(strings.ReplaceAll(id, "-", "")))
	}
	return
}

func (t codexHookThread) route() string {
	if t == (codexHookThread{}) {
		return ""
	}
	id := hex.EncodeToString(t[:])
	return id[:8] + "-" + id[8:12] + "-" + id[12:16] + "-" + id[16:20] + "-" + id[20:]
}
