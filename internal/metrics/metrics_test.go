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

package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestMetricsRegistration(t *testing.T) {
	// Create a new registry for testing
	testRegistry := prometheus.NewRegistry()

	// Register metrics
	testRegistry.MustRegister(
		APIRequestsTotal,
		APIRequestDuration,
		APIRetriesTotal,
		ReconciliationErrorsTotal,
		ReconciliationDuration,
		MonitorsTotal,
		MaintenanceWindowsTotal,
		MonitorGroupsTotal,
		RateLimitRemaining,
	)

	// Initialize some label combinations so vec metrics show up
	APIRequestsTotal.WithLabelValues("GET", "monitors", "200").Add(0)
	APIRequestDuration.WithLabelValues("GET", "monitors").Observe(0)
	APIRetriesTotal.WithLabelValues("monitors", "timeout").Add(0)
	ReconciliationErrorsTotal.WithLabelValues("monitor", "unknown").Add(0)
	ReconciliationDuration.WithLabelValues("monitor").Observe(0)
	MonitorsTotal.WithLabelValues("http", "running").Set(0)

	// Verify metrics are registered by collecting them
	metrics, err := testRegistry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	// Expected metric names
	expectedMetrics := map[string]bool{
		"uptimerobot_api_requests_total":              false,
		"uptimerobot_api_request_duration_seconds":    false,
		"uptimerobot_api_retries_total":               false,
		"uptimerobot_reconciliation_errors_total":     false,
		"uptimerobot_reconciliation_duration_seconds": false,
		"uptimerobot_monitors_total":                  false,
		"uptimerobot_maintenance_windows_total":       false,
		"uptimerobot_monitor_groups_total":            false,
		"uptimerobot_rate_limit_remaining":            false,
	}

	// Check that all expected metrics are present
	for _, mf := range metrics {
		if _, ok := expectedMetrics[mf.GetName()]; ok {
			expectedMetrics[mf.GetName()] = true
		}
	}

	// Verify all metrics were found
	for name, found := range expectedMetrics {
		if !found {
			t.Errorf("Expected metric %s was not registered", name)
		}
	}
}

func TestAPIRequestsTotal(t *testing.T) {
	// Test that we can increment the counter
	APIRequestsTotal.WithLabelValues("GET", "monitors", "200").Inc()

	// Verify the metric was recorded
	metricCh := make(chan prometheus.Metric, 1)
	APIRequestsTotal.Collect(metricCh)
	close(metricCh)

	metric := <-metricCh
	var m dto.Metric
	if err := metric.Write(&m); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	if m.Counter == nil {
		t.Fatal("Expected counter metric")
	}

	if m.Counter.GetValue() <= 0 {
		t.Errorf("Expected counter value > 0, got %v", m.Counter.GetValue())
	}
}

func TestAPIRequestDuration(t *testing.T) {
	// Test that we can observe durations
	APIRequestDuration.WithLabelValues("GET", "monitors").Observe(0.5)
	APIRequestDuration.WithLabelValues("GET", "monitors").Observe(1.0)

	// Verify the metric was recorded
	metricCh := make(chan prometheus.Metric, 10)
	APIRequestDuration.Collect(metricCh)
	close(metricCh)

	metric := <-metricCh
	var m dto.Metric
	if err := metric.Write(&m); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	if m.Histogram == nil {
		t.Fatal("Expected histogram metric")
	}

	if m.Histogram.GetSampleCount() <= 0 {
		t.Errorf("Expected sample count > 0, got %v", m.Histogram.GetSampleCount())
	}
}

func TestReconciliationDuration(t *testing.T) {
	// Test that we can observe reconciliation durations
	ReconciliationDuration.WithLabelValues("monitor").Observe(0.1)

	// Verify the metric was recorded
	metricCh := make(chan prometheus.Metric, 10)
	ReconciliationDuration.Collect(metricCh)
	close(metricCh)

	metric := <-metricCh
	var m dto.Metric
	if err := metric.Write(&m); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	if m.Histogram == nil {
		t.Fatal("Expected histogram metric")
	}

	if m.Histogram.GetSampleCount() <= 0 {
		t.Errorf("Expected sample count > 0, got %v", m.Histogram.GetSampleCount())
	}
}

func TestMonitorsTotal(t *testing.T) {
	// Test that we can set gauge values
	MonitorsTotal.WithLabelValues("http", "running").Set(5)
	MonitorsTotal.WithLabelValues("http", "paused").Set(2)

	// Verify the metrics were recorded
	metricCh := make(chan prometheus.Metric, 10)
	MonitorsTotal.Collect(metricCh)
	close(metricCh)

	count := 0
	for metric := range metricCh {
		var m dto.Metric
		if err := metric.Write(&m); err != nil {
			t.Fatalf("Failed to write metric: %v", err)
		}

		if m.Gauge == nil {
			t.Fatal("Expected gauge metric")
		}
		count++
	}

	if count != 2 {
		t.Errorf("Expected 2 gauge metrics, got %d", count)
	}
}

func TestMaintenanceWindowsTotal(t *testing.T) {
	// Test that we can set the gauge
	MaintenanceWindowsTotal.Set(3)

	// Verify the metric was recorded
	metricCh := make(chan prometheus.Metric, 1)
	MaintenanceWindowsTotal.Collect(metricCh)
	close(metricCh)

	metric := <-metricCh
	var m dto.Metric
	if err := metric.Write(&m); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	if m.Gauge == nil {
		t.Fatal("Expected gauge metric")
	}

	if m.Gauge.GetValue() != 3 {
		t.Errorf("Expected gauge value 3, got %v", m.Gauge.GetValue())
	}
}
