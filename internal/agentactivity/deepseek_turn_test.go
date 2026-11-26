package agentactivity

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
)

func TestMatchingTerminalOnly(t *testing.T) {
	now := time.Now()
	var log bytes.Buffer
	add := func(seq int, kind string, at time.Time) {
		fmt.Fprintf(&log, `{"type":"turn/end","seq":%d,"time":%d,"data":{"reason":{"kind":%q}}}`+"\n", seq, at.UnixMilli(), kind)
	}
	add(10, "completed", now.Add(-time.Hour))
	fmt.Fprintf(&log, `{"type":"tool/end","data":%q}`+"\n", strings.Repeat("x", 200<<10))
	add(20, "aborted", now)
	add(30, "error", now)
	add(40, "completed", now)
	for _, tc := range []struct {
		seq            int
		found, success bool
	}{
		{10, true, false}, {20, true, false}, {30, true, false}, {40, true, true}, {41, false, false},
	} {
		e, found := findDeepSeekTurnEnd(bytes.NewReader(log.Bytes()), tc.seq)
		if found != tc.found || e.successfulSince(now) != tc.success {
			t.Errorf("seq %d: %+v found=%t", tc.seq, e, found)
		}
	}
}

func TestReadDeepSeekCanonicalCompressedLog(t *testing.T) {
	const id = "session-native"
	home := t.TempDir()
	path := filepath.Join(home, "sessions", "workspace", id, "session.v4.jsonl.zstd")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	enc, err := zstd.NewWriter(f, zstd.WithEncoderConcurrency(1))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	want := deepSeekTurnEnd{Type: "turn/end", Seq: 20, Time: now.UnixMilli()}
	want.Data.Reason.Kind = "completed"
	if err := json.NewEncoder(enc).Encode(want); err != nil {
		t.Fatal(err)
	}
	if err := enc.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if got, ok := readDeepSeekTurnEnd(home, id, 20); !ok || got != want {
		t.Fatalf("got %v, found=%t", got, ok)
	}
}
