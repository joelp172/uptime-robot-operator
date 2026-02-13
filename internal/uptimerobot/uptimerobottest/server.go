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

package uptimerobottest

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"

	"github.com/joelp172/uptime-robot-operator/internal/uptimerobot/uptimerobottest/responses"
)

// ServerState tracks the state of the mock server for testing purposes.
type ServerState struct {
	mu              sync.RWMutex
	deletedMonitors map[string]bool // Track deleted monitor IDs
	integrations    map[int]map[string]any
	nextIntegration int
}

func defaultIntegrations() (map[int]map[string]any, int) {
	return map[int]map[string]any{
		101: {
			"id":                     101,
			"friendlyName":           "Mock Slack",
			"enableNotificationsFor": "Down",
			"type":                   "Slack",
			"status":                 "Active",
			"sslExpirationReminder":  false,
			"value":                  "https://hooks.slack.com/services/T000/B000/MOCK",
			"customValue":            "mock",
			"customValue2":           "",
			"customValue3":           "",
			"customValue4":           "",
		},
	}, 102
}

// NewServerState creates a new server state tracker.
func NewServerState() *ServerState {
	integrations, next := defaultIntegrations()
	return &ServerState{
		deletedMonitors: make(map[string]bool),
		integrations:    integrations,
		nextIntegration: next,
	}
}

// MarkMonitorDeleted marks a monitor as deleted.
func (s *ServerState) MarkMonitorDeleted(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deletedMonitors[id] = true
}

// MarkMonitorActive clears deleted state for a monitor.
func (s *ServerState) MarkMonitorActive(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.deletedMonitors, id)
}

// IsMonitorDeleted checks if a monitor has been deleted.
func (s *ServerState) IsMonitorDeleted(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.deletedMonitors[id]
}

// Reset clears all tracked state.
func (s *ServerState) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	integrations, next := defaultIntegrations()
	s.deletedMonitors = make(map[string]bool)
	s.integrations = integrations
	s.nextIntegration = next
}

func (s *ServerState) createIntegration(body map[string]any) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextIntegration
	s.nextIntegration++

	data, _ := body["data"].(map[string]any)
	friendlyName, _ := data["friendlyName"].(string)
	enable, _ := data["enableNotificationsFor"].(string)
	webhook, _ := data["webhookURL"].(string)
	customValue, _ := data["customValue"].(string)
	ssl, _ := data["sslExpirationReminder"].(bool)

	record := map[string]any{
		"id":                     id,
		"friendlyName":           friendlyName,
		"enableNotificationsFor": enable,
		"type":                   "Slack",
		"status":                 "Active",
		"sslExpirationReminder":  ssl,
		"value":                  webhook,
		"customValue":            customValue,
		"customValue2":           "",
		"customValue3":           "",
		"customValue4":           "",
	}
	s.integrations[id] = record
	return record
}

func (s *ServerState) listIntegrations() []map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]map[string]any, 0, len(s.integrations))
	for _, integration := range s.integrations {
		out = append(out, integration)
	}
	return out
}

func (s *ServerState) deleteIntegration(id int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.integrations, id)
}

// NewServer creates a new test server that mimics the UptimeRobot v3 API.
func NewServer() *httptest.Server {
	state := NewServerState()
	return NewServerWithState(state)
}

