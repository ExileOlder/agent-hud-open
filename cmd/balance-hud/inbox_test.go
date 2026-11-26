package main

import (
	"strconv"
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
