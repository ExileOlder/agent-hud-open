package agentactivity

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const antigravityWaitFixture = `{"trajectorySummaries":{"cascade":{"waitingSteps":[
	{"stepIndex":2,"step":{"status":"CORTEX_STEP_STATUS_WAITING","metadata":{"executionId":"q","sourceTrajectoryStepInfo":{"cascadeId":"cascade","trajectoryId":"trajectory","stepIndex":2}},"requestedInteraction":{"askQuestion":{"questions":[{"question":"private question","options":[{"label":"private option"}]}]}}}},
	{"stepIndex":3,"step":{"status":9,"metadata":{"executionId":"p","sourceTrajectoryStepInfo":{"cascadeId":"cascade","trajectoryId":"trajectory","stepIndex":3}},"requestedInteraction":{"permission":{"resource":{"action":"read","target":"private resource"}}}}},
	{"stepIndex":4,"step":{"status":"CORTEX_STEP_STATUS_DONE","metadata":{"sourceTrajectoryStepInfo":{"trajectoryId":"trajectory","stepIndex":4}},"requestedInteraction":{"ask_question":{"questions":[{"question":"done"}]}}}},
	{"stepIndex":5,"step":{"status":9,"metadata":{"sourceTrajectoryStepInfo":{"cascadeId":"wrong","trajectoryId":"trajectory","stepIndex":5}},"requestedInteraction":{"askQuestion":{"questions":[{"question":"wrong conversation"}]}}}},
	{"stepIndex":6,"step":{"status":9,"metadata":{"sourceTrajectoryStepInfo":{"trajectoryId":"trajectory","stepIndex":7}},"requestedInteraction":{"askQuestion":{"questions":[{"question":"wrong step"}]}}}}
]}}}`

func TestAntigravityReadProjectsOnlyWaitingIDs(t *testing.T) {
	got, err := decodeAntigravityWaits(strings.NewReader(antigravityWaitFixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != (AntigravityWait{"", "cascade", "trajectory", "q", 2, false}) ||
		got[1] != (AntigravityWait{"", "cascade", "trajectory", "p", 3, true}) {
		t.Fatalf("unexpected waiting IDs: %v", got)
	}
}

func TestAntigravityTokenOnlyReachesRegisteredLoopbackPeer(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != antigravityReadPath || r.Header.Get("X-Codeium-Csrf-Token") != "fictional-test-token" {
			t.Error("unexpected native request")
		}
		_, _ = w.Write([]byte(antigravityWaitFixture))
	}))
	defer server.Close()
	port := uint16(server.Listener.Addr().(*net.TCPAddr).Port)
	query := func(pid uint32) ([]AntigravityWait, error) {
		d := antigravityDaemon{pid, port, "fictional-test-token"}
		tr := newAntigravityTransport(d, antigravityOwnsPeer)
		defer tr.CloseIdleConnections()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		return readAntigravityWaits(ctx, &http.Client{Transport: tr}, d)
	}
	pid := uint32(os.Getpid())
	if _, err := query(0xffffffff); err == nil || calls.Load() != 0 {
		t.Fatal("sent a credential before verifying the peer")
	}
	if got, err := query(pid); err != nil || len(got) != 2 || calls.Load() != 1 {
		t.Fatalf("registered TLS peer: %v, %v, calls %d", got, err, calls.Load())
	}
}

func TestAntigravityAccountComesFromRunningService(t *testing.T) {
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != antigravityIdentityPath {
			t.Error("account lookup used the wrong request")
		}
		_, _ = w.Write([]byte(`{"userStatus":{"email":" Two@Example.Test "}}`))
	}))
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	d := antigravityDaemon{HTTPSPort: uint16(s.Listener.Addr().(*net.TCPAddr).Port)}
	if account, err := antigravityAccount(ctx, s.Client(), d); err != nil || account != "two@example.test" {
		t.Fatalf("wrong active account: %q (%v)", account, err)
	}
}

