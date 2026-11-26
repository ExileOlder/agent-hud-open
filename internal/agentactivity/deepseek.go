package agentactivity

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DeepSeekStatus is derived only from the Harness's local client projection.
// Pending requests are reported once, without reading the request content.
type DeepSeekStatus struct {
	Running   bool
	Questions int
}

type deepSeekProjection struct {
	Record struct {
		Rows struct {
			Turn struct {
				Value struct {
					Open *int `json:"openTurnStartSeq"`
				} `json:"val"`
			} `json:"turnBoundary"`
			Questions struct {
				Seq   int
				Value struct {
					Questions struct{ Active []struct{} }
				} `json:"val"`
			} `json:"userQuestions"`
		}
	}
}

func readDeepSeekProjection(path string) (deepSeekProjection, bool) {
	var view deepSeekProjection
	f, err := os.Open(path)
	if err != nil {
		return view, false
	}
	defer f.Close()
	err = json.NewDecoder(io.LimitReader(f, 1<<20)).Decode(&view)
	return view, err == nil
}

const deepSeekProjectionDir = "storages/session_projcache/sessions"

func DeepSeekHome(path string) bool {
	if path == "" {
		return false
	}
	d, err := os.Stat(filepath.Join(path, deepSeekProjectionDir))
	return err == nil && d.IsDir()
}

func WatchDeepSeek(ctx context.Context, home string) <-chan DeepSeekStatus {
	if !DeepSeekHome(home) {
		return nil
	}
	out := make(chan DeepSeekStatus, 1)
	go func() {
		defer close(out)
		ticker := time.NewTicker(1500 * time.Millisecond)
		defer ticker.Stop()
		dir := filepath.Join(home, deepSeekProjectionDir)
		type cacheEntry struct {
			modified                   time.Time
			running                    bool
			questionCount, questionSeq int
		}
		cache := map[string]cacheEntry{}
		running := false
		for {
			now := time.Now()
			state := DeepSeekStatus{}
			files, err := os.ReadDir(dir)
			if err == nil {
				retain := map[string]cacheEntry{}
				for _, f := range files {
					if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
						continue
					}
					info, err := f.Info()
					if err != nil || !info.Mode().IsRegular() || now.Sub(info.ModTime()) >= 30*time.Minute {
						continue
					}
					old := cache[f.Name()]
					c := old
					if c.modified != info.ModTime() {
						view, ok := readDeepSeekProjection(filepath.Join(dir, f.Name()))
						if !ok {
							continue
						}
						rows := view.Record.Rows
						c = cacheEntry{info.ModTime(), rows.Turn.Value.Open != nil, len(rows.Questions.Value.Questions.Active), rows.Questions.Seq}
						if c.questionCount > 0 && (old.questionCount == 0 || c.questionSeq != old.questionSeq) {
							state.Questions++
						}
					}
					retain[f.Name()] = c
					state.Running = state.Running || (c.running && c.questionCount == 0)
				}
				cache = retain
				if state.Running != running || state.Questions > 0 {
					select {
					case out <- state:
					case <-ctx.Done():
						return
					}
					running = state.Running
				}
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
