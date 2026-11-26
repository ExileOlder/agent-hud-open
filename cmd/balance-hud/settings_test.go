package main

import (
	"testing"

	"github.com/ExileOlder/agent-hud-open/internal/quota"
)

func TestPinnedAccountNeverBorrowsDifferentQuota(t *testing.T) {
	raw := []result{{Provider: "antigravity", Accounts: []quota.Account{
		{Label: "one@example.test", Windows: []quota.Window{{Name: "7d", Remaining: 61}}},
		{Label: "two@example.test", Windows: []quota.Window{{Name: "7d", Remaining: 89}}},
	}}}
	p := placement{Accounts: [2]string{"two@example.test", "missing@example.test"}}
	rows := view(settingsResults(raw, p))
	if rows[1].Percent != 89 || rows[2].Percent != -1 || rows[2].Identity != "missing@example.test" {
		t.Fatalf("slots %v %v", rows[1], rows[2])
	}
	p = placement{}
	panel := panelSettings{page: 1, raw: raw}
	if !panel.selectAccount(&p, 0, view(raw)) {
		t.Fatal("selection failed")
	}
	rows = view(settingsResults(raw, p))
	if rows[1].Percent != 89 || rows[2].Percent != 61 {
		t.Fatal(rows)
	}
}
