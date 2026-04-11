package uptimerobottest

import (
	"io"
	"net/http"
	"testing"
)

func TestServerStateResetMatchesNewServerState(t *testing.T) {
	initial := NewServerState()

	// Mutate state first to ensure Reset actually restores defaults.
	initial.MarkMonitorDeleted("777")
	initial.deleteIntegration(101)
	initial.nextIntegration = 999

	initial.Reset()
	fresh := NewServerState()

	if initial.nextIntegration != fresh.nextIntegration {
		t.Fatalf("nextIntegration mismatch after reset: got %d want %d", initial.nextIntegration, fresh.nextIntegration)
	}
	if len(initial.integrations) != len(fresh.integrations) {
		t.Fatalf("integrations length mismatch after reset: got %d want %d", len(initial.integrations), len(fresh.integrations))
	}
	for id := range fresh.integrations {
		if _, ok := initial.integrations[id]; !ok {
			t.Fatalf("expected integration id %d to exist after reset", id)
		}
	}
	if initial.IsMonitorDeleted("777") {
		t.Fatal("expected deleted monitor tracking to be cleared after reset")
	}
}

// TestServerState_GlobalHTTPStatus verifies that SetGlobalHTTPStatus causes
// every endpoint to return the forced status code.
func TestServerState_GlobalHTTPStatus(t *testing.T) {
	state := NewServerState()
	srv := NewServerWithState(state)
	defer srv.Close()

	state.SetGlobalHTTPStatus(http.StatusServiceUnavailable)

	resp, err := http.Get(srv.URL + "/monitors") //nolint:noctx
	if err != nil {
		t.Fatalf("GET /monitors: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected %d, got %d", http.StatusServiceUnavailable, resp.StatusCode)
	}

	// After reset, normal behaviour should be restored.
	state.SetGlobalHTTPStatus(0)

	resp2, err := http.Get(srv.URL + "/monitors") //nolint:noctx
	if err != nil {
		t.Fatalf("GET /monitors after reset: %v", err)
	}
	defer func() { _ = resp2.Body.Close() }()

	if resp2.StatusCode != http.StatusOK {
		t.Errorf("expected %d after reset, got %d", http.StatusOK, resp2.StatusCode)
	}
}

// TestServerState_IntermittentFailure verifies that SetIntermittentFailure causes
// the first N requests to return the configured status code, then returns to normal.
func TestServerState_IntermittentFailure(t *testing.T) {
	state := NewServerState()
	srv := NewServerWithState(state)
	defer srv.Close()

	const failCount = 2
	state.SetIntermittentFailure(http.StatusInternalServerError, failCount)

	doGet := func() int {
		resp, err := http.Get(srv.URL + "/monitors") //nolint:noctx
		if err != nil {
			t.Fatalf("GET /monitors: %v", err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		return resp.StatusCode
	}

	for i := range failCount {
		if got := doGet(); got != http.StatusInternalServerError {
			t.Errorf("request %d: expected %d, got %d", i+1, http.StatusInternalServerError, got)
		}
	}

	// After failCount requests the server should return to normal.
	if got := doGet(); got != http.StatusOK {
		t.Errorf("request %d: expected %d after intermittent failures, got %d", failCount+1, http.StatusOK, got)
	}
}

// TestServerState_ResetClearsOverrides verifies that Reset clears global and
// intermittent-failure overrides.
func TestServerState_ResetClearsOverrides(t *testing.T) {
	state := NewServerState()
	state.SetGlobalHTTPStatus(http.StatusBadGateway)
	state.SetIntermittentFailure(http.StatusTooManyRequests, 5)

	state.Reset()

	if state.forceGlobalHTTPStatus != 0 {
		t.Errorf("expected forceGlobalHTTPStatus=0 after Reset, got %d", state.forceGlobalHTTPStatus)
	}
	if state.intermittentFailCount != 0 {
		t.Errorf("expected intermittentFailCount=0 after Reset, got %d", state.intermittentFailCount)
	}
	if state.intermittentFailStatus != 0 {
		t.Errorf("expected intermittentFailStatus=0 after Reset, got %d", state.intermittentFailStatus)
	}
}
