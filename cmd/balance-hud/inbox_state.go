package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ExileOlder/agent-hud-open/internal/codexnotice"
)

// Store only the badge kind and IDs used for routing/replay suppression.
// Accounts, tool names, excerpts and questions never leave the client.
type savedNotice struct {
	Agent  int    `json:"agent"`
	Tone   int    `json:"tone"`
	Thread string `json:"thread,omitempty"`
	Key    string `json:"key,omitempty"`
}

const maxNoticeKey = 256

func restoredInbox() mailInbox {
	m := mailInbox{profiles: antigravityProfileKeys()}
	if dir, err := os.UserConfigDir(); err == nil {
		m.statePath = filepath.Join(dir, "BalanceHUD", "inbox.json")
		if err := m.restore(); err != nil && !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(os.Stderr, "Could not restore HUD inbox:", err)
		}
	}
	return m
}

func (m *mailInbox) restore() error {
	f, err := os.Open(m.statePath)
	if err != nil {
		return err
	}
	defer f.Close()
	var rows []savedNotice
	if err := json.NewDecoder(io.LimitReader(f, 32<<10)).Decode(&rows); err != nil {
		return err
	}
	for _, row := range rows[:min(len(rows), mailLimit)] {
		if row.Agent <= 0 || row.Agent >= len(agentNames) || row.Tone < 0 || row.Tone > 2 {
			continue
		}
		thread := row.Thread
		if row.Agent != 1 || !codexThreadID.MatchString(thread) {
			thread = ""
		}
		m.items = append(m.items, mailItem{agent: row.Agent, permission: row.Tone == 2, key: boundedNoticeKey(row.Key),
			event: codexnotice.Event{Thread: thread, Question: row.Tone > 0}})
	}
	return nil
}

func boundedNoticeKey(key string) string {
	if len(key) > maxNoticeKey {
		return ""
	}
	return key
}

func (m *mailInbox) saveNotices() {
	if m.statePath == "" {
		return
	}
	if err := m.writeNotices(); err != nil {
		fmt.Fprintln(os.Stderr, "Could not save HUD inbox:", err)
	}
}

func (m *mailInbox) writeNotices() error {
	if len(m.items) == 0 && len(m.queue) == 0 {
		err := os.Remove(m.statePath)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	rows := make([]savedNotice, 0, min(mailLimit, len(m.items)+len(m.queue)))
	add := func(item mailItem) {
		if len(rows) == mailLimit {
			return
		}
		r := savedNotice{Agent: item.agent, Tone: item.tone(), Key: item.noticeKey()}
		if item.agent == 1 && codexThreadID.MatchString(item.event.Thread) {
			r.Thread = item.event.Thread
		}
		rows = append(rows, r)
	}
	// After restart the message is already at the dock, even if its paper
	// plane was still in flight at shutdown.
	for i := len(m.queue) - 1; i >= 0; i-- {
		add(m.queue[i])
	}
	for _, item := range m.items {
		add(item)
	}
	dir := filepath.Dir(m.statePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".inbox-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	err = json.NewEncoder(file).Encode(rows)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), m.statePath)
}

func (item mailItem) noticeKey() string {
	if item.key != "" {
		return boundedNoticeKey(item.key)
	}
	if item.agent == 1 && item.event.ID != "" && codexThreadID.MatchString(item.event.Thread) {
		return boundedNoticeKey(item.event.Thread + "/" + item.event.ID)
	}
	return ""
}
