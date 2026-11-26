package agentactivity

import "testing"

func TestAntigravityLifecycleTail(t *testing.T) {
	const prompt = `{"type":"USER_INPUT","source":"USER_EXPLICIT","content":"private"}`
	const tool = `{"type":"PLANNER_RESPONSE","status":"DONE","tool_calls":[{"name":"read_file"}]}`
	for _, test := range []struct {
		steps   string
		working bool
	}{
		{prompt, true},
		{prompt + "\n" + tool + "\n" + `{"type":"TOOL_RESPONSE"}`, true},
		{prompt + "\n" + tool + "\n" + `{"type":"PLANNER_RESPONSE","status":"DONE"}`, false},
		{tool + "\n" + `{"type":"PLANNER_RESPONSE","status":"CANCELED","tool_calls":[{"name":"read_file"}]}`, false},
		{`{"type":"USER_INPUT","source":"SYSTEM"}`, false},
		{prompt + "\n" + `{"type":"PLANNER_RE`, true},
	} {
		if _, got := antigravityPose([]byte(test.steps)); got != test.working {
			t.Errorf("working %t, want %t for %q", got, test.working, test.steps)
		}
	}
}
