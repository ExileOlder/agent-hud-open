package main

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ExileOlder/agent-hud-open/internal/codexnotice"
)

func TestInboxDismissesOnlySelectedVisibleMessage(t *testing.T) {
	var m mailInbox
	for n := 0; n < 8; n++ {
		m.receive(mailItem{agent: 1, event: codexnotice.Event{ID: strconv.Itoa(n)}}, time.Now(), false)
	}
	m.action(90)
	m.action(94)
	m.action(94)
	y := mailHeader + mailRow + 29
	hit := mailTarget(mailWidth-40, y, mailWidth, len(m.visible()))
	if hit != mailDismissBase+1 {
		t.Fatalf("close hit=%d", hit)
	}
	if _, open := m.action(hit); open {
		t.Fatal("dismissing opened the client")
	}
	if len(m.items) != 7 || !m.open || m.offset != 1 {
		t.Fatalf("bad inbox after removal: count=%d offset=%d open=%t", len(m.items), m.offset, m.open)
	}
	for _, item := range m.items {
		if item.event.ID == "4" {
			t.Fatal("selected message survived")
		}
	}
	body := mailTarget(140, y, mailWidth, len(m.visible()))
	if item, ok := m.action(body); !ok || item.event.ID != "5" {
		t.Fatalf("wrong visible row routed: %+v", item)
	}
	for len(m.items) > 0 {
		m.action(mailDismissBase)
	}
	if m.open || m.offset != 0 {
		t.Fatal("empty inbox remains open")
	}
}

func TestInboxRestoresPendingCardsAndInFlightCardsWithoutReplayingThem(t *testing.T) {
	path := filepath.Join(t.TempDir(), "BalanceHUD", "inbox.json")
	const thread = "cdefabcd-1234-5678-9abc-def012345678"
	m := mailInbox{statePath: path}
	now := time.Now()
	codex := mailItem{agent: 1, account: "private-account", profile: 37, event: codexnotice.Event{
		Thread: thread, ID: "question:42", Question: true,
	}}
	deepseek := mailItem{agent: deepSeekAgent, key: "session/a", permission: true, event: codexnotice.Event{Question: true}}
	m.receive(codex, now, false)
	m.receive(deepseek, now, true)
	disk, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(disk), "private-account") || strings.Contains(string(disk), "profile") {
		t.Fatalf("unexpected inbox on disk: %s, %v", disk, err)
	}

	restored := mailInbox{statePath: path}
	if err := restored.restore(); err != nil {
		t.Fatal(err)
	}
	if len(restored.items) != 2 || len(restored.queue) != 0 || restored.open {
		t.Fatalf("wrong restored inbox: %+v", restored)
	}
	if one, two := restored.items[0], restored.items[1]; one.summary() != "等你确认一下" ||
		two.agent != 1 || clientRoute(0, two.event.Thread) != "codex://threads/"+thread {
		t.Fatalf("restored badge or navigation changed: %+v", restored.items)
	}
	restored.receive(codex, now.Add(time.Second), true)
	restored.receive(deepseek, now.Add(time.Second), true)
	if len(restored.items) != 2 || len(restored.queue) != 0 {
		t.Fatal("startup polling replayed a still-pending saved request")
	}
	restored.action(mailDismissBase)
	again := mailInbox{statePath: path}
	if err := again.restore(); err != nil || len(again.items) != 1 || again.items[0].agent != 1 {
		t.Fatalf("individual dismissal was not saved: %+v, %v", again.items, err)
	}
	again.receive(mailItem{agent: 5}, now, true)
	again.action(92)
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) || len(again.queue) != 0 {
		t.Fatalf("inbox survived clear-all: %+v, %v", again.queue, err)
	}
}

func TestInboxSnapshotKeepsOnlyNewestThirtyTwoNotices(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inbox.json")
	m := mailInbox{statePath: path}
	for i := range mailLimit + 3 {
		m.receive(mailItem{agent: deepSeekAgent, key: strconv.Itoa(i)}, time.Now(), true)
	}
	restored := mailInbox{statePath: path}
	if err := restored.restore(); err != nil {
		t.Fatal(err)
	}
	if len(restored.items) != mailLimit {
		t.Fatalf("stored %d messages", len(restored.items))
	}
	for i, item := range restored.items {
		if item.key != strconv.Itoa(mailLimit+2-i) {
			t.Fatalf("row %d was not restored in arrival order: %s", i, item.key)
		}
	}
}
