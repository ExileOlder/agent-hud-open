// Package agentactivity observes local lifecycle metadata for HUD artwork.
package agentactivity

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type AntigravityProfile struct {
	Account    string
	GeminiHome string
}

type step struct {
	Kind   string            `json:"type"`
	Source string            `json:"source"`
	Status string            `json:"status"`
	Tools  []json.RawMessage `json:"tool_calls"`
}

// The upstream reducer treats user input and planner tool calls as in flight.
// A planner response without tools ends on DONE, ERROR or CANCELED. Tool
// responses only renew existing work. No conversation text leaves this reader.
func antigravityPose(tail []byte) (Tool, bool) {
	returned := false
	for end := len(tail); end > 0; {
		i := bytes.LastIndexByte(tail[:end], '\n')
		var s step
		if json.Unmarshal(tail[i+1:end], &s) == nil {
			switch s.Kind {
			case "USER_INPUT":
				if s.Source != "SYSTEM" {
					return Think, true
				}
			case "TOOL_RESPONSE", "GENERIC":
				returned = true
			case "PLANNER_RESPONSE":
				if s.Status == "ERROR" || s.Status == "CANCELED" {
					return Think, false
				}
				if !returned {
					for _, raw := range s.Tools {
						var call struct {
							Name string `json:"name"`
						}
						if json.Unmarshal(raw, &call) != nil {
							_ = json.Unmarshal(raw, &call.Name)
						}
						if kind := ClassifyTool(call.Name); kind != Think {
							return kind, true
						}
					}
				}
				return Think, len(s.Tools) > 0 || s.Status != "DONE"
			}
		}
		end = i
	}
	return Think, false
}

func readAntigravityTail(path string) (Tool, bool) {
	const bytesToRead = 128 << 10
	f, err := os.Open(path)
	if err != nil {
		return Think, false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return Think, false
	}
	if info.Size() > bytesToRead {
		if _, err = f.Seek(-bytesToRead, io.SeekEnd); err != nil {
			return Think, false
		}
	}
	b, err := io.ReadAll(io.LimitReader(f, bytesToRead))
	if err != nil {
		return Think, false
	}
	return antigravityPose(b)
}

type cached struct {
	modified time.Time
	size     int64
	running  bool
	tool     Tool
}

func WatchAntigravity(ctx context.Context, profiles []AntigravityProfile) <-chan map[string]Tool {
	if len(profiles) == 0 {
		return nil
	}
	out := make(chan map[string]Tool, 1)
	go func() {
		defer close(out)
		ticker := time.NewTicker(2500 * time.Millisecond)
		defer ticker.Stop()
		files := map[string]cached{}
		before := map[string]Tool{}
		for {
			now := time.Now()
			next := map[string]Tool{}
			bestAt := map[string]time.Time{}
			keep := map[string]cached{}
			for _, p := range profiles {
				if p.Account == "" || !filepath.IsAbs(p.GeminiHome) {
					continue
				}
				for _, client := range []string{"antigravity", "antigravity-cli", "antigravity-ide"} {
					dirs, err := os.ReadDir(filepath.Join(p.GeminiHome, client, "brain"))
					if err != nil {
						continue
					}
					for _, dir := range dirs {
						if !dir.IsDir() {
							continue
						}
						path := filepath.Join(p.GeminiHome, client, "brain", dir.Name(), ".system_generated", "logs", "transcript.jsonl")
						info, err := os.Stat(path)
						if err != nil || !info.Mode().IsRegular() || now.Sub(info.ModTime()) >= 30*time.Minute {
							continue
						}
						state := files[path]
						if state.modified != info.ModTime() || state.size != info.Size() {
							tool, running := readAntigravityTail(path)
							state = cached{info.ModTime(), info.Size(), running, tool}
						}
						keep[path] = state
						if state.running {
							key := strings.ToLower(p.Account)
							if at := bestAt[key]; at.IsZero() || (next[key] == Think && state.tool != Think) || (next[key] == Think) == (state.tool == Think) && state.modified.After(at) {
								next[key], bestAt[key] = state.tool, state.modified
							}
						}
					}
				}
			}
			files = keep
			changed := len(next) != len(before)
			for key, kind := range next {
				if prev, ok := before[key]; !ok || prev != kind {
					changed = true
					break
				}
			}
			if changed {
				select {
				case out <- next:
				case <-ctx.Done():
					return
				}
				before = next
			}
			select {
			case <-ticker.C:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}
