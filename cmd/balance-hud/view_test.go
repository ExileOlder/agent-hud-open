package main

import (
	"slices"
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

func TestMistralShowsAmountAndPercentOnSameLine(t *testing.T) {
	rows := view([]result{{Provider: "mistral", Accounts: []quota.Account{{Label: "Mistral Vibe", Balance: "137.14 / 150.00 USD", Windows: []quota.Window{{Name: "Subscription", Remaining: 91.426666}}}}}})
	if !strings.Contains(rows[3].Details, "137.14 / 150.00 USD (91.43% remaining)") {
		t.Fatal(rows[3].Details)
	}
}

func TestCompactPeekMatchesOriginalOrder(t *testing.T) {
	items := accountPeek("antigravity", quota.Account{Label: "second@example.test", Windows: []quota.Window{
		{Name: "Gemini Models weekly", Remaining: 93}, {Name: "Gemini Models 5h", Remaining: 86},
		{Name: "Claude and GPT models weekly", Remaining: 100}, {Name: "Claude and GPT models 5h", Remaining: 100},
	}})
	want := []peekItem{{"Account", "second@example.test", 0}, {"Gemini 7d", "93%", 1}, {"Gemini 5h", "86%", 1}, {"3rd-party 7d", "100%", 1}, {"3rd-party 5h", "100%", 1}}
	if !slices.Equal(items, want) {
		t.Fatalf("got %+v", items)
	}
	mistral := accountPeek("mistral", quota.Account{Balance: "137.14 / 150.00 USD", Windows: []quota.Window{{Remaining: 91.426666}}})
	if len(mistral) != 1 || mistral[0] != (peekItem{"Mistral", "$137.14 / $150.00 (91.43%)", 1}) {
		t.Fatalf("got %+v", mistral)
	}
}
