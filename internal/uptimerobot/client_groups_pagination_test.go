/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package uptimerobot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEnumerateGroupsFromBackend_FollowsNextLink(t *testing.T) {
	t.Parallel()

	var baseURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/monitor-groups" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		switch r.URL.Query().Get("page") {
		case "":
			next := baseURL + "/monitor-groups?page=2"
			_, _ = w.Write([]byte(`{
				"nextLink": "` + next + `",
				"data": [
					{"id": 1, "name": "Page One"}
				]
			}`))
		case "2":
			next := baseURL + "/monitor-groups?page=3"
			_, _ = w.Write([]byte(`{
				"nextLink": "` + next + `",
				"data": [
					{"id": 2, "name": "Page Two"}
				]
			}`))
		case "3":
			_, _ = w.Write([]byte(`{
				"nextLink": null,
				"data": [
					{"id": 3, "name": "Page Three"}
				]
			}`))
		default:
			t.Fatalf("unexpected page: %q", r.URL.Query().Get("page"))
		}
	}))
	defer srv.Close()
	baseURL = srv.URL

	client := Client{url: srv.URL, apiKey: "test-key"}
	groups, err := client.EnumerateGroupsFromBackend(context.Background())
	if err != nil {
		t.Fatalf("EnumerateGroupsFromBackend returned error: %v", err)
	}
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups across paginated responses, got %d", len(groups))
	}
	if groups[0].ID != 1 || groups[1].ID != 2 || groups[2].ID != 3 {
		t.Fatalf("unexpected pagination order: %+v", groups)
	}
}

func TestEnumerateGroupsFromBackend_RelativeNextLink(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "":
			_, _ = w.Write([]byte(`{
				"nextLink": "/monitor-groups?page=2",
				"data": [{"id": 10, "name": "Relative One"}]
			}`))
		case "2":
			_, _ = w.Write([]byte(`{
				"nextLink": null,
				"data": [{"id": 20, "name": "Relative Two"}]
			}`))
		default:
			t.Fatalf("unexpected page: %q", r.URL.Query().Get("page"))
		}
	}))
	defer srv.Close()

	client := Client{url: srv.URL, apiKey: "test-key"}
	groups, err := client.EnumerateGroupsFromBackend(context.Background())
	if err != nil {
		t.Fatalf("EnumerateGroupsFromBackend returned error: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups via relative nextLink, got %d", len(groups))
	}
}