func antigravityLongHistory() string {
	var response strings.Builder
	response.WriteString(`{"historyCursor":true,"trajectorySummaries":{`)
	details := strings.Repeat("private archived title", 90)
	for i := range 2200 {
		fmt.Fprintf(&response, `"older-%d":{"title":%q,"waitingSteps":[]},`, i, details)
	}
	response.WriteString(strings.TrimPrefix(antigravityWaitFixture, `{"trajectorySummaries":{`))
	return response.String()
}

func TestAntigravityLargeHistoryStillFindsBothRequestTypes(t *testing.T) {
	history := antigravityLongHistory()
	expected, err := decodeAntigravityWaits(strings.NewReader(antigravityWaitFixture))
	if err != nil || len(expected) != 2 {
		t.Fatalf("unexpected fixture: %v %v", expected, err)
	}
	got, err := decodeAntigravityWaits(strings.NewReader(history))
	if err != nil || len(got) != 2 || got[0] != expected[0] || got[1] != expected[1] {
		t.Fatalf("streaming changed the pending notices: %v %v", got, err)
	}
	if got, err := decodeAntigravityWaits(strings.NewReader(history[:len(history)-1])); err == nil || got != nil {
		t.Fatalf("partial response leaked a notice: %v %v", got, err)
	}
}

func BenchmarkAntigravityLongHistory(b *testing.B) {
	payload := antigravityLongHistory()
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got, err := decodeAntigravityWaits(strings.NewReader(payload))
		if err != nil || len(got) != 2 {
			b.Fatalf("pending notices: %v %v", got, err)
		}
	}
}

func TestAntigravityKeepsEachPendingCascadeAndDropsReplacedSummary(t *testing.T) {
	first := strings.TrimSuffix(strings.TrimPrefix(antigravityWaitFixture, `{"trajectorySummaries":{`), "}}")
	second := strings.ReplaceAll(first, `"cascade"`, `"other"`)
	for _, tc := range []struct {
		entries string
		count   int
	}{
		{first + "," + second, 4},
		{first + "," + second + `,"cascade":{"waitingSteps":[]}`, 2},
	} {
		got, err := decodeAntigravityWaits(strings.NewReader(`{"trajectorySummaries":{` + tc.entries + "}}"))
		if err != nil || len(got) != tc.count {
			t.Fatalf("unexpected pending cascades: %v %v", got, err)
		}
		if tc.count == 2 && (got[0].Cascade != "other" || got[1].Cascade != "other") {
			t.Fatalf("stale cascade replaced the live request: %v", got)
		}
	}
}

func TestAntigravityNewRequestsVerifyAccountWithoutMovingOldRequests(t *testing.T) {
	old := AntigravityWait{Cascade: "same-client", Trajectory: "first-task", Step: 1, Account: "first@example.test"}
	unchanged := old
	unchanged.Account = ""
	fresh := AntigravityWait{Cascade: "same-client", Trajectory: "second-task", Step: 2}
	freshApproval := AntigravityWait{Cascade: "same-client", Trajectory: "third-task", Step: 3, Permission: true}
	waits := []AntigravityWait{unchanged, fresh, freshApproval}
	lookups := 0
	identify := func() string {
		lookups++
		return "second@example.test"
	}
	attributeAntigravityWaits([]AntigravityWait{old}, waits, identify)
	if lookups != 1 || waits[0].Account != old.Account ||
		waits[1].Account != "second@example.test" || waits[2].Account != "second@example.test" {
		t.Fatalf("switched account was assigned to an existing request or not checked for the new ones: %v (%d)", waits, lookups)
	}
	freshUnknown := AntigravityWait{Cascade: "same-client", Trajectory: "unverified-task", Step: 4}
	next := []AntigravityWait{unchanged, fresh, freshUnknown}
	attributeAntigravityWaits(waits, next, func() string { return "" })
	if next[0].Account != old.Account || next[1].Account != "second@example.test" || next[2].Account != "" {
		t.Fatalf("failed identity check leaked the last account onto a new request: %v", next)
	}
	again := []AntigravityWait{freshUnknown, fresh}
	attributeAntigravityWaits(next, again, identify)
	if lookups != 1 || again[0].Account != "" || again[1].Account != "second@example.test" {
		t.Fatalf("unchanged notices were relabeled or repeatedly queried: %v (%d)", again, lookups)
	}
}
