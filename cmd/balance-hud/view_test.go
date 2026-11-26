package main

import (
	"strings"
	"testing"

	"github.com/ExileOlder/agent-hud-open/internal/quota"
)

func TestViewKeepsAccountsSeparateAndExposesTightestQuota(t *testing.T) {
	rows := view([]result{{Provider: "antigravity", Accounts: []quota.Account{
		{Label: "one@example.test", Windows: []quota.Window{{Name: "5h", Remaining: 60}, {Name: "7d", Remaining: 12}}},
		{Label: "two@example.test", Windows: []quota.Window{{Name: "5h", Remaining: 84}}},
	}}, {Provider: "mistral", Error: "unavailable"}})
	if rows[1].Number != "12%" || rows[2].Number != "84%" || rows[3].Number != "--" ||
		!strings.Contains(rows[1].Details, "one@example.test") || !strings.Contains(rows[2].Details, "two@example.test") {
		t.Fatalf("incorrect rows: %+v", rows)
	}
}

func TestLastReadingOnFailureAndAccountReplacement(t *testing.T) {
	old := view([]result{{Provider: "antigravity", Accounts: []quota.Account{
		{Label: "one@example.test", Windows: []quota.Window{{Name: "5h", Remaining: 60}}},
		{Label: "two@example.test", Windows: []quota.Window{{Name: "5h", Remaining: 84}}},
	}}})
	partial := updateView(old, []result{{Provider: "antigravity", Accounts: []quota.Account{
		{Label: "one@example.test"},
		{Label: "three@example.test"},
	}}})
	if !partial[1].Stale || partial[1].Number != "60%*" || partial[2].Percent != -1 || partial[2].Stale {
		t.Fatalf("failed reading or identity was mishandled: %+v", partial)
	}
	failed := updateView(partial, []result{{Provider: "antigravity", Error: "offline"}})
	if strings.Count(failed[1].Details, "Last reading") != 1 || !failed[1].Stale {
		t.Fatalf("bad stale detail: %+v", failed[1])
	}
}