// NewServerWithState creates a new test server with the given state tracker.
func NewServerWithState(state *ServerState) *httptest.Server {
	mux := http.NewServeMux()

	// GET /monitors - List monitors
	mux.HandleFunc("GET /monitors", func(w http.ResponseWriter, r *http.Request) {
		handleGetMonitors(w, r, state)
	})
	mux.HandleFunc("GET /monitors/", func(w http.ResponseWriter, r *http.Request) {
		handleGetMonitors(w, r, state)
	})

	// POST /monitors - Create monitor
	mux.HandleFunc("POST /monitors", handleCreateMonitor)

	// PATCH /monitors/{id} - Update monitor
	mux.HandleFunc("PATCH /monitors/", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateMonitor(w, r, state)
	})

	// DELETE /monitors/{id} - Delete monitor
	mux.HandleFunc("DELETE /monitors/", func(w http.ResponseWriter, r *http.Request) {
		handleDeleteMonitor(w, r, state)
	})

	// POST /monitors/{id}/pause - Pause monitor
	mux.HandleFunc("POST /monitors/{id}/pause", func(w http.ResponseWriter, r *http.Request) {
		handlePauseMonitor(w, r, state)
	})

	// POST /monitors/{id}/start - Start monitor
	mux.HandleFunc("POST /monitors/{id}/start", func(w http.ResponseWriter, r *http.Request) {
		handleStartMonitor(w, r, state)
	})

	// GET /user/me - Get user info
	mux.HandleFunc("GET /user/me", handleGetUser)

	// GET /user/alert-contacts - Get alert contacts
	mux.HandleFunc("GET /user/alert-contacts", handleGetAlertContacts)

	// GET /maintenance-windows - List maintenance windows
	mux.HandleFunc("GET /maintenance-windows", handleGetMaintenanceWindows)
	mux.HandleFunc("GET /maintenance-windows/", handleGetMaintenanceWindows)

	// POST /maintenance-windows - Create maintenance window
	mux.HandleFunc("POST /maintenance-windows", handleCreateMaintenanceWindow)

	// PATCH /maintenance-windows/{id} - Update maintenance window
	mux.HandleFunc("PATCH /maintenance-windows/", handleUpdateMaintenanceWindow)

	// DELETE /maintenance-windows/{id} - Delete maintenance window
	mux.HandleFunc("DELETE /maintenance-windows/", handleDeleteMaintenanceWindow)

	// GET /monitor-groups - List monitor groups
	mux.HandleFunc("GET /monitor-groups", handleGetMonitorGroups)
	mux.HandleFunc("GET /monitor-groups/", handleGetMonitorGroups)

	// POST /monitor-groups - Create monitor group
	mux.HandleFunc("POST /monitor-groups", handleCreateMonitorGroup)

	// PATCH /monitor-groups/{id} - Update monitor group
	mux.HandleFunc("PATCH /monitor-groups/", handleUpdateMonitorGroup)

	// DELETE /monitor-groups/{id} - Delete monitor group
	mux.HandleFunc("DELETE /monitor-groups/", handleDeleteMonitorGroup)

	// Integrations endpoints
	mux.HandleFunc("GET /integrations", func(w http.ResponseWriter, r *http.Request) {
		handleGetIntegrations(w, state)
	})
	mux.HandleFunc("POST /integrations", func(w http.ResponseWriter, r *http.Request) {
		handleCreateIntegration(w, r, state)
	})
	mux.HandleFunc("DELETE /integrations/", func(w http.ResponseWriter, r *http.Request) {
		handleDeleteIntegration(w, r, state)
	})

	return httptest.NewServer(mux)
}

func handleGetMonitors(w http.ResponseWriter, r *http.Request, state *ServerState) {
	// Check for specific monitor ID in path
	path := strings.TrimPrefix(r.URL.Path, "/monitors/")
	if path != "" && path != r.URL.Path {
		// Single monitor request - check if deleted
		monitorID := path
		if state.IsMonitorDeleted(monitorID) {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "monitor not found"})
			return
		}
		serveJSONFile(w, "monitor.json")
		return
	}

	// List monitors
	serveJSONFile(w, "monitors.json")
}

func handleCreateMonitor(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusCreated)
	serveJSONFile(w, "monitor_create.json")
}

func handleUpdateMonitor(w http.ResponseWriter, r *http.Request, state *ServerState) {
	monitorID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/monitors/"), "/")
	if monitorID != "" && monitorID != r.URL.Path {
		// Simulate monitor becoming active again after update/recreate path.
		state.MarkMonitorActive(monitorID)
	}
	serveJSONFile(w, "monitor_update.json")
}

