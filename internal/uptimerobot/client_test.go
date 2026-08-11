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
	"encoding/json"
	"strings"
	"testing"
	"time"

	uptimerobotv1 "github.com/joelp172/uptime-robot-operator/api/v1alpha1"
	"github.com/joelp172/uptime-robot-operator/internal/uptimerobot/urtypes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildCreateMonitorRequest_MaintenanceWindowIds(t *testing.T) {
	client := NewClient("test-api-key")

	t.Run("includes maintenance window IDs when provided", func(t *testing.T) {
		interval := metav1.Duration{Duration: 300000000000}   // 5m
		timeout := metav1.Duration{Duration: 30000000000}     // 30s
		gracePeriod := metav1.Duration{Duration: 60000000000} // 60s

		monitor := uptimerobotv1.MonitorValues{
			Name:                 "Test Monitor",
			URL:                  "https://example.com",
			Interval:             &interval,
			Timeout:              &timeout,
			GracePeriod:          &gracePeriod,
			MaintenanceWindowIDs: []int{12345, 67890},
		}

		req := client.buildCreateMonitorRequest(monitor, nil)

		if len(req.MaintenanceWindowsIds) != 2 {
			t.Errorf("expected 2 maintenance window IDs, got %d", len(req.MaintenanceWindowsIds))
		}
		if req.MaintenanceWindowsIds[0] != 12345 {
			t.Errorf("expected first ID to be 12345, got %d", req.MaintenanceWindowsIds[0])
		}
		if req.MaintenanceWindowsIds[1] != 67890 {
			t.Errorf("expected second ID to be 67890, got %d", req.MaintenanceWindowsIds[1])
		}
	})

	t.Run("omits maintenance window IDs when empty", func(t *testing.T) {
		interval := metav1.Duration{Duration: 300000000000}   // 5m
		timeout := metav1.Duration{Duration: 30000000000}     // 30s
		gracePeriod := metav1.Duration{Duration: 60000000000} // 60s

		monitor := uptimerobotv1.MonitorValues{
			Name:        "Test Monitor",
			URL:         "https://example.com",
			Interval:    &interval,
			Timeout:     &timeout,
			GracePeriod: &gracePeriod,
		}

		req := client.buildCreateMonitorRequest(monitor, nil)

		if req.MaintenanceWindowsIds != nil {
			t.Errorf("expected MaintenanceWindowsIds to be nil, got %v", req.MaintenanceWindowsIds)
		}
	})
}

func TestBuildCreateMonitorRequest_Region(t *testing.T) {
	client := NewClient("test-api-key")

	t.Run("includes regional data when region is set", func(t *testing.T) {
		interval := metav1.Duration{Duration: 300000000000}   // 5m
		timeout := metav1.Duration{Duration: 30000000000}     // 30s
		gracePeriod := metav1.Duration{Duration: 60000000000} // 60s

		monitor := uptimerobotv1.MonitorValues{
			Name:        "Test Monitor",
			URL:         "https://example.com",
			Interval:    &interval,
			Timeout:     &timeout,
			GracePeriod: &gracePeriod,
			Region:      "eu",
		}

		req := client.buildCreateMonitorRequest(monitor, nil)

		if req.RegionalData != "eu" {
			t.Errorf("expected RegionalData to be eu, got %q", req.RegionalData)
		}
	})

	t.Run("omits regional data when region is empty", func(t *testing.T) {
		interval := metav1.Duration{Duration: 300000000000}   // 5m
		timeout := metav1.Duration{Duration: 30000000000}     // 30s
		gracePeriod := metav1.Duration{Duration: 60000000000} // 60s

		monitor := uptimerobotv1.MonitorValues{
			Name:        "Test Monitor",
			URL:         "https://example.com",
			Interval:    &interval,
			Timeout:     &timeout,
			GracePeriod: &gracePeriod,
		}

		req := client.buildCreateMonitorRequest(monitor, nil)

		if req.RegionalData != "" {
			t.Errorf("expected RegionalData to be empty, got %q", req.RegionalData)
		}
	})
}

