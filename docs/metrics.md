# Prometheus Metrics

The Uptime Robot Operator exposes custom Prometheus metrics in addition to the standard controller-runtime metrics. These metrics provide insights into API performance, reconciliation behavior, and resource health.

## Enabling Metrics

By default, the metrics endpoint is **disabled**. To enable it, set the `--metrics-bind-address` flag when deploying the operator:

```yaml
# HTTP metrics on port 8443 (default in cluster manifests/charts)
args:
  - --metrics-bind-address=:8443

# HTTP metrics on port 8080 (common for local development)
args:
  - --metrics-bind-address=:8080
```

The metrics endpoint will be available at `/metrics` on the specified port.

The operator currently serves metrics over HTTP only. If TLS is required, terminate TLS at the network layer (for example, with a service mesh, ingress proxy, or gateway).

## Custom Metrics

### API Request Metrics

#### `uptimerobot_api_requests_total`
**Type:** Counter  
**Labels:**
- `method` - HTTP method (GET, POST, PUT, DELETE)
- `endpoint` - API endpoint (e.g., "monitors", "contacts")
- `status_code` - HTTP status code or "error" for network errors

**Description:** Total number of API requests made to the UptimeRobot API.

**Example:**
```promql
# Total successful monitor creation requests
uptimerobot_api_requests_total{method="POST", endpoint="monitors", status_code="200"}

# Rate of API errors
rate(uptimerobot_api_requests_total{status_code!~"2.."}[5m])
```

#### `uptimerobot_api_request_duration_seconds`
**Type:** Histogram  
**Labels:**
- `method` - HTTP method (GET, POST, PUT, DELETE)
- `endpoint` - API endpoint (e.g., "monitors", "contacts")

**Description:** Duration of API requests to UptimeRobot in seconds.

**Buckets:** Default Prometheus buckets (0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10)

**Example:**
```promql
# 99th percentile latency for monitor API calls
histogram_quantile(0.99, rate(uptimerobot_api_request_duration_seconds_bucket{endpoint="monitors"}[5m]))

# Average API call duration
rate(uptimerobot_api_request_duration_seconds_sum[5m]) / rate(uptimerobot_api_request_duration_seconds_count[5m])
```

#### `uptimerobot_api_retries_total`
**Type:** Counter  
**Labels:**
- `endpoint` - API endpoint being retried
- `reason` - Reason for retry (e.g., "timeout", "status_429", "network_error")

**Description:** Total number of API retry attempts.

**Example:**
```promql
# Rate of retries due to rate limiting
rate(uptimerobot_api_retries_total{reason="status_429"}[5m])

# Total retries per endpoint
sum by (endpoint) (uptimerobot_api_retries_total)
```

### Reconciliation Metrics

#### `uptimerobot_reconciliation_errors_total`
**Type:** Counter  
**Labels:**
- `controller` - Controller name (e.g., "monitor", "account", "contact")
- `error_type` - Type of error (e.g., "account_not_found", "secret_not_found", "api_create_error", "api_update_error", "sync_error")

**Description:** Total number of reconciliation errors by controller and error type.

**Example:**
```promql
# Error rate by controller
rate(uptimerobot_reconciliation_errors_total[5m])

# Most common error types
topk(5, sum by (error_type) (uptimerobot_reconciliation_errors_total))
```

#### `uptimerobot_reconciliation_duration_seconds`
**Type:** Histogram  
**Labels:**
- `controller` - Controller name (e.g., "monitor", "account", "contact")

**Description:** Duration of reconciliation loops in seconds.

**Buckets:** Default Prometheus buckets (0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10)

**Example:**
```promql
# 95th percentile reconciliation time for monitors
histogram_quantile(0.95, rate(uptimerobot_reconciliation_duration_seconds_bucket{controller="monitor"}[5m]))

# Average reconciliation time by controller
rate(uptimerobot_reconciliation_duration_seconds_sum[5m]) / rate(uptimerobot_reconciliation_duration_seconds_count[5m])
```

### Other Registered Metrics

#### `uptimerobot_monitors_total`
**Type:** Gauge  
**Status:** Registered but not populated by controllers yet.

#### `uptimerobot_maintenance_windows_total`
**Type:** Gauge  
**Status:** Registered but not populated by controllers yet.

#### `uptimerobot_monitor_groups_total`
**Type:** Gauge  
**Status:** Registered but not populated by controllers yet.

#### `uptimerobot_rate_limit_remaining`
**Type:** Gauge

**Description:** Remaining API quota (if available from API responses).

**Note:** This metric is not yet implemented. UptimeRobot API v3 does not currently expose rate limit information in response headers.

## Standard Controller-Runtime Metrics

In addition to the custom metrics above, the operator exposes standard controller-runtime metrics:

