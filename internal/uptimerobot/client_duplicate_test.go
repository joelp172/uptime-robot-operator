package uptimerobot

import (
	"testing"

	uptimerobotv1 "github.com/joelp172/uptime-robot-operator/api/v1alpha1"
)

func TestSelectDuplicateMonitorCandidate(t *testing.T) {
	t.Run("matches by name and url", func(t *testing.T) {
		desired := uptimerobotv1.MonitorValues{
			Name: "My Monitor",
			URL:  "https://example.com",
		}
		existing := []MonitorResponse{
			{ID: 101, FriendlyName: "Other", URL: "https://example.com"},
			{ID: 202, FriendlyName: "My Monitor", URL: "https://example.com/"},
		}

		match, ok := selectDuplicateMonitorCandidate(existing, desired)
		if !ok {
			t.Fatalf("expected match, got no match")
		}
		if match.ID != 202 {
			t.Fatalf("expected ID 202, got %d", match.ID)
		}
	})

	t.Run("does not match by url alone", func(t *testing.T) {
		desired := uptimerobotv1.MonitorValues{
			URL: "https://example.com",
		}
		existing := []MonitorResponse{
			{ID: 101, FriendlyName: "Existing", URL: "https://example.com"},
		}

		if _, ok := selectDuplicateMonitorCandidate(existing, desired); ok {
			t.Fatalf("expected no match for URL-only candidate")
		}
	})

	t.Run("does not match when name matches but url mismatches", func(t *testing.T) {
		desired := uptimerobotv1.MonitorValues{
			Name: "My Monitor",
			URL:  "https://example.com",
		}
		existing := []MonitorResponse{
			{ID: 101, FriendlyName: "My Monitor", URL: "https://example.net"},
		}

		if _, ok := selectDuplicateMonitorCandidate(existing, desired); ok {
			t.Fatalf("expected no match when URL mismatches")
		}
	})

	t.Run("matches by unique name when url not provided", func(t *testing.T) {
		desired := uptimerobotv1.MonitorValues{
			Name: "My Monitor",
		}
		existing := []MonitorResponse{
			{ID: 101, FriendlyName: "Other", URL: "https://a.example"},
			{ID: 202, FriendlyName: "My Monitor", URL: "https://b.example"},
		}

		match, ok := selectDuplicateMonitorCandidate(existing, desired)
		if !ok {
			t.Fatalf("expected unique name match, got no match")
		}
		if match.ID != 202 {
			t.Fatalf("expected ID 202, got %d", match.ID)
		}
	})

	t.Run("rejects ambiguous name match", func(t *testing.T) {
		desired := uptimerobotv1.MonitorValues{
			Name: "Duplicate Name",
		}
		existing := []MonitorResponse{
			{ID: 101, FriendlyName: "Duplicate Name", URL: "https://a.example"},
			{ID: 202, FriendlyName: "Duplicate Name", URL: "https://b.example"},
		}

		if _, ok := selectDuplicateMonitorCandidate(existing, desired); ok {
			t.Fatalf("expected no match for ambiguous duplicates")
		}
	})
}
