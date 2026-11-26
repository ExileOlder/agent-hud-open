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
	path    string
	info    os.FileInfo
	offset  int64 // last completely decoded compressed frame boundary
	turn    int
	closed  bool
	pending map[string]struct{}
	compact deepSeekCompaction
}

const noDeepSeekTurn = -1
const deepSeekCompactLifetime = 5 * time.Minute

type deepSeekCompaction struct {
	id string
	at time.Time
}

func (a *deepSeekLifecycle) compacting(now time.Time) bool {
	return !a.compact.at.IsZero() && now.Sub(a.compact.at) < deepSeekCompactLifetime
}

func (a *deepSeekLifecycle) watch(home, session string, turn int) []string {
	if a.path == "" {
		groups, err := os.ReadDir(filepath.Join(home, "sessions"))
		if err != nil {
			return nil
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
			return nil
		}
	}
	if a.turn != turn {
		a.turn, a.info, a.offset, a.pending, a.closed, a.compact = turn, nil, 0, nil, false, deepSeekCompaction{}
	}
	n, _ := a.read()
	return n
}

func (a *deepSeekLifecycle) read() ([]string, bool) {
	f, err := os.Open(a.path)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, false
	}
	offset, pending, closed, compact := a.offset, a.pending, a.closed, a.compact
	if a.info == nil || !os.SameFile(a.info, info) || info.Size() < offset ||
		(info.Size() == offset && !info.ModTime().Equal(a.info.ModTime())) {
		offset, pending, closed, compact = 0, nil, false, deepSeekCompaction{}
	}
	if info.Size() == offset {
		if offset == 0 {
			a.info, a.offset, a.pending, a.closed, a.compact = info, 0, nil, false, deepSeekCompaction{}
		}
		return nil, true
	}
	// For closed, idle sessions with only old data, wait until the log is
	// fresh before inflating it. No five-minute compaction can still be live.
	if offset == 0 && a.turn == noDeepSeekTurn && time.Since(info.ModTime()) >= deepSeekCompactLifetime {
		return nil, true
	}
	stream := io.NewSectionReader(f, offset, info.Size()-offset)
	dec, err := zstd.NewReader(stream, zstd.WithDecoderConcurrency(1), zstd.WithDecoderLowmem(true), zstd.WithDecoderMaxMemory(16<<20))
	if err != nil {
		return nil, false
	}
	defer dec.Close()
	next, closed, compact, ok := deepSeekLifecycleTail(dec, a.turn, maps.Clone(pending), closed, compact)
	if !ok {
		return nil, false // Writer may still be appending a frame. Retry from the same boundary.
	}
	var ids []string
	for id := range next {
		if _, known := a.pending[id]; !known {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	a.info, a.offset, a.pending, a.closed, a.compact = info, info.Size(), next, closed, compact
	return ids, true
}

func deepSeekLifecycleTail(r io.Reader, turn int, pending map[string]struct{}, closed bool, compact deepSeekCompaction) (map[string]struct{}, bool, deepSeekCompaction, bool) {
	br := bufio.NewReaderSize(r, 64<<10)
	for {
		line, err := br.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			// Conversation and tool results may be large. Only small
			// lifecycle records are projected out of the stream.
			for errors.Is(err, bufio.ErrBufferFull) {
				line, err = br.ReadSlice('\n')
			}
		} else if err == nil && (bytes.Contains(line, []byte(`"compaction/start"`)) ||
			bytes.Contains(line, []byte(`"compaction/end"`)) ||
			turn != noDeepSeekTurn && !closed && (bytes.Contains(line, []byte(`"approval/asked"`)) ||
				bytes.Contains(line, []byte(`"approval/decided"`)) || bytes.Contains(line, []byte(`"turn/end"`)))) {
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
						compact = deepSeekCompaction{event.Data.CompactionID, time.UnixMilli(event.Time)}
					}
				case "compaction/end":
					if compact.id == event.Data.CompactionID {
						compact = deepSeekCompaction{}
					}
				default:
					if turn == noDeepSeekTurn || closed || event.Seq < turn {
						break
					}
					switch event.Type {
					case "approval/asked":
						if event.Data.ID != "" {
							if pending == nil {
								pending = make(map[string]struct{})
							}
							pending[event.Data.ID] = struct{}{}
						}
					case "approval/decided":
						delete(pending, event.Data.ID)
					case "turn/end":
						clear(pending)
						closed = true // A stale projection cannot expose requests from a newer turn.
					}
				}
			}
		}
		if err != nil {
			return pending, closed, compact, err == io.EOF && len(line) == 0
		}
	}
}