func TestBuildUpdateMonitorRequest_MaintenanceWindowIds(t *testing.T) {
	client := NewClient("test-api-key")

	t.Run("includes maintenance window IDs when provided", func(t *testing.T) {
		interval := metav1.Duration{Duration: 300000000000}   // 5m
		timeout := metav1.Duration{Duration: 30000000000}     // 30s
		gracePeriod := metav1.Duration{Duration: 60000000000} // 60s

		monitor := uptimerobotv1.MonitorValues{
			Name:                 "Test Monitor",
			URL:                  "https://example.com",
			Interval:             &interval,
			Timeout:              &timeout,
			GracePeriod:          &gracePeriod,
			MaintenanceWindowIDs: []int{12345, 67890},
		}

		req := client.buildUpdateMonitorRequest(monitor, nil)

		if len(req.MaintenanceWindowsIds) != 2 {
			t.Errorf("expected 2 maintenance window IDs, got %d", len(req.MaintenanceWindowsIds))
		}
		if req.MaintenanceWindowsIds[0] != 12345 {
			t.Errorf("expected first ID to be 12345, got %d", req.MaintenanceWindowsIds[0])
		}
		if req.MaintenanceWindowsIds[1] != 67890 {
			t.Errorf("expected second ID to be 67890, got %d", req.MaintenanceWindowsIds[1])
		}
	})

	t.Run("omits maintenance window IDs when empty", func(t *testing.T) {
		interval := metav1.Duration{Duration: 300000000000}   // 5m
		timeout := metav1.Duration{Duration: 30000000000}     // 30s
		gracePeriod := metav1.Duration{Duration: 60000000000} // 60s

		monitor := uptimerobotv1.MonitorValues{
			Name:        "Test Monitor",
			URL:         "https://example.com",
			Interval:    &interval,
			Timeout:     &timeout,
			GracePeriod: &gracePeriod,
		}

		req := client.buildUpdateMonitorRequest(monitor, nil)

		if req.MaintenanceWindowsIds != nil {
			t.Errorf("expected MaintenanceWindowsIds to be nil, got %v", req.MaintenanceWindowsIds)
		}
	})
}

func TestBuildUpdateMonitorRequest_Region(t *testing.T) {
	client := NewClient("test-api-key")

	t.Run("includes regional data when region is set", func(t *testing.T) {
		interval := metav1.Duration{Duration: 300000000000}   // 5m
		timeout := metav1.Duration{Duration: 30000000000}     // 30s
		gracePeriod := metav1.Duration{Duration: 60000000000} // 60s

		monitor := uptimerobotv1.MonitorValues{
			Name:        "Test Monitor",
			URL:         "https://example.com",
			Interval:    &interval,
			Timeout:     &timeout,
			GracePeriod: &gracePeriod,
			Region:      "na",
		}

		req := client.buildUpdateMonitorRequest(monitor, nil)

		if req.RegionalData != "na" {
			t.Errorf("expected RegionalData to be na, got %q", req.RegionalData)
		}
	})

	t.Run("omits regional data when region is empty", func(t *testing.T) {
		interval := metav1.Duration{Duration: 300000000000}   // 5m
		timeout := metav1.Duration{Duration: 30000000000}     // 30s
		gracePeriod := metav1.Duration{Duration: 60000000000} // 60s

		monitor := uptimerobotv1.MonitorValues{
			Name:        "Test Monitor",
			URL:         "https://example.com",
			Interval:    &interval,
			Timeout:     &timeout,
			GracePeriod: &gracePeriod,
		}

		req := client.buildUpdateMonitorRequest(monitor, nil)

		if req.RegionalData != "" {
			t.Errorf("expected RegionalData to be empty, got %q", req.RegionalData)
		}
	})
}

func TestBuildUpdateMonitorRequest_PingOmitsUnsupportedFields(t *testing.T) {
	client := NewClient("test-api-key")
	interval := metav1.Duration{Duration: 300000000000}   // 5m
	timeout := metav1.Duration{Duration: 30000000000}     // 30s
	gracePeriod := metav1.Duration{Duration: 60000000000} // 60s

	monitor := uptimerobotv1.MonitorValues{
		Name:        "Ping Monitor",
		Type:        urtypes.TypePing,
		URL:         "8.8.8.8",
		Interval:    &interval,
		Timeout:     &timeout,
		GracePeriod: &gracePeriod,
	}

	req := client.buildUpdateMonitorRequest(monitor, nil)

	if req.URL != "" {
		t.Errorf("expected URL to be omitted for ping updates, got %q", req.URL)
	}
	if req.HTTPMethod != "" {
		t.Errorf("expected HTTPMethod to be omitted for ping updates, got %q", req.HTTPMethod)
	}
	if req.Timeout != 0 {
		t.Errorf("expected Timeout to be omitted for ping updates, got %d", req.Timeout)
	}
}

