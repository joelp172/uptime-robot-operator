package uptimerobot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

const integrationsPath = "/integrations"

func TestCreateSlackIntegration(t *testing.T) {
	t.Parallel()

	var gotReq CreateSlackIntegrationRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected method %s, got %s", http.MethodPost, r.Method)
		}
		if r.URL.Path != integrationsPath {
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

	if gotReq.Type != slackIntegrationType {
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
	if resp.Type == nil || *resp.Type != slackIntegrationType {
		t.Fatalf("expected response type Slack, got %#v", resp.Type)
	}
}

func TestCreateSlackIntegration_AdoptsExistingOnDuplicateWebhookConflict(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == integrationsPath:
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"message":"This integration already exists.","code":"021-001"}`))
		case r.Method == http.MethodGet && r.URL.Path == integrationsPath:
			_, _ = w.Write([]byte(`{
				"nextLink": null,
				"data": [
					{
						"id": 77,
						"friendlyName": "Shared Slack",
						"enableNotificationsFor": "Down",
						"type": "Slack",
						"status": "Active",
						"sslExpirationReminder": false,
						"value": "https://hooks.slack.com/services/T000/B000/SHARED",
						"customValue": "shared",
						"customValue2": "",
						"customValue3": "",
						"customValue4": ""
					}
				]
			}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := Client{url: server.URL, apiKey: "test-key"}
	resp, err := client.CreateSlackIntegration(context.Background(), SlackIntegrationData{
		FriendlyName:           "Shared Slack",
		EnableNotificationsFor: "Down",
		SSLExpirationReminder:  false,
		WebhookURL:             "https://hooks.slack.com/services/T000/B000/SHARED",
		CustomValue:            "shared",
	})
	if err != nil {
		t.Fatalf("expected duplicate adoption to succeed, got error: %v", err)
	}
	if resp.ID != 77 {
		t.Fatalf("expected adopted integration id 77, got %d", resp.ID)
	}
	if resp.Type == nil || *resp.Type != slackIntegrationType {
		t.Fatalf("expected response type Slack, got %#v", resp.Type)
	}
}

