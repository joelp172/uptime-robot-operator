package uptimerobot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateSlackIntegration(t *testing.T) {
	t.Parallel()

	var gotReq CreateSlackIntegrationRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected method %s, got %s", http.MethodPost, r.Method)
		}
		if r.URL.Path != "/integrations" {
			t.Fatalf("expected path /integrations, got %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		_, _ = w.Write([]byte(`{
			"id": 12345,
			"friendlyName": "Slack from unit test",
			"enableNotificationsFor": "Down",
			"type": "Slack",
			"status": "Active",
			"sslExpirationReminder": false,
			"value": "https://hooks.slack.com/services/T000/B000/XXX",
			"customValue": "custom message",
			"customValue2": "",
			"customValue3": "",
			"customValue4": ""
		}`))
	}))
	defer server.Close()

	client := Client{url: server.URL, apiKey: "test-key"}
	resp, err := client.CreateSlackIntegration(context.Background(), SlackIntegrationData{
		FriendlyName:           "Slack from unit test",
		EnableNotificationsFor: "Down",
		SSLExpirationReminder:  false,
		WebhookURL:             "https://hooks.slack.com/services/T000/B000/XXX",
		CustomValue:            "custom message",
	})
	if err != nil {
		t.Fatalf("CreateSlackIntegration returned error: %v", err)
	}

	if gotReq.Type != "Slack" {
		t.Fatalf("expected request type Slack, got %s", gotReq.Type)
	}
	if gotReq.Data.FriendlyName != "Slack from unit test" {
		t.Fatalf("unexpected friendlyName: %s", gotReq.Data.FriendlyName)
	}
	if gotReq.Data.EnableNotificationsFor != "Down" {
		t.Fatalf("unexpected enableNotificationsFor: %s", gotReq.Data.EnableNotificationsFor)
	}
	if gotReq.Data.WebhookURL != "https://hooks.slack.com/services/T000/B000/XXX" {
		t.Fatalf("unexpected webhookURL: %s", gotReq.Data.WebhookURL)
	}
	if gotReq.Data.CustomValue != "custom message" {
		t.Fatalf("unexpected customValue: %s", gotReq.Data.CustomValue)
	}

	if resp.ID != 12345 {
		t.Fatalf("expected id 12345, got %d", resp.ID)
	}
	if resp.Type == nil || *resp.Type != "Slack" {
		t.Fatalf("expected response type Slack, got %#v", resp.Type)
	}
}

func TestListAndDeleteIntegrations(t *testing.T) {
	t.Parallel()

	deleteCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/integrations":
			_, _ = w.Write([]byte(`{
				"nextLink": null,
				"data": [
					{
						"id": 10,
						"friendlyName": "Existing Slack",
						"enableNotificationsFor": "UpAndDown",
						"type": "Slack",
						"status": "Active",
						"sslExpirationReminder": false,
						"value": "https://hooks.slack.com/services/T000/B000/YYY",
						"customValue": "hello",
						"customValue2": "",
						"customValue3": "",
						"customValue4": ""
					}
				]
			}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/integrations/10":
			deleteCalled = true
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := Client{url: server.URL, apiKey: "test-key"}

	integrations, err := client.ListIntegrations(context.Background())
	if err != nil {
		t.Fatalf("ListIntegrations returned error: %v", err)
	}
	if len(integrations) != 1 {
		t.Fatalf("expected 1 integration, got %d", len(integrations))
	}
	if integrations[0].ID != 10 {
		t.Fatalf("expected id 10, got %d", integrations[0].ID)
	}

	if err := client.DeleteIntegration(context.Background(), 10); err != nil {
		t.Fatalf("DeleteIntegration returned error: %v", err)
	}
	if !deleteCalled {
		t.Fatal("expected delete endpoint to be called")
	}
}