- `controller_runtime_reconcile_total` - Total reconciliations per controller
- `controller_runtime_reconcile_errors_total` - Total reconciliation errors
- `controller_runtime_reconcile_time_seconds` - Reconciliation latency
- `workqueue_*` - Work queue metrics (depth, adds, retries, etc.)
- `rest_client_*` - Kubernetes API client metrics
- `go_*` - Go runtime metrics (goroutines, memory, GC, etc.)
- `process_*` - Process metrics (CPU, memory, file descriptors, etc.)

## Querying Examples

### Dashboard Queries

#### API Health
```promql
# API success rate (%)
100 * (
  sum(rate(uptimerobot_api_requests_total{status_code=~"2.."}[5m]))
  /
  sum(rate(uptimerobot_api_requests_total[5m]))
)

# API error rate by status code
sum by (status_code) (rate(uptimerobot_api_requests_total{status_code!~"2.."}[5m]))
```

#### Reconciliation Health
```promql
# Reconciliation error rate by controller
sum by (controller) (rate(uptimerobot_reconciliation_errors_total[5m]))

# Slowest reconciling controller (95th percentile)
topk(5, histogram_quantile(0.95, rate(uptimerobot_reconciliation_duration_seconds_bucket[5m])))
```

#### API Performance
```promql
# API requests per second by endpoint
sum by (endpoint) (rate(uptimerobot_api_requests_total[5m]))

# API latency heatmap
sum(rate(uptimerobot_api_request_duration_seconds_bucket[5m])) by (le)
```

### Alerting Rules

#### High API Error Rate
```yaml
- alert: HighAPIErrorRate
  expr: |
    (
      sum(rate(uptimerobot_api_requests_total{status_code!~"2.."}[5m]))
      /
      sum(rate(uptimerobot_api_requests_total[5m]))
    ) > 0.1
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "High API error rate"
    description: "API error rate is {{ $value | humanizePercentage }} over the last 5 minutes"
```

#### High Reconciliation Error Rate
```yaml
- alert: HighReconciliationErrorRate
  expr: |
    sum by (controller) (rate(uptimerobot_reconciliation_errors_total[5m])) > 0.5
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "High reconciliation error rate for {{ $labels.controller }}"
    description: "{{ $labels.controller }} controller is experiencing {{ $value }} errors per second"
```

#### Slow Reconciliation
```yaml
- alert: SlowReconciliation
  expr: |
    histogram_quantile(0.95,
      rate(uptimerobot_reconciliation_duration_seconds_bucket[5m])
    ) > 10
  for: 10m
  labels:
    severity: warning
  annotations:
    summary: "Slow reconciliation for {{ $labels.controller }}"
    description: "95th percentile reconciliation time is {{ $value }}s for {{ $labels.controller }}"
```

#### High API Retry Rate
```yaml
- alert: HighAPIRetryRate
  expr: |
    sum(rate(uptimerobot_api_retries_total[5m])) > 1
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "High API retry rate"
    description: "API retry rate is {{ $value }} per second"
```

## Grafana Dashboard

A sample Grafana dashboard is provided in `docs/grafana-dashboard.json`. Import it into Grafana to visualize:

- API request rate and latency
- API error rate by status code
- Reconciliation duration and error rate by controller
- API retry patterns

The dashboard expects a Prometheus datasource with UID `prometheus`. Update datasource UIDs in the JSON if your Grafana datasource uses a different UID.

## Security Considerations

Metrics are exposed over HTTP by the operator. For encrypted or authenticated transport, enforce controls at the network layer (for example, mTLS/mesh policy, ingress auth, namespace network policy, or private cluster networking).

## Troubleshooting

### Metrics not appearing

1. Check that the metrics endpoint is enabled:
   ```bash
   kubectl get deployment -n uptime-robot-system uptime-robot-controller-manager -o yaml | grep metrics-bind-address
   ```

2. Verify the metrics endpoint is accessible (port must match your `--metrics-bind-address`):
   ```bash
   kubectl port-forward -n uptime-robot-system deployment/uptime-robot-controller-manager 8443:8443
   curl http://localhost:8443/metrics | grep uptimerobot_
   ```

3. Check operator logs for metric registration:
   ```bash
   kubectl logs -n uptime-robot-system deployment/uptime-robot-controller-manager | grep metrics
   ```

### Missing custom metrics

If you see standard controller-runtime metrics but not custom `uptimerobot_*` metrics:

1. Verify the operator version supports custom metrics (v0.2.0+)
2. Check that metrics were registered successfully in the logs
3. Ensure at least one reconciliation has occurred (vec metrics only appear after first label combination is used)

### High cardinality warnings

If you see warnings about high metric cardinality:

1. Review the number of unique monitor types and statuses
2. Consider aggregating metrics using recording rules
3. Adjust retention policies in Prometheus

## Further Reading

- [Prometheus Best Practices](https://prometheus.io/docs/practices/naming/)
- [Controller-Runtime Metrics](https://book.kubebuilder.io/reference/metrics.html)
- [Grafana Dashboards](https://grafana.com/docs/grafana/latest/dashboards/)
