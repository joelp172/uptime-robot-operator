package uptimerobot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPurgeGroupFromBackend_NotFoundIsSuccess(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/monitor-groups/14150" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Monitor group not found","code":"000-004"}`))
	}))
	defer server.Close()

	client := Client{url: server.URL, apiKey: "test-key"}
	if err := client.PurgeGroupFromBackend(context.Background(), "14150"); err != nil {
		t.Fatalf("PurgeGroupFromBackend returned error for 404: %v", err)
	}
}

func TestDeleteMaintenanceWindow_NotFoundIsSuccess(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/maintenance-windows/4242" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Resource you were trying to access is not found.","code":"000-004"}`))
	}))
	defer server.Close()

	client := Client{url: server.URL, apiKey: "test-key"}
	if err := client.DeleteMaintenanceWindow(context.Background(), "4242"); err != nil {
		t.Fatalf("DeleteMaintenanceWindow returned error for 404: %v", err)
	}
}
