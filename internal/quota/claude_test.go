package quota

import "testing"

func TestClaudeQuota(t *testing.T) {
	a, err := parseClaude([]byte(`{"rate_limits_available":true,"rate_limits":{"five_hour":{"utilization":21.5},"seven_day":{"utilization":40},"seven_day_opus":{"utilization":0}}}`))
	if err != nil || len(a.Windows) != 3 || a.Windows[0].Remaining != 78.5 || a.Windows[1].Remaining != 60 || a.Windows[2].Remaining != 100 {
		t.Fatalf("%+v %v", a, err)
	}
	if _, err = parseClaude([]byte(`{"rate_limits_available":false}`)); err == nil {
		t.Fatal("signed-out result treated as available")
	}
}
