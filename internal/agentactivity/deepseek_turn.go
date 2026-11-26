package agentactivity

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/klauspost/compress/zstd"
)

// The projection's turnBoundary.seq identifies the canonical turn/end line.
// The projection does not retain its outcome, so a closed turn alone cannot
// distinguish success, cancellation, or an error.
type deepSeekTurnEnd struct {
	Type string
	Seq  int
	Time int64
	Data struct{ Reason struct{ Kind string } }
}

func findDeepSeekTurnEnd(r io.Reader, seq int) (deepSeekTurnEnd, bool) {
	br := bufio.NewReaderSize(r, 64<<10)
	for {
		line, err := br.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			// Tool results can be much larger than lifecycle records. Discard a long
			// line in place, then continue to the next event.
			for errors.Is(err, bufio.ErrBufferFull) {
				_, err = br.ReadSlice('\n')
			}
		} else if bytes.Contains(line, []byte(`"turn/end"`)) {
			var event deepSeekTurnEnd
			if json.Unmarshal(line, &event) == nil && event.Type == "turn/end" && event.Seq == seq {
				return event, true
			}
		}
		if err != nil {
			return deepSeekTurnEnd{}, false
		}
	}
}

func readDeepSeekTurnEnd(home, session string, seq int) (deepSeekTurnEnd, bool) {
	groups, err := os.ReadDir(filepath.Join(home, "sessions"))
	if err != nil {
		return deepSeekTurnEnd{}, false
	}
	for _, group := range groups {
		if !group.IsDir() {
			continue
		}
		path := filepath.Join(home, "sessions", group.Name(), session, "session.v4.jsonl.zstd")
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		dec, err := zstd.NewReader(f, zstd.WithDecoderConcurrency(1), zstd.WithDecoderLowmem(true), zstd.WithDecoderMaxMemory(16<<20))
		var event deepSeekTurnEnd
		found := false
		if err == nil {
			event, found = findDeepSeekTurnEnd(dec, seq)
			dec.Close()
		}
		f.Close()
		if found {
			return event, true
		}
	}
	return deepSeekTurnEnd{}, false
}

func (e deepSeekTurnEnd) successfulSince(start time.Time) bool {
	return e.Data.Reason.Kind == "completed" && e.Time >= start.UnixMilli()
}
