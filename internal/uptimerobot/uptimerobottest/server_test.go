package uptimerobottest

import "testing"

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
