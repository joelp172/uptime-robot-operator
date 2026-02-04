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
	"strings"

	"github.com/joelp172/uptime-robot-operator/internal/uptimerobot/uptimerobottest/responses"
)

// NewServer creates a new test server that mimics the UptimeRobot v3 API.
func NewServer() *httptest.Server {
	mux := http.NewServeMux()

	// GET /monitors - List monitors
	mux.HandleFunc("GET /monitors", handleGetMonitors)
	mux.HandleFunc("GET /monitors/", handleGetMonitors)

	// POST /monitors - Create monitor
	mux.HandleFunc("POST /monitors", handleCreateMonitor)

	// PATCH /monitors/{id} - Update monitor
	mux.HandleFunc("PATCH /monitors/", handleUpdateMonitor)

	// DELETE /monitors/{id} - Delete monitor
	mux.HandleFunc("DELETE /monitors/", handleDeleteMonitor)

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

	return httptest.NewServer(mux)
}

func handleGetMonitors(w http.ResponseWriter, r *http.Request) {
	// Check for specific monitor ID in path
	path := strings.TrimPrefix(r.URL.Path, "/monitors/")
	if path != "" && path != r.URL.Path {
		// Single monitor request
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

func handleUpdateMonitor(w http.ResponseWriter, r *http.Request) {
	serveJSONFile(w, "monitor_update.json")
}

func handleDeleteMonitor(w http.ResponseWriter, r *http.Request) {
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
