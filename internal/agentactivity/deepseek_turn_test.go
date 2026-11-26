package agentactivity

import (
	"bytes"
	"context"
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
	if got, ok := readDeepSeekTurnEnd(path, 20); !ok || got != want {
		t.Fatalf("got %v, found=%t", got, ok)
	}
}

func TestDeepSeekCompletionUsesIncrementalTerminalWithoutReplay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.zstd")
	enc, err := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1))
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close()
	initial := enc.EncodeAll([]byte("{\"type\":\"turn/start\",\"seq\":10}\n"), nil)
	if err := os.WriteFile(path, initial, 0600); err != nil {
		t.Fatal(err)
	}
	log := deepSeekLifecycle{path: path}
	log.watch("", "", 10, 0)
	if log.offset != int64(len(initial)) {
		t.Fatal("did not establish the open-turn cursor")
	}
	// Leave the previously read region unreadable. Only the appended frame
	// can supply this completion and the following turn's request.
	now := time.Now()
	tail := enc.EncodeAll([]byte(fmt.Sprintf(
		"{\"type\":\"turn/end\",\"seq\":16,\"time\":%d,\"data\":{\"reason\":{\"kind\":\"completed\"}}}\n"+
			"{\"type\":\"turn/start\",\"seq\":20}\n"+
			"{\"type\":\"approval/asked\",\"seq\":21,\"data\":{\"id\":\"next\"}}\n", now.UnixMilli())), nil)
	f, err := os.OpenFile(path, os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt(append(make([]byte, len(initial)), tail...), 0); err != nil {
		t.Fatal(err)
	}
	f.Close()
	ids, end, found := log.watch("", "", noDeepSeekTurn, 16)
	if len(ids) != 0 || !found || !end.successfulSince(now) {
		t.Fatalf("fresh terminal was lost or future request surfaced: %v %+v %t", ids, end, found)
	}
	ids, _, _ = log.watch("", "", 20, 0)
	if len(ids) != 1 || ids[0] != "next" || log.closed {
		t.Fatalf("next turn lost its pending request: %v", ids)
	}
}

func TestDeepSeekTerminalAfterProjectionIsDeliveredOnce(t *testing.T) {
	home := t.TempDir()
	const session = "late-terminal"
	projection := filepath.Join(home, deepSeekProjectionDir, session+".json")
	log := filepath.Join(home, "sessions", "workspace", session, "session.v4.jsonl.zstd")
	for _, name := range []string{projection, log} {
		if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	enc, err := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1))
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close()
	writeProjection := func(seq int, turning string) {
		t.Helper()
		s := fmt.Sprintf(`{"record":{"rows":{"turnBoundary":{"seq":%d,"val":{"openTurnStartSeq":%s}}}}}`, seq, turning)
		if err := os.WriteFile(projection, []byte(s), 0600); err != nil {
			t.Fatal(err)
		}
	}
	open := enc.EncodeAll([]byte("{\"type\":\"turn/start\",\"seq\":10}\n"), nil)
	if err := os.WriteFile(log, open, 0600); err != nil {
		t.Fatal(err)
	}
	writeProjection(10, "10")
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	ch := WatchDeepSeek(ctx, home)
	if got := <-ch; !got.Running {
		t.Fatalf("open turn: %+v", got)
	}
	writeProjection(20, "null")
	if got := <-ch; got.Running || got.Completed != 0 {
		t.Fatalf("projection close fabricated a completion: %+v", got)
	}
	end := fmt.Sprintf(`{"type":"turn/end","seq":20,"time":%d,"data":{"reason":{"kind":"completed"}}}`+"\n", time.Now().UnixMilli())
	if err := os.WriteFile(log, enc.EncodeAll([]byte(end), open), 0600); err != nil {
		t.Fatal(err)
	}
	if got := <-ch; got.Running || got.Completed != 1 {
		t.Fatalf("late terminal: %+v", got)
	}
	select {
	case got := <-ch:
		t.Fatalf("completion was sent again: %+v", got)
	case <-time.After(1700 * time.Millisecond):
	}
}

func TestDeepSeekLaggingProjectionKeepsItsExactOutcome(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.zstd")
	enc, err := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1))
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close()
	lines := `{"type":"turn/end","seq":16,"data":{"reason":{"kind":"aborted"}}}
{"type":"turn/start","seq":20}
{"type":"turn/end","seq":26,"data":{"reason":{"kind":"completed"}}}
`
	if err := os.WriteFile(path, enc.EncodeAll([]byte(lines), nil), 0600); err != nil {
		t.Fatal(err)
	}
	log := deepSeekLifecycle{path: path}
	log.watch("", "", noDeepSeekTurn, 0)
	offset := log.offset
	_, end, ok := log.watch("", "", noDeepSeekTurn, 16)
	if !ok || end.Seq != 16 || end.Data.Reason.Kind != "aborted" || log.offset != offset {
		t.Fatalf("newer completion masked the aborted outcome: %+v, found=%t", end, ok)
	}
}
