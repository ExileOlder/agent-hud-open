package agentactivity

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// DeepSeekStatus is derived only from the Harness's local client projection.
// Pending requests are reported once, without reading the request content.
type DeepSeekStatus struct {
	Running         bool
	Compacting      bool
	Pending         bool // at least one question remains open
	ApprovalPending bool
	Questions       []string
	Approvals       []string
	Completed       int
}

type deepSeekProjection struct {
	Record struct {
		Rows struct {
			Turn struct {
				Seq   int
				Value struct {
					Open *int `json:"openTurnStartSeq"`
				} `json:"val"`
			} `json:"turnBoundary"`
			Approval struct {
				Value struct{ Policy string } `json:"val"`
			} `json:"approvalState"`
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
			turnSeq, scannedSeq        int
			openTurn                   *int
			approvalDisabled           bool
			log                        deepSeekLifecycle
		}
		cache := map[string]cacheEntry{}
		running, pending, approving, compacting := false, false, false, false
		started := time.Now()
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
					session := strings.TrimSuffix(f.Name(), ".json")
					old := cache[f.Name()]
					c := old
					if c.modified != info.ModTime() {
						view, ok := readDeepSeekProjection(filepath.Join(dir, f.Name()))
						if !ok {
							continue
						}
						rows := view.Record.Rows
						c.modified = info.ModTime()
						c.openTurn = rows.Turn.Value.Open
						c.running = c.openTurn != nil
						c.approvalDisabled = rows.Approval.Value.Policy == "never"
						c.questionCount = len(rows.Questions.Value.Questions.Active)
						c.questionSeq = rows.Questions.Seq
						c.turnSeq = rows.Turn.Seq
						if c.questionCount > 0 && (old.questionCount == 0 || c.questionSeq != old.questionSeq) {
							state.Questions = append(state.Questions, session+"/"+strconv.Itoa(c.questionSeq))
						}
					}
					terminal := 0
					if !c.running && c.turnSeq > 0 && c.turnSeq != c.scannedSeq {
						switch {
						case old.modified.IsZero() && c.modified.Before(started) || now.Sub(c.modified) >= 2*time.Minute:
							// Skip pre-existing history or terminal records that stayed unreadable.
							c.scannedSeq = c.turnSeq
						default:
							terminal = c.turnSeq
						}
					}
					turn := noDeepSeekTurn
					if c.running && !c.approvalDisabled {
						turn = *c.openTurn
					}
					if c.running || c.log.compacting(now) || now.Sub(c.modified) < deepSeekCompactLifetime {
						ids, ending, found := c.log.watch(home, session, turn, terminal)
						for _, id := range ids {
							state.Approvals = append(state.Approvals, session+"/"+id)
						}
						if found {
							c.scannedSeq = terminal
							if ending.successfulSince(started) {
								state.Completed++
							}
						}
					}
					retain[f.Name()] = c
					waitingApproval := len(c.log.pending) > 0
					state.Compacting = state.Compacting || c.log.compacting(now)
					state.ApprovalPending = state.ApprovalPending || waitingApproval
					state.Running = state.Running || (c.running && !c.log.closed && c.questionCount == 0 && !waitingApproval)
					state.Pending = state.Pending || c.questionCount > 0
				}
				cache = retain
				if state.Running != running || state.Compacting != compacting || state.Pending != pending || state.ApprovalPending != approving || len(state.Questions) > 0 || len(state.Approvals) > 0 || state.Completed > 0 {
					select {
					case out <- state:
					case <-ctx.Done():
						return
					}
					running, pending, approving, compacting = state.Running, state.Pending, state.ApprovalPending, state.Compacting
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