// slackIntegrationTestServer returns an httptest.Server that always responds
// with a 409/021-001 on POST /integrations and serves the given list payload
// on GET /integrations. It records the GET request count so callers can
// assert whether the duplicate-adoption branch attempted a list.
func slackIntegrationTestServer(t *testing.T, listBody string, listStatus int) (*httptest.Server, *int32) {
	t.Helper()
	var listCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == integrationsPath:
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"message":"This integration already exists.","code":"021-001"}`))
		case r.Method == http.MethodGet && r.URL.Path == integrationsPath:
			atomic.AddInt32(&listCount, 1)
			if listStatus != 0 {
				w.WriteHeader(listStatus)
				return
			}
			_, _ = w.Write([]byte(listBody))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	return srv, &listCount
}

func TestCreateSlackIntegration_AdoptionChoosesByFriendlyNameAndWebhook(t *testing.T) {
	t.Parallel()

	listBody := `{
		"nextLink": null,
		"data": [
			{"id": 10, "friendlyName": "Cluster A", "type": "Slack", "value": "https://hooks.slack.com/services/SHARED", "sslExpirationReminder": false, "customValue": ""},
			{"id": 11, "friendlyName": "Cluster B", "type": "Slack", "value": "https://hooks.slack.com/services/SHARED", "sslExpirationReminder": false, "customValue": ""}
		]
	}`
	srv, _ := slackIntegrationTestServer(t, listBody, 0)
	defer srv.Close()

	client := Client{url: srv.URL, apiKey: "test-key"}
	resp, err := client.CreateSlackIntegration(context.Background(), SlackIntegrationData{
		FriendlyName: "Cluster B",
		WebhookURL:   "https://hooks.slack.com/services/SHARED",
	})
	if err != nil {
		t.Fatalf("expected adoption to succeed, got error: %v", err)
	}
	if resp.ID != 11 {
		t.Fatalf("expected adopted integration id 11 (matching FriendlyName), got %d", resp.ID)
	}
}

func TestCreateSlackIntegration_NoAdoptionOnAmbiguousCandidates(t *testing.T) {
	t.Parallel()

	listBody := `{
		"nextLink": null,
		"data": [
			{"id": 10, "friendlyName": "Cluster A", "type": "Slack", "value": "https://hooks.slack.com/services/SHARED", "sslExpirationReminder": false, "customValue": ""},
			{"id": 11, "friendlyName": "Cluster A", "type": "Slack", "value": "https://hooks.slack.com/services/SHARED", "sslExpirationReminder": false, "customValue": ""}
		]
	}`
	srv, _ := slackIntegrationTestServer(t, listBody, 0)
	defer srv.Close()

	client := Client{url: srv.URL, apiKey: "test-key"}
	_, err := client.CreateSlackIntegration(context.Background(), SlackIntegrationData{
		FriendlyName: "Cluster A",
		WebhookURL:   "https://hooks.slack.com/services/SHARED",
	})
	if err == nil {
		t.Fatalf("expected original 409 to be surfaced when candidates are ambiguous")
	}
}

func TestCreateSlackIntegration_NoAdoptionOnZeroCandidates(t *testing.T) {
	t.Parallel()

	listBody := `{
		"nextLink": null,
		"data": [
			{"id": 10, "friendlyName": "Other", "type": "Slack", "value": "https://hooks.slack.com/services/OTHER", "sslExpirationReminder": false, "customValue": ""}
		]
	}`
	srv, _ := slackIntegrationTestServer(t, listBody, 0)
	defer srv.Close()

	client := Client{url: srv.URL, apiKey: "test-key"}
	_, err := client.CreateSlackIntegration(context.Background(), SlackIntegrationData{
		FriendlyName: "Cluster A",
		WebhookURL:   "https://hooks.slack.com/services/SHARED",
	})
	if err == nil {
		t.Fatalf("expected original 409 to be surfaced when no candidate matches")
	}
}

func TestCreateSlackIntegration_RefusesAdoptionWhenFriendlyNameEmpty(t *testing.T) {
	t.Parallel()

	// A unique webhook match must not be enough on its own — without a
	// FriendlyName we cannot tell whether this integration belongs to us or
	// another cluster, so adoption must be refused and the original 409 surfaced.
	listBody := `{
		"nextLink": null,
		"data": [
			{"id": 42, "friendlyName": "Existing", "type": "Slack", "value": "https://hooks.slack.com/services/SHARED", "sslExpirationReminder": false, "customValue": ""}
		]
	}`
	srv, _ := slackIntegrationTestServer(t, listBody, 0)
	defer srv.Close()

	client := Client{url: srv.URL, apiKey: "test-key"}
	_, err := client.CreateSlackIntegration(context.Background(), SlackIntegrationData{
		FriendlyName: "",
		WebhookURL:   "https://hooks.slack.com/services/SHARED",
	})
	if err == nil {
		t.Fatalf("expected original 409 to be surfaced when FriendlyName is empty")
	}
}

func TestCreateSlackIntegration_SkipsNonSlackWithSameWebhook(t *testing.T) {
	t.Parallel()

	// A non-Slack integration using the same value must NOT be adopted as Slack.
	listBody := `{
		"nextLink": null,
		"data": [
			{"id": 10, "friendlyName": "Webhook Target", "type": "Webhook", "value": "https://hooks.slack.com/services/SHARED", "sslExpirationReminder": false, "customValue": ""}
		]
	}`
	srv, _ := slackIntegrationTestServer(t, listBody, 0)
	defer srv.Close()

	client := Client{url: srv.URL, apiKey: "test-key"}
	_, err := client.CreateSlackIntegration(context.Background(), SlackIntegrationData{
		FriendlyName: "Webhook Target",
		WebhookURL:   "https://hooks.slack.com/services/SHARED",
	})
	if err == nil {
		t.Fatalf("expected original 409 to be surfaced; non-Slack integration must not be adopted")
	}
}

func TestCreateSlackIntegration_NoListCallOnUnrecognized409Code(t *testing.T) {
	t.Parallel()

	var listCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == integrationsPath:
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"message":"Some other conflict.","code":"999-999"}`))
		case r.Method == http.MethodGet && r.URL.Path == integrationsPath:
			atomic.AddInt32(&listCount, 1)
			_, _ = w.Write([]byte(`{"nextLink":null,"data":[]}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	client := Client{url: srv.URL, apiKey: "test-key"}
	_, err := client.CreateSlackIntegration(context.Background(), SlackIntegrationData{
		FriendlyName: "Anything",
		WebhookURL:   "https://hooks.slack.com/services/SHARED",
	})
	if err == nil {
		t.Fatalf("expected original 409 to be surfaced for unrecognized code")
	}
	if got := atomic.LoadInt32(&listCount); got != 0 {
		t.Fatalf("expected no ListIntegrations call for unrecognized 409 code, got %d", got)
	}
}

func TestCreateSlackIntegration_ListFailureSurfacesOriginal409(t *testing.T) {
	t.Parallel()

	srv, listCount := slackIntegrationTestServer(t, "", http.StatusInternalServerError)
	defer srv.Close()

	client := Client{url: srv.URL, apiKey: "test-key"}
	_, err := client.CreateSlackIntegration(context.Background(), SlackIntegrationData{
		FriendlyName: "Cluster A",
		WebhookURL:   "https://hooks.slack.com/services/SHARED",
	})
	if err == nil {
		t.Fatalf("expected original 409 to be surfaced when list fails")
	}
	if got := atomic.LoadInt32(listCount); got == 0 {
		t.Fatalf("expected list attempt on 409 path, got 0")
	}
}

func TestListAndDeleteIntegrations(t *testing.T) {
	t.Parallel()

	deleteCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == integrationsPath:
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