func handleDeleteMonitor(w http.ResponseWriter, r *http.Request, state *ServerState) {
	// Extract monitor ID from path
	path := strings.TrimPrefix(r.URL.Path, "/monitors/")
	if path != "" && path != r.URL.Path {
		monitorID := path
		state.MarkMonitorDeleted(monitorID)
	}
	w.WriteHeader(http.StatusNoContent)
}

func handlePauseMonitor(w http.ResponseWriter, r *http.Request, state *ServerState) {
	// POST /monitors/{id}/pause - Pause monitor
	monitorID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/monitors/"), "/pause")
	if monitorID != "" && monitorID != r.URL.Path && state.IsMonitorDeleted(monitorID) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "monitor not found"})
		return
	}
	// This is idempotent - pausing an already paused monitor returns success
	w.WriteHeader(http.StatusNoContent)
}

func handleStartMonitor(w http.ResponseWriter, r *http.Request, state *ServerState) {
	// POST /monitors/{id}/start - Start (resume) monitor
	monitorID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/monitors/"), "/start")
	if monitorID != "" && monitorID != r.URL.Path && state.IsMonitorDeleted(monitorID) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "monitor not found"})
		return
	}
	// This is idempotent - starting an already active monitor returns success
	w.WriteHeader(http.StatusNoContent)
}

func handleGetUser(w http.ResponseWriter, r *http.Request) {
	serveJSONFile(w, "user_me.json")
}

func handleGetAlertContacts(w http.ResponseWriter, r *http.Request) {
	serveJSONFile(w, "alert_contacts.json")
}

func handleGetMaintenanceWindows(w http.ResponseWriter, r *http.Request) {
	// Check for specific maintenance window ID in path
	path := strings.TrimPrefix(r.URL.Path, "/maintenance-windows/")
	if path != "" && path != r.URL.Path {
		// Single maintenance window request
		serveJSONFile(w, "maintenance_window.json")
		return
	}

	// List maintenance windows
	serveJSONFile(w, "maintenance_windows.json")
}

func handleCreateMaintenanceWindow(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusCreated)
	serveJSONFile(w, "maintenance_window_create.json")
}

func handleUpdateMaintenanceWindow(w http.ResponseWriter, r *http.Request) {
	serveJSONFile(w, "maintenance_window_update.json")
}

func handleDeleteMaintenanceWindow(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func handleGetMonitorGroups(w http.ResponseWriter, r *http.Request) {
	// Check for specific monitor group ID in path
	path := strings.TrimPrefix(r.URL.Path, "/monitor-groups/")
	if path != "" && path != r.URL.Path {
		// Single monitor group request
		serveJSONFile(w, "monitor_group.json")
		return
	}

	// List monitor groups
	serveJSONFile(w, "monitor_groups.json")
}

func handleCreateMonitorGroup(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusCreated)
	serveJSONFile(w, "monitor_group_create.json")
}

func handleUpdateMonitorGroup(w http.ResponseWriter, r *http.Request) {
	serveJSONFile(w, "monitor_group_update.json")
}

func handleDeleteMonitorGroup(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func handleGetIntegrations(w http.ResponseWriter, state *ServerState) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"nextLink": nil,
		"data":     state.listIntegrations(),
	})
}

func handleCreateIntegration(w http.ResponseWriter, r *http.Request, state *ServerState) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid json"})
		return
	}
	record := state.createIntegration(body)
	w.WriteHeader(http.StatusCreated)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(record)
}

func handleDeleteIntegration(w http.ResponseWriter, r *http.Request, state *ServerState) {
	path := strings.TrimPrefix(r.URL.Path, "/integrations/")
	if path != "" && path != r.URL.Path {
		if id, err := strconv.Atoi(path); err == nil {
			state.deleteIntegration(id)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func serveJSONFile(w http.ResponseWriter, filename string) {
	f, err := responses.FS.Open(filename)
	if err != nil {
		// Return a generic success response if file doesn't exist
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return
	}
	defer func() { _ = f.Close() }()

	w.Header().Set("Content-Type", "application/json")
	_, _ = io.Copy(w, f)
}
