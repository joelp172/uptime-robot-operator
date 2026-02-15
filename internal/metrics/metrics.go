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
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// APIRequestsTotal tracks total API requests to UptimeRobot
	// Note: status_code label can have high cardinality (200-599 + "error").
	// Monitor cardinality in production and consider grouping into ranges (2xx, 3xx, 4xx, 5xx) if needed.
	APIRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "uptimerobot_api_requests_total",
			Help: "Total number of API requests to UptimeRobot",
		},
		[]string{"method", "endpoint", "status_code"},
	)

	// APIRequestDuration tracks API request latency
	APIRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "uptimerobot_api_request_duration_seconds",
			Help: "Duration of API requests to UptimeRobot in seconds",
			// Custom buckets optimized for HTTP API calls (100ms to 30s)
			Buckets: []float64{0.1, 0.25, 0.5, 1, 2, 5, 10, 30},
		},
		[]string{"method", "endpoint"},
	)

	// APIRetriesTotal tracks API retry attempts
	APIRetriesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "uptimerobot_api_retries_total",
			Help: "Total number of API retry attempts",
		},
		[]string{"endpoint", "reason"},
	)

	// ReconciliationErrorsTotal tracks reconciliation errors by controller
	ReconciliationErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "uptimerobot_reconciliation_errors_total",
			Help: "Total number of reconciliation errors",
		},
		[]string{"controller", "error_type"},
	)

	// ReconciliationDuration tracks reconciliation duration by controller
	ReconciliationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "uptimerobot_reconciliation_duration_seconds",
			Help:    "Duration of reconciliation loop in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"controller"},
	)

	// MonitorsTotal tracks the number of monitors by type and status
	MonitorsTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "uptimerobot_monitors_total",
			Help: "Total number of monitors by type and status",
		},
		[]string{"type", "status"},
	)

	// MaintenanceWindowsTotal tracks the number of maintenance windows
	MaintenanceWindowsTotal = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "uptimerobot_maintenance_windows_total",
			Help: "Total number of maintenance windows",
		},
	)

	// MonitorGroupsTotal tracks the number of monitor groups
	MonitorGroupsTotal = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "uptimerobot_monitor_groups_total",
			Help: "Total number of monitor groups",
		},
	)

	// RateLimitRemaining tracks remaining API quota if available
	RateLimitRemaining = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "uptimerobot_rate_limit_remaining",
			Help: "Remaining API quota",
		},
	)
)

// RegisterMetrics registers all custom metrics with the controller-runtime metrics registry
func RegisterMetrics() {
	metrics.Registry.MustRegister(
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
}
