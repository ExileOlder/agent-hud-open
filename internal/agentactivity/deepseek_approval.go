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

	"github.com/klauspost/compress/zstd"
)

// The projection stores only the approval policy, not outstanding requests.
// Harness appends complete Zstandard frames containing newline-terminated
// events, with approval/asked and approval/decided sharing a request ID.
type deepSeekApprovals struct {
	path    string
	info    os.FileInfo
	offset  int64 // last completely decoded compressed frame boundary
	turn    int
	closed  bool
	pending map[string]struct{}
}

func (a *deepSeekApprovals) watch(home, session string, turn int) int {
	if a.path == "" {
		groups, err := os.ReadDir(filepath.Join(home, "sessions"))
		if err != nil {
			return 0
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
			return 0
		}
	}
	if a.turn != turn {
		a.turn, a.info, a.offset, a.pending, a.closed = turn, nil, 0, nil, false
	}
	if a.closed {
		return 0
	}
	n, _ := a.read()
	return n
}

func (a *deepSeekApprovals) read() (int, bool) {
	f, err := os.Open(a.path)
	if err != nil {
		return 0, false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return 0, false
	}
	offset, pending := a.offset, a.pending
	if a.info == nil || !os.SameFile(a.info, info) || info.Size() < offset ||
		(info.Size() == offset && !info.ModTime().Equal(a.info.ModTime())) {
		offset, pending = 0, nil
	}
	if info.Size() == offset {
		if offset == 0 {
			a.info, a.offset, a.pending = info, 0, nil
		}
		return 0, true
	}
	stream := io.NewSectionReader(f, offset, info.Size()-offset)
	dec, err := zstd.NewReader(stream, zstd.WithDecoderConcurrency(1), zstd.WithDecoderLowmem(true), zstd.WithDecoderMaxMemory(16<<20))
	if err != nil {
		return 0, false
	}
	defer dec.Close()
	next, closed, ok := deepSeekApprovalTail(dec, a.turn, maps.Clone(pending))
	if !ok {
		return 0, false // Writer may still be appending a frame. Retry from the same boundary.
	}
	n := 0
	for id := range next {
		if _, known := a.pending[id]; !known {
			n++
		}
	}
	a.info, a.offset, a.pending, a.closed = info, info.Size(), next, closed
	return n, true
}

func deepSeekApprovalTail(r io.Reader, turn int, pending map[string]struct{}) (map[string]struct{}, bool, bool) {
	closed := false
	br := bufio.NewReaderSize(r, 64<<10)
	for {
		line, err := br.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			// Conversation and tool results may be large. Only small
			// approval status records are projected out of the stream.
			for errors.Is(err, bufio.ErrBufferFull) {
				line, err = br.ReadSlice('\n')
			}
		} else if err == nil && !closed && (bytes.Contains(line, []byte(`"approval/asked"`)) ||
			bytes.Contains(line, []byte(`"approval/decided"`)) || bytes.Contains(line, []byte(`"turn/end"`))) {
			var event struct {
				Type string
				Seq  int
				Data struct{ ID string }
			}
			if json.Unmarshal(line, &event) == nil && event.Seq >= turn {
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
					// Ignore events outside the open turn reported by the
					// projection, including a newer turn not yet projected.
					// Keep reading so a torn tail never advances the cursor.
					clear(pending)
					closed = true
				}
			}
		}
		if err != nil {
			return pending, closed, err == io.EOF && len(line) == 0
		}
	}
}