func TestBuildUpdateMonitorRequest_PortOmitsURL(t *testing.T) {
	client := NewClient("test-api-key")
	interval := metav1.Duration{Duration: time.Minute}
	timeout := metav1.Duration{Duration: 30 * time.Second}
	gracePeriod := metav1.Duration{Duration: time.Minute}

	monitor := uptimerobotv1.MonitorValues{
		Name:        "Port Monitor",
		Type:        urtypes.TypePort,
		URL:         "example.com",
		Port:        &uptimerobotv1.MonitorPort{Number: 443},
		Interval:    &interval,
		Timeout:     &timeout,
		GracePeriod: &gracePeriod,
	}

	req := client.buildUpdateMonitorRequest(monitor, nil)

	// v3 rejects a URL on PORT edits with "Invalid URL for this monitor type",
	// so the host is omitted on update (the port itself is still sent and editable).
	if req.URL != "" {
		t.Errorf("expected URL to be omitted for port updates, got %q", req.URL)
	}
	if req.Port != 443 {
		t.Errorf("expected Port to be preserved on port updates, got %d", req.Port)
	}
	// buildUpdateMonitorRequest sends Timeout in whole seconds.
	if wantTimeout := int(timeout.Seconds()); req.Timeout != wantTimeout {
		t.Errorf("expected Timeout %d for port updates, got %d", wantTimeout, req.Timeout)
	}
}

// TestMonitorRequestsCarryResponseTimeThreshold pins the field that the
// "HTTPS Full" e2e spec reports as lost: it sets responseTimeThreshold=5000 and
// the UptimeRobot API returns 0. This asserts the operator's side of that
// exchange -- the value reaches both request structs and survives JSON
// marshalling under the name the API spec uses (api-spec/monitor-post.json
// documents "responseTimeThreshold", a number in 0..60000).
func TestMonitorRequestsCarryResponseTimeThreshold(t *testing.T) {
	client := NewClient("test-api-key")
	threshold := 5000
	interval := metav1.Duration{Duration: 5 * time.Minute}
	timeout := metav1.Duration{Duration: 30 * time.Second}
	gracePeriod := metav1.Duration{Duration: time.Minute}

	monitor := uptimerobotv1.MonitorValues{
		Name:                  "E2E HTTPS Full",
		Type:                  urtypes.TypeHTTPS,
		URL:                   "https://httpbin.org/get",
		Interval:              &interval,
		Timeout:               &timeout,
		GracePeriod:           &gracePeriod,
		ResponseTimeThreshold: &threshold,
	}

	t.Run("create request", func(t *testing.T) {
		req := client.buildCreateMonitorRequest(monitor, uptimerobotv1.MonitorContacts{})
		if req.ResponseTimeThreshold == nil {
			t.Fatal("expected ResponseTimeThreshold to be set on create")
		}
		if *req.ResponseTimeThreshold != threshold {
			t.Errorf("expected ResponseTimeThreshold %d on create, got %d", threshold, *req.ResponseTimeThreshold)
		}

		body, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("failed to marshal create request: %v", err)
		}
		if !strings.Contains(string(body), `"responseTimeThreshold":5000`) {
			t.Errorf("create request JSON missing responseTimeThreshold: %s", body)
		}
	})

	t.Run("update request", func(t *testing.T) {
		req := client.buildUpdateMonitorRequest(monitor, nil)
		if req.ResponseTimeThreshold == nil {
			t.Fatal("expected ResponseTimeThreshold to be set on update")
		}
		if *req.ResponseTimeThreshold != threshold {
			t.Errorf("expected ResponseTimeThreshold %d on update, got %d", threshold, *req.ResponseTimeThreshold)
		}

		body, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("failed to marshal update request: %v", err)
		}
		if !strings.Contains(string(body), `"responseTimeThreshold":5000`) {
			t.Errorf("update request JSON missing responseTimeThreshold: %s", body)
		}
	})
}
