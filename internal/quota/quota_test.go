package quota

import (
	"strings"
	"testing"
)

func TestVibeBudget(t *testing.T) {
	a, e := parseMistral(strings.NewReader(`{"vibe_budget":{"initial_budget":150,"usage_percentage":7.84,"currency":"USD"}}`))
	if e != nil || a.Balance != "138.24 / 150.00 USD" || a.Windows[0].Remaining != 92.16 {
		t.Fatalf("%+v %v", a, e)
	}
	if _, e = parseMistral(strings.NewReader(`{"vibe_budget":{"initial_budget":150,"currency":"USD"}}`)); e == nil {
		t.Fatal("missing usage accepted")
	}
}
func TestCodexSubscription(t *testing.T) {
	a, e := parseCodex([]byte(`{"rateLimits":{"primary":{"usedPercent":25,"windowDurationMins":300},"secondary":{"usedPercent":67,"windowDurationMins":10080}}}`))
	if e != nil || len(a.Windows) != 2 || a.Windows[0].Remaining != 75 || a.Windows[1].Name != "7d" {
		t.Fatalf("%+v %v", a, e)
	}
}
func TestSeparateAntigravityAccounts(t *testing.T) {
	a, e := parseAntigravity(strings.NewReader(`{"data":{"accounts":[{"email":"a@example.test","groups":[{"name":"Gemini","buckets":[{"window":"5h","remaining_fraction":0.7}]}]},{"email":"b@example.test","usage_error":"offline"},{"email":"A@example.test"}]}}`))
	if e != nil || len(a) != 2 || len(a[0].Windows) != 1 || a[0].Windows[0].Remaining != 70 || len(a[1].Windows) != 0 {
		t.Fatalf("%+v %v", a, e)
	}
}
