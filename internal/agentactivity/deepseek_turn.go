package agentactivity

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
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

func readDeepSeekTurnEnd(path string, seq int) (deepSeekTurnEnd, bool) {
	f, err := os.Open(path)
	if err != nil {
		return deepSeekTurnEnd{}, false
	}
	defer f.Close()
	dec, err := zstd.NewReader(f, zstd.WithDecoderConcurrency(1), zstd.WithDecoderLowmem(true), zstd.WithDecoderMaxMemory(16<<20))
	if err != nil {
		return deepSeekTurnEnd{}, false
	}
	defer dec.Close()
	return findDeepSeekTurnEnd(dec, seq)
}

func (e deepSeekTurnEnd) successfulSince(start time.Time) bool {
	return e.Data.Reason.Kind == "completed" && e.Time >= start.UnixMilli()
}
