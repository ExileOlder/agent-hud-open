package main

import (
	"strings"
	"testing"
)

func TestCompletionEvents(t *testing.T) {
	for _, x := range []struct {
		id    int
		input string
		want  bool
	}{
		{1, `{"hook_event_name":"Stop","last_assistant_message":"private"}`, true},
		{1, `{"hook_event_name":"Interrupt"}`, false},
		{2, `{"terminationReason":"NO_TOOL_CALL","fullyIdle":true}`, true},
		{2, `{"terminationReason":"model_stop","fullyIdle":true}`, true},
		{2, `{"terminationReason":"model_stop","fullyIdle":false}`, false},
		{2, `{"terminationReason":"model_stop","fullyIdle":true,"error":"error"}`, false},
		{3, `{"notification_type":"turn_completed"}`, true},
		{3, `{"notification_type":"turn_failed"}`, false},
	} {
		if got := completed(x.id, strings.NewReader(x.input)); got != x.want {
			t.Fatalf("%q: %v", x.input, got)
		}
	}
}
