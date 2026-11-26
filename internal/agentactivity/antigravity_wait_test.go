package agentactivity

import (
	"context"
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
