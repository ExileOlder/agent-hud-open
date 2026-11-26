package agentactivity

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
)

func TestDeepSeekPendingApprovalFollowsFlushedFrameBoundaries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.v4.jsonl.zstd")
	enc, err := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1))
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close()
	appendBytes := func(data []byte) {
		t.Helper()
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.Write(data)
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			t.Fatalf("append: %v, %v", err, closeErr)
		}
	}
	frame := func(lines ...string) []byte {
		return enc.EncodeAll([]byte(strings.Join(lines, "\n")+"\n"), nil)
	}
	a := deepSeekApprovals{path: path}
	check := func(turn, new, pending int) {
		t.Helper()
		if got := a.watch("", "", turn); got != new || len(a.pending) != pending {
			t.Fatalf("turn %d: got %d new, %d pending; want %d new, %d pending", turn, got, len(a.pending), new, pending)
		}
	}
	appendBytes(frame(
		`{"type":"approval/asked","seq":4,"data":{"id":"earlier"}}`,
		`{"type":"turn/end","seq":6}`,
		`{"type":"turn/start","seq":10}`,
		`{"type":"approval/asked","seq":11,"data":{"id":"auto"}}`,
		`{"type":"approval/decided","seq":12,"data":{"id":"auto","outcome":"rejected"}}`,
	))
	check(10, 0, 0)
	offset := a.offset
	partial := frame(`{"type":"approval/asked","seq":13,"data":{"id":"ask-a","reason":"private"}}`)
	appendBytes(partial[:len(partial)-1])
	check(10, 0, 0)
	if a.offset != offset {
		t.Fatal("a partial frame advanced the cursor")
	}
	appendBytes(partial[len(partial)-1:])
	check(10, 1, 1)
	check(10, 0, 1)
	appendBytes(frame(`{"type":"approval/asked","seq":14,"data":{"id":"ask-b"}}`))
	check(10, 1, 2)
	appendBytes(frame(`{"type":"approval/decided","seq":15,"data":{"id":"ask-a"}}`))
	check(10, 0, 1)
	appendBytes(frame(`{"type":"turn/end","seq":16}`))
	check(10, 0, 0)
	appendBytes(frame(`{"type":"turn/start","seq":20}`, `{"type":"approval/asked","seq":21,"data":{"id":"next"}}`))
	check(10, 0, 0) // stale checkpoint must not report a request from a different turn
	check(20, 1, 1)
}

func TestDeepSeekWatcherReceivesAndClearsApproval(t *testing.T) {
	home := t.TempDir()
	projection := filepath.Join(home, deepSeekProjectionDir, "example.json")
	log := filepath.Join(home, "sessions", "workspace", "example", "session.v4.jsonl.zstd")
	for _, f := range []string{projection, log} {
		if err := os.MkdirAll(filepath.Dir(f), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(projection, []byte(`{"record":{"rows":{"turnBoundary":{"seq":10,"val":{"openTurnStartSeq":10}},"approvalState":{"val":{"policy":"ask"}}}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	enc, err := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1))
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close()
	if err := os.WriteFile(log, enc.EncodeAll([]byte("{\"type\":\"approval/asked\",\"seq\":11,\"data\":{\"id\":\"approval\",\"reason\":\"private\"}}\n"), nil), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	updates := WatchDeepSeek(ctx, home)
	first := <-updates
	if first.Approvals != 1 || !first.ApprovalPending || first.Running || first.Pending {
		t.Fatalf("waiting approval reported as %+v", first)
	}
	f, err := os.OpenFile(log, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Write(enc.EncodeAll([]byte("{\"type\":\"approval/decided\",\"seq\":12,\"data\":{\"id\":\"approval\"}}\n"), nil))
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("append decision: %v %v", err, closeErr)
	}
	second, ok := <-updates
	if !ok || second.Approvals != 0 || second.ApprovalPending || !second.Running {
		t.Fatalf("resolved approval reported as %+v", second)
	}
}
