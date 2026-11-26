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
func antigravityWorking(tail []byte) bool {
	for end := len(tail); end > 0; {
		i := bytes.LastIndexByte(tail[:end], '\n')
		var s step
		if json.Unmarshal(tail[i+1:end], &s) == nil {
			switch s.Kind {
			case "USER_INPUT":
				if s.Source != "SYSTEM" {
					return true
				}
			case "PLANNER_RESPONSE":
				if s.Status == "ERROR" || s.Status == "CANCELED" {
					return false
				}
				return len(s.Tools) > 0 || s.Status != "DONE"
			}
		}
		end = i
	}
	return false
}

func readAntigravityTail(path string) bool {
	const bytesToRead = 128 << 10
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	if info.Size() > bytesToRead {
		if _, err = f.Seek(-bytesToRead, io.SeekEnd); err != nil {
			return false
		}
	}
	b, err := io.ReadAll(io.LimitReader(f, bytesToRead))
	return err == nil && antigravityWorking(b)
}

type cached struct {
	modified time.Time
	size     int64
	running  bool
}

func WatchAntigravity(ctx context.Context, profiles []AntigravityProfile) <-chan map[string]bool {
	if len(profiles) == 0 {
		return nil
	}
	out := make(chan map[string]bool, 1)
	go func() {
		defer close(out)
		ticker := time.NewTicker(2500 * time.Millisecond)
		defer ticker.Stop()
		files := map[string]cached{}
		before := map[string]bool{}
		for {
			now := time.Now()
			next := map[string]bool{}
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
							state = cached{info.ModTime(), info.Size(), readAntigravityTail(path)}
						}
						keep[path] = state
						if state.running {
							next[strings.ToLower(p.Account)] = true
						}
					}
				}
			}
			files = keep
			changed := len(next) != len(before)
			for key := range next {
				if !before[key] {
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
