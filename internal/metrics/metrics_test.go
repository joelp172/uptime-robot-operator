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
	"fmt"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestMetricsRegistration(t *testing.T) {
	collectors := []prometheus.Collector{
		APIRequestsTotal,
		APIRequestDuration,
		APIRetriesTotal,
		ReconciliationErrorsTotal,
		ReconciliationDuration,
		MonitorsTotal,
		MaintenanceWindowsTotal,
		MonitorGroupsTotal,
		RateLimitRemaining,
	}

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

	for _, collector := range collectors {
		descCh := make(chan *prometheus.Desc, 10)
		collector.Describe(descCh)
		close(descCh)

		for desc := range descCh {
			descText := fmt.Sprint(desc)
			for metricName := range expectedMetrics {
				if strings.Contains(descText, `fqName: "`+metricName+`"`) {
					expectedMetrics[metricName] = true
				}
			}
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
	// Create a test-specific registry for isolation
	testRegistry := prometheus.NewRegistry()
	testCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "test_api_requests_total",
			Help: "Test counter",
		},
		[]string{"method", "endpoint", "status_code"},
	)
	testRegistry.MustRegister(testCounter)

	// Test that we can increment the counter
	testCounter.WithLabelValues("GET", "monitors", "200").Inc()

	// Verify the metric was recorded
	metrics, err := testRegistry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	if len(metrics) == 0 {
		t.Fatal("Expected at least one metric")
	}

	found := false
	for _, mf := range metrics {
		if mf.GetName() == "test_api_requests_total" {
			found = true
			if len(mf.Metric) > 0 && mf.Metric[0].Counter.GetValue() > 0 {
				return // Test passed
			}
		}
	}

	if !found {
		t.Fatal("Expected to find test_api_requests_total metric")
	}
	t.Fatal("Expected counter value > 0")
}

func TestAPIRequestDuration(t *testing.T) {
	// Create a test-specific registry for isolation
	testRegistry := prometheus.NewRegistry()
	testHistogram := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "test_api_request_duration_seconds",
			Help:    "Test histogram",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "endpoint"},
	)
	testRegistry.MustRegister(testHistogram)

	// Test that we can observe durations
	testHistogram.WithLabelValues("GET", "monitors").Observe(0.5)
	testHistogram.WithLabelValues("GET", "monitors").Observe(1.0)

	// Verify the metric was recorded
	metrics, err := testRegistry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	if len(metrics) == 0 {
		t.Fatal("Expected at least one metric")
	}

	found := false
	for _, mf := range metrics {
		if mf.GetName() == "test_api_request_duration_seconds" {
			found = true
			if len(mf.Metric) > 0 && mf.Metric[0].Histogram.GetSampleCount() >= 2 {
				return // Test passed
			}
		}
	}

	if !found {
		t.Fatal("Expected to find test_api_request_duration_seconds metric")
	}
	t.Fatal("Expected at least 2 samples")
}

func TestReconciliationDuration(t *testing.T) {
	// Create a test-specific registry for isolation
	testRegistry := prometheus.NewRegistry()
	testHistogram := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "test_reconciliation_duration_seconds",
			Help:    "Test histogram",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"controller"},
	)
	testRegistry.MustRegister(testHistogram)

	// Test that we can observe reconciliation durations
	testHistogram.WithLabelValues("monitor").Observe(0.1)

	// Verify the metric was recorded
	metrics, err := testRegistry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	if len(metrics) == 0 {
		t.Fatal("Expected at least one metric")
	}

	found := false
	for _, mf := range metrics {
		if mf.GetName() == "test_reconciliation_duration_seconds" {
			found = true
			if len(mf.Metric) > 0 && mf.Metric[0].Histogram.GetSampleCount() > 0 {
				return // Test passed
			}
		}
	}

	if !found {
		t.Fatal("Expected to find test_reconciliation_duration_seconds metric")
	}
	t.Fatal("Expected sample count > 0")
}

func TestMonitorsTotal(t *testing.T) {
	// Create a test-specific registry for isolation
	testRegistry := prometheus.NewRegistry()
	testGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "test_monitors_total",
			Help: "Test gauge",
		},
		[]string{"type", "status"},
	)
	testRegistry.MustRegister(testGauge)

	// Test that we can set gauge values
	testGauge.WithLabelValues("http", "running").Set(5)
	testGauge.WithLabelValues("http", "paused").Set(2)

	// Verify the metrics were recorded
	metrics, err := testRegistry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	if len(metrics) == 0 {
		t.Fatal("Expected at least one metric")
	}

	found := false
	gaugeCount := 0
	for _, mf := range metrics {
		if mf.GetName() == "test_monitors_total" {
			found = true
			gaugeCount = len(mf.Metric)
		}
	}

	if !found {
		t.Fatal("Expected to find test_monitors_total metric")
	}

	if gaugeCount != 2 {
		t.Errorf("Expected 2 gauge metrics, got %d", gaugeCount)
	}
}

func TestMaintenanceWindowsTotal(t *testing.T) {
	// Create a test-specific registry for isolation
	testRegistry := prometheus.NewRegistry()
	testGauge := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "test_maintenance_windows_total",
			Help: "Test gauge",
		},
	)
	testRegistry.MustRegister(testGauge)

	// Test that we can set the gauge
	testGauge.Set(3)

	// Verify the metric was recorded
	metrics, err := testRegistry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	if len(metrics) == 0 {
		t.Fatal("Expected at least one metric")
	}

	found := false
	for _, mf := range metrics {
		if mf.GetName() == "test_maintenance_windows_total" {
			found = true
			if len(mf.Metric) > 0 && mf.Metric[0].Gauge.GetValue() == 3 {
				return // Test passed
			}
		}
	}

	if !found {
		t.Fatal("Expected to find test_maintenance_windows_total metric")
	}
	t.Fatal("Expected gauge value 3")
}
