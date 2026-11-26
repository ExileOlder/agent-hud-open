package agentactivity

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/klauspost/compress/zstd"
)

// Harness appends complete Zstandard frames containing newline-terminated
// events. Observe compaction even outside a turn; approvals belong to the
// open turn selected by the projection.
type deepSeekLifecycle struct {
	path        string
	info        os.FileInfo
	offset      int64 // last completely decoded compressed frame boundary
	state       deepSeekLogState
	pending     map[string]struct{} // pending for the currently projected open turn
	visibleTurn int
	closed      bool
	backfill    struct {
		seq int
		at  int64
		end deepSeekTurnEnd
	}
}

type deepSeekLogState struct {
	turn          int // start of the latest turn
	pending       map[string]struct{}
	compact       deepSeekCompaction
	ending, match deepSeekTurnEnd
}

const noDeepSeekTurn = -1
const deepSeekCompactLifetime = 5 * time.Minute

type deepSeekCompaction struct {
	id string
	at time.Time
}

func (a *deepSeekLifecycle) compacting(now time.Time) bool {
	return !a.state.compact.at.IsZero() && now.Sub(a.state.compact.at) < deepSeekCompactLifetime
}

func (a *deepSeekLifecycle) watch(home, session string, turn, terminal int) ([]string, deepSeekTurnEnd, bool) {
	if a.path == "" {
		groups, err := os.ReadDir(filepath.Join(home, "sessions"))
		if err != nil {
			return nil, deepSeekTurnEnd{}, false
		}
		for _, group := range groups {
			if !group.IsDir() {
				continue
			}
			path := filepath.Join(home, "sessions", group.Name(), session, "session.v4.jsonl.zstd")
			if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
				a.path = path
				break
			}
		}
		if a.path == "" {
			return nil, deepSeekTurnEnd{}, false
		}
	}

	a.read(turn, terminal)
	var oldPending map[string]struct{}
	if turn == a.visibleTurn {
		oldPending = a.pending
	}
	a.visibleTurn, a.pending, a.closed = turn, nil, false
	if turn != noDeepSeekTurn {
		a.closed = a.state.ending.Seq >= turn || a.state.turn > turn
		if a.state.turn == turn && !a.closed {
			a.pending = a.state.pending
		}
	}
	var ids []string
	for id := range a.pending {
		if _, known := oldPending[id]; !known {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	if terminal <= 0 {
		return ids, deepSeekTurnEnd{}, false
	}
	if e := a.state.ending; e.Seq == terminal {
		return ids, e, true
	}
	if e := a.state.match; e.Seq == terminal {
		return ids, e, true
	}
	if a.state.ending.Seq > terminal {
		// Only a lagging projection can request a terminal older than the
		// latest one already read. Look it up once per file position.
		if a.backfill.seq != terminal || a.backfill.at != a.offset {
			e, _ := readDeepSeekTurnEnd(a.path, terminal)
			a.backfill.seq, a.backfill.at, a.backfill.end = terminal, a.offset, e
		}
		if e := a.backfill.end; e.Seq == terminal {
			return ids, e, true
		}
	}
	return ids, deepSeekTurnEnd{}, false
}

func (a *deepSeekLifecycle) read(turn, terminal int) {
	f, err := os.Open(a.path)
	if err != nil {
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return
	}
	offset, state := a.offset, a.state
	if a.info == nil || !os.SameFile(a.info, info) || info.Size() < offset ||
		(info.Size() == offset && !info.ModTime().Equal(a.info.ModTime())) {
		offset, state = 0, deepSeekLogState{}
		a.backfill.seq = 0
	}
	if info.Size() == offset {
		a.info, a.offset, a.state = info, offset, state
		return
	}
	// An old, closed log does not have any currently observable compaction.
	if offset == 0 && turn == noDeepSeekTurn && terminal == 0 && time.Since(info.ModTime()) >= deepSeekCompactLifetime {
		return
	}
	if state.turn == 0 && turn > state.ending.Seq {
		state.turn = turn // the projection may already know the first open turn
	}
	stream := io.NewSectionReader(f, offset, info.Size()-offset)
	dec, err := zstd.NewReader(stream, zstd.WithDecoderConcurrency(1), zstd.WithDecoderLowmem(true), zstd.WithDecoderMaxMemory(16<<20))
	if err != nil {
		return
	}
	defer dec.Close()
	state.pending = maps.Clone(state.pending)
	next, ok := deepSeekLifecycleTail(dec, state, terminal)
	if ok {
		a.info, a.offset, a.state = info, info.Size(), next
	}
	// An incomplete frame leaves the cursor unchanged until the writer flushes.
}

func deepSeekLifecycleTail(r io.Reader, state deepSeekLogState, terminal int) (deepSeekLogState, bool) {
	br := bufio.NewReaderSize(r, 64<<10)
	for {
		line, err := br.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			for errors.Is(err, bufio.ErrBufferFull) {
				line, err = br.ReadSlice('\n')
			}
		} else if err == nil && (bytes.Contains(line, []byte(`"compaction/start"`)) ||
			bytes.Contains(line, []byte(`"compaction/end"`)) ||
			bytes.Contains(line, []byte(`"turn/start"`)) ||
			bytes.Contains(line, []byte(`"turn/end"`)) ||
			bytes.Contains(line, []byte(`"approval/asked"`)) ||
			bytes.Contains(line, []byte(`"approval/decided"`))) {
			var event struct {
				Type string
				Seq  int
				Time int64
				Data struct {
					ID           string
					CompactionID string `json:"compactionId"`
				}
			}
			if json.Unmarshal(line, &event) == nil {
				switch event.Type {
				case "compaction/start":
					if event.Data.CompactionID != "" && event.Time > 0 {
						state.compact = deepSeekCompaction{event.Data.CompactionID, time.UnixMilli(event.Time)}
					}
				case "compaction/end":
					if state.compact.id == event.Data.CompactionID {
						state.compact = deepSeekCompaction{}
					}
				case "turn/start":
					if event.Seq >= state.turn {
						state.turn, state.pending = event.Seq, nil
					}
				case "turn/end":
					var ending deepSeekTurnEnd
					if json.Unmarshal(line, &ending) == nil {
						state.ending = ending
						if ending.Seq == terminal {
							state.match = ending
						}
					}
					if event.Seq >= state.turn {
						state.pending = nil
					}
				case "approval/asked", "approval/decided":
					if state.turn == 0 || event.Seq < state.turn || state.ending.Seq >= state.turn {
						break
					}
					if event.Type == "approval/asked" && event.Data.ID != "" {
						if state.pending == nil {
							state.pending = make(map[string]struct{})
						}
						state.pending[event.Data.ID] = struct{}{}
					} else {
						delete(state.pending, event.Data.ID)
					}
				}
			}
		}
		if err != nil {
			return state, err == io.EOF && len(line) == 0
		}
	}
}
