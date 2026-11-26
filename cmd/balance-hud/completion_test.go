package main

import (
	"strings"
	"testing"
	"time"
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
		{5, `{"hook_event_name":"Stop","background_tasks":[],"session_crons":[]}`, true},
		{5, `{"hook_event_name":"Stop","background_tasks":[{"description":"private"}]}`, false},
		{5, `{"hook_event_name":"StopFailure"}`, false},
	} {
		if e := parseHook(x.id, strings.NewReader(x.input)); (e.agent != 0 && e.phase == hookComplete) != x.want {
			t.Fatalf("%q: %v", x.input, e)
		}
	}
}

func TestNativeLifecyclePayload(t *testing.T) {
	for _, tc := range []struct {
		agent   int
		payload string
		phase   hookPhase
		valid   bool
	}{
		{5, `{"hook_event_name":"UserPromptSubmit","session_id":"one","prompt":"private"}`, hookWork, true},
		{5, `{"hook_event_name":"PreToolUse","tool_name":"AskUserQuestion","session_id":"one"}`, hookWait, true},
		{5, `{"hook_event_name":"PermissionRequest","session_id":"one"}`, hookPermission, true},
		{5, `{"hook_event_name":"SessionEnd","session_id":"one"}`, hookEnd, true},
		{3, `{"hook_event_name":"pre_llm","session_id":"one"}`, hookWork, true},
		{3, `{"notification_type":"question_asked","session_id":"one"}`, hookWait, true},
		{3, `{"notification_type":"turn_failed","session_id":"one"}`, hookEnd, true},
		{3, `{"hook_event_name":"pre_llm","session_id":"child","parent_session_id":"one"}`, hookWork, false},
		{5, `{"hook_event_name":"PreToolUse","tool_name":"Bash","session_id":"one"}`, hookCommand, true},
	} {
		e := parseHook(tc.agent, strings.NewReader(tc.payload))
		if tc.valid && (e.agent != tc.agent || e.phase != tc.phase || e.session == 0 || e != decodeHook(e.code(), e.session)) || !tc.valid && e.agent != 0 {
			t.Errorf("%s mapped to %+v", tc.payload, e)
		}
	}
}

func TestCodexApprovalKeepsOnlyConversationRoute(t *testing.T) {
	const thread = "cdefabcd-1234-5678-9abc-def012345678"
	event := parseHook(1, strings.NewReader(`{"hook_event_name":"PermissionRequest","session_id":"`+strings.ToUpper(thread)+`","tool_input":{"command":"private","description":"private"}}`))
	want := hookEvent{agent: 1, phase: hookPermission, thread: parseApprovalThread(thread)}
	if event != want {
		t.Fatalf("unexpected projected metadata: %+v", event)
	}
	var m mailInbox
	now := time.Now()
	m.codexApproval(event.thread, now, true)
	if len(m.queue) != 1 || len(m.items) != 0 {
		t.Fatal("approval must start from the dock, then land in the inbox")
	}
	m.advance(now.Add(mailFlight), true)
	if len(m.items) != 1 {
		t.Fatal("approval did not land")
	}
	item := m.items[0]
	if !item.permission || item.summary() != "等你确认一下" || clientRoute(0, item.event.Thread) != "codex://threads/"+thread {
		t.Fatalf("unexpected approval notice: %+v", item)
	}
	if invalid := parseApprovalThread("cdefabcd-1234-5678-9abc-def012345678?extra"); invalid.route() != "" {
		t.Fatal("unrecognized session ID must not change the Codex route")
	}
}
