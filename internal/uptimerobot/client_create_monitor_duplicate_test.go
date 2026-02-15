package uptimerobot

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	uptimerobotv1 "github.com/joelp172/uptime-robot-operator/api/v1alpha1"
	"github.com/joelp172/uptime-robot-operator/internal/uptimerobot/urtypes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCreateMonitor_AdoptsDuplicateByUniqueURLAndType(t *testing.T) {
	t.Parallel()

	const monitorsPath = "/monitors"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == monitorsPath:
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"message":"Duplicate monitor already exists.","code":"009-011"}`))
		case r.Method == http.MethodGet && r.URL.Path == monitorsPath:
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": 42, "friendlyName": "Legacy monitor", "url": "https://api.example.com/health", "type": "HTTP"},
					{"id": 88, "friendlyName": "Other", "url": "https://api.example.com/health", "type": "KEYWORD"},
				},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := Client{url: server.URL, apiKey: "test-key"}
	interval := metav1.Duration{Duration: 300000000000}
	timeout := metav1.Duration{Duration: 30000000000}
	gracePeriod := metav1.Duration{Duration: 60000000000}
	monitor := uptimerobotv1.MonitorValues{
		Name:        "API Health Check",
		URL:         "https://api.example.com/health",
		Type:        urtypes.TypeHTTPS,
		Interval:    &interval,
		Timeout:     &timeout,
		GracePeriod: &gracePeriod,
	}

	result, err := client.CreateMonitor(context.Background(), monitor, nil)
	if err != nil {
		t.Fatalf("expected duplicate adoption to succeed, got error: %v", err)
	}
	if result.ID != "42" {
		t.Fatalf("expected adopted monitor ID 42, got %q", result.ID)
	}
}

func TestCreateMonitor_DuplicateReturnsErrorWhenCandidateAmbiguous(t *testing.T) {
	t.Parallel()

	const monitorsPath = "/monitors"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == monitorsPath:
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"message":"Duplicate monitor already exists.","code":"009-011"}`))
		case r.Method == http.MethodGet && r.URL.Path == monitorsPath:
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": 42, "friendlyName": "One", "url": "https://api.example.com/health", "type": "HTTP"},
					{"id": 43, "friendlyName": "Two", "url": "https://api.example.com/health", "type": "HTTP"},
				},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := Client{url: server.URL, apiKey: "test-key"}
	interval := metav1.Duration{Duration: 300000000000}
	timeout := metav1.Duration{Duration: 30000000000}
	gracePeriod := metav1.Duration{Duration: 60000000000}
	monitor := uptimerobotv1.MonitorValues{
		Name:        "API Health Check",
		URL:         "https://api.example.com/health",
		Type:        urtypes.TypeHTTPS,
		Interval:    &interval,
		Timeout:     &timeout,
		GracePeriod: &gracePeriod,
	}

	_, err := client.CreateMonitor(context.Background(), monitor, nil)
	if err == nil {
		t.Fatal("expected error for ambiguous duplicate candidates, got nil")
	}
	if !errors.Is(err, ErrStatus) {
		t.Fatalf("expected ErrStatus, got %v", err)
	}
	if !strings.Contains(err.Error(), "409 Conflict") {
		t.Fatalf("expected 409 conflict error, got %v", err)
	}
}
