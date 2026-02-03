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
	"testing"

	uptimerobotv1 "github.com/joelp172/uptime-robot-operator/api/v1alpha1"
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
