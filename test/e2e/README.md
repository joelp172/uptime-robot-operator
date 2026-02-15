# E2E Test Guide

End-to-end tests validate the operator against the real UptimeRobot API.

## Running Tests

Choose the mode that matches your goal:

| Mode | Command | API Key | What it runs |
|------|---------|---------|--------------|
| **Basic** | `make test-e2e` | Not needed | Operator deployment, metrics endpoint |
| **Full** | `make test-e2e-real` | Required | CRD reconciliation (monitors, maintenance windows, etc.) |
| **Verbose** | `make test-e2e-verbose` | Required | All tests (basic + full) with detailed debug output |

### Basic Tests (No API Key)

Validates operator deployment and metrics. Skips all CRD reconciliation tests that call the UptimeRobot API.

```bash
# 1) Create Kind cluster and deploy operator
make dev-cluster

# 2) Run basic tests
make test-e2e
```

Prerequisites: Kind cluster named `kind`. Use `KIND_CLUSTER=e2e-test make dev-cluster` if your cluster has a different name.

### Full Tests (Requires API Key)

Creates real monitors, maintenance windows, and other resources in UptimeRobot. Use a test account, not production.

```bash
make dev-cluster
export UPTIME_ROBOT_API_KEY=your-test-key
make test-e2e-real
```

Prerequisites: Kind cluster named `kind`, and `UPTIME_ROBOT_API_KEY` set.

### Verbose Mode (Debugging)

When tests fail, run with verbose output to see node events and stack traces:

```bash
make dev-cluster
export UPTIME_ROBOT_API_KEY=your-test-key
make test-e2e-verbose
```

Or use the raw `go test` command for full control:

```bash
kind delete cluster --name kind || true
kind create cluster --name kind
kubectl config use-context kind-kind
make cert-manager-install

export UPTIME_ROBOT_API_KEY=your-test-key
go test ./test/e2e -run TestE2E -count=1 -v -timeout=20m -args \
  -ginkgo.v -ginkgo.show-node-events -ginkgo.trace
```

Use `KIND_CLUSTER=e2e-test` if your cluster name is not `kind`.

### Running Specific Test Suites

Run only specific test suites using Ginkgo label filters:

```bash
# Run only MonitorGroup tests
export UPTIME_ROBOT_API_KEY=your-test-key
go test ./test/e2e/ -v -ginkgo.v -ginkgo.label-filter="monitorgroup" -timeout 20m

# Run only Monitor tests
go test ./test/e2e/ -v -ginkgo.v -ginkgo.label-filter="monitor" -timeout 20m

# Run specific monitor tests
go test ./test/e2e/ -v -ginkgo.v -ginkgo.label-filter="monitor" -ginkgo.focus="Regional Monitoring|specified region" -timeout 20m
UPTIME_ROBOT_API_KEY=your_key KIND_CLUSTER=kind go test ./test/e2e/ -v -ginkgo.v -ginkgo.label-filter="monitor" -ginkgo.focus="Regional Monitoring|specified region" -timeout 20m

# Run only MaintenanceWindow tests
go test ./test/e2e/ -v -ginkgo.v -ginkgo.label-filter="maintenancewindow" -timeout 20m

# Run only Account and Contact tests
go test ./test/e2e/ -v -ginkgo.v -ginkgo.label-filter="account || contact" -timeout 20m
```

**Note:** Ensure the operator is deployed before running specific suites (`make dev-cluster`).

### Debug Logging

```bash
E2E_DEBUG=1 make test-e2e-real
```

## Test Coverage

### Monitor Tests

- HTTP/HTTPS basic and full configuration
- Authentication (Basic auth)
- POST requests with body
- Keyword monitors (Exists/NotExists)
- Ping monitors
- Port monitors
- DNS monitors
- Contact assignment and thresholds

### MonitorGroup Tests

- Basic lifecycle (create, update, delete)
- Prune behaviour on deletion
- Monitor references and tracking
- Monitor count validation

### MaintenanceWindow Tests

- All interval types (once, daily, weekly, monthly)
- Schedule updates
- Monitor references (add/update/remove)
- Prune behaviour (true/false)
- Duration handling

## Test Structure

Tests run in order:
1. Suite setup (build image, deploy operator)
2. CRD reconciliation tests
3. MaintenanceWindow tests
4. Suite teardown

Each test:
1. Creates resources via kubectl
2. Waits for status.ready=true
3. Validates against UptimeRobot API
4. Cleans up resources

## Configuration

- **Cluster:** Kind cluster named `kind`
- **cert-manager:** pinned to `v1.16.2` by default (`CERT_MANAGER_VERSION` to override)
- **API endpoint:** `https://api.uptimerobot.com/v3` (override with `UPTIME_ROBOT_API`)
- **Poll timeout:** 3 minutes
- **Poll interval:** 5 seconds

## Troubleshooting

### Tests Hang

Enable debug logging:

```bash
E2E_DEBUG=1 make test-e2e-real
```

Or run the verbose suite with node event tracing:

```bash
make test-e2e-verbose
```

Debug output shows:
- API calls and responses
- Field validation comparisons
- Error messages

### API Timeouts

Check:
- API endpoint being used
- Network connectivity
- API key validity

### Field Validation Failures

Debug logs show expected vs actual values for each field.

## Cleanup

```bash
kubectl delete maintenancewindows,monitors,contacts,accounts --all
make dev-cluster-delete
```
