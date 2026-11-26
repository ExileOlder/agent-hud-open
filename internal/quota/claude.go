package quota

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"time"
)

const claudeUsageID = "balance-hud-usage"

// Claude asks the installed official client what its subscription allows.
// Like Agent HUD's engine_usage.rs, it sends an engine control message, no
// user turn; OAuth and account selection stay inside Claude Code.
func Claude(ctx context.Context, executable string) (Account, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	dir, err := os.MkdirTemp("", "balance-hud-claude-")
	if err != nil {
		return Account{}, err
	}
	defer os.Remove(dir)
	cmd := exec.CommandContext(ctx, executable, "-p", "--input-format", "stream-json",
		"--output-format", "stream-json", "--verbose", "--settings", `{"disableAllHooks":true}`)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CLAUDE_CODE_ENTRYPOINT=balance-hud")
	configureProcess(cmd)
	in, err := cmd.StdinPipe()
	if err != nil {
		return Account{}, err
	}
	defer in.Close()
	out, err := cmd.StdoutPipe()
	if err != nil {
		return Account{}, err
	}
	defer out.Close()
	if err = cmd.Start(); err != nil {
		return Account{}, err
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()
	request := map[string]any{"type": "control_request", "request_id": claudeUsageID,
		"request": map[string]any{"subtype": "get_usage", "skip_behaviors": true}}
	if err = json.NewEncoder(in).Encode(request); err != nil {
		return Account{}, err
	}
	scan := bufio.NewScanner(out)
	scan.Buffer(make([]byte, 4096), 1<<20)
	for scan.Scan() {
		var msg struct {
			Type     string
			Response struct {
				ID      string `json:"request_id"`
				Subtype string
				Payload json.RawMessage `json:"response"`
			}
		}
		if json.Unmarshal(scan.Bytes(), &msg) != nil || msg.Type != "control_response" || msg.Response.ID != claudeUsageID {
			continue
		}
		if msg.Response.Subtype != "success" {
			return Account{}, errors.New("Claude Code rejected the quota request")
		}
		return parseClaude(msg.Response.Payload)
	}
	if ctx.Err() != nil {
		return Account{}, ctx.Err()
	}
	return Account{}, errors.New("Claude Code returned no quota response")
}

func parseClaude(payload []byte) (Account, error) {
	type limit struct {
		Used  *float64 `json:"utilization"`
		Reset string   `json:"resets_at"`
	}
	var response struct {
		Available  bool             `json:"rate_limits_available"`
		RateLimits map[string]limit `json:"rate_limits"`
	}
	if err := json.Unmarshal(payload, &response); err != nil {
		return Account{}, err
	}
	if !response.Available {
		return Account{}, errors.New("Sign in to Claude Code with a subscription")
	}
	a := Account{Label: "Claude Code"}
	for _, key := range []struct{ id, label string }{
		{"five_hour", "5h"}, {"seven_day", "7d"}, {"seven_day_opus", "Opus 7d"},
		{"seven_day_sonnet", "Sonnet 7d"}, {"seven_day_haiku", "Haiku 7d"},
	} {
		w := response.RateLimits[key.id]
		if w.Used != nil && valid(*w.Used) {
			a.Windows = append(a.Windows, Window{key.label, 100 - *w.Used, w.Reset})
		}
	}
	if len(a.Windows) == 0 {
		return Account{}, errors.New("Claude Code returned no subscription windows")
	}
	return a, nil
}
