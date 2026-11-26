package agentactivity

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
)

func TestDeepSeekCompactionMatchesIdentityAndExpires(t *testing.T) {
	now := time.Now()
	rows := fmt.Sprintf(`{"type":"compaction/start","seq":7,"time":%d,"data":{"compactionId":"one","turn":null}}
{"type":"compaction/end","seq":8,"data":{"compactionId":"unrelated","turn":null}}
{"type":"approval/asked","seq":9,"data":{"id":"not-in-a-turn"}}
`, now.UnixMilli())
	state, ok := deepSeekLifecycleTail(strings.NewReader(rows), deepSeekLogState{}, 0)
	a := deepSeekLifecycle{state: state}
	if !ok || len(state.pending) != 0 || !a.compacting(now) || a.compacting(now.Add(deepSeekCompactLifetime)) {
		t.Fatalf("manual compaction was folded incorrectly: %+v, pending=%v", state.compact, state.pending)
	}
}

func TestDeepSeekWatcherRendersStandaloneCompactionWithoutCompletingATask(t *testing.T) {
	home := t.TempDir()
	projection := filepath.Join(home, deepSeekProjectionDir, "example.json")
	log := filepath.Join(home, "sessions", "workspace", "example", "session.v4.jsonl.zstd")
	for _, f := range []string{projection, log} {
		if err := os.MkdirAll(filepath.Dir(f), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(projection, []byte(`{"record":{"rows":{"turnBoundary":{"seq":0,"val":{"openTurnStartSeq":null}},"approvalState":{"val":{"policy":"never"}}}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	enc, err := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1))
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close()
	start := fmt.Sprintf(`{"type":"compaction/start","seq":7,"time":%d,"data":{"compactionId":"one","turn":null}}`+"\n", time.Now().UnixMilli())
	if err := os.WriteFile(log, enc.EncodeAll([]byte(start), nil), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	ch := WatchDeepSeek(ctx, home)
	got := <-ch
	if !got.Compacting || got.Running || got.Completed != 0 || len(got.Approvals) != 0 {
		t.Fatalf("unexpected manual compaction state: %+v", got)
	}
	file, err := os.OpenFile(log, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.Write(enc.EncodeAll([]byte(`{"type":"compaction/end","seq":8,"data":{"compactionId":"one","turn":null}}`+"\n"), nil))
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("append: %v %v", err, closeErr)
	}
	got, ok := <-ch
	if !ok || got.Compacting || got.Running || got.Completed != 0 || len(got.Approvals) != 0 {
		t.Fatalf("manual compaction end invented a task or left the mark busy: %+v", got)
	}
}
