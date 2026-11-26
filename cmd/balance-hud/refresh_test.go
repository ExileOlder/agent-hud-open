package main

import (
	"context"
	"testing"
	"time"

	"github.com/ExileOlder/agent-hud-open/internal/quota"
)

func TestQuotaRefreshDoesNotWaitForSlowProvider(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	release := make(chan struct{})
	updates := refreshQuotas(ctx, []quotaSource{
		{"antigravity", func(ctx context.Context) ([]quota.Account, error) {
			select {
			case <-release:
				return []quota.Account{{Label: "A"}, {Label: "B"}}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}},
		{"codex", func(context.Context) ([]quota.Account, error) {
			return []quota.Account{{Label: "Codex", Windows: []quota.Window{{Remaining: 71}}}}, nil
		}},
	})
	select {
	case r := <-updates:
		if r.Provider != "codex" || len(r.Accounts) != 1 {
			t.Fatalf("unexpected first reading: %+v", r)
		}
	case <-ctx.Done():
		t.Fatal("completed Codex reading was blocked by Antigravity")
	}
	close(release)
	r, ok := <-updates
	if !ok || r.Provider != "antigravity" || len(r.Accounts) != 2 {
		t.Fatalf("slow provider did not contribute both accounts: %+v", r)
	}
	if _, ok := <-updates; ok {
		t.Fatal("finished batch did not close")
	}
}

func TestQuotaUpdatesKeepIndependentRowsAndCLIOrder(t *testing.T) {
	codex := result{Provider: "codex", Accounts: []quota.Account{{Label: "Codex", Windows: []quota.Window{{Remaining: 81}}}}}
	agy := result{Provider: "antigravity", Accounts: []quota.Account{
		{Label: "A", Windows: []quota.Window{{Remaining: 76}}},
		{Label: "B", Windows: []quota.Window{{Remaining: 39}}},
	}}
	r := mergeQuota(mergeQuota(nil, agy), codex)
	previous := view(r)
	r = mergeQuota(r, result{Provider: "codex", Error: "unavailable"})
	rows := updateView(previous, settingsResults(r, placement{}))
	if rows[0].Percent != 81 || !rows[0].Stale || rows[1].Percent != 76 || rows[2].Percent != 39 {
		t.Fatalf("failed Codex refresh changed unrelated rows: %+v", rows)
	}
	updates := make(chan result, 2)
	updates <- agy
	updates <- codex
	close(updates)
	got := collectQuotas(updates)
	if len(got) != 2 || got[0].Provider != "codex" || got[1].Provider != "antigravity" {
		t.Fatalf("unexpected CLI order: %+v", got)
	}
}
