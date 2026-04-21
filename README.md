# Uptime Robot Operator

[![Build](https://github.com/joelp172/uptime-robot-operator/actions/workflows/build.yml/badge.svg)](https://github.com/joelp172/uptime-robot-operator/actions/workflows/build.yml)
[![codecov](https://codecov.io/gh/joelp172/uptime-robot-operator/branch/main/graph/badge.svg)](https://codecov.io/gh/joelp172/uptime-robot-operator)
[![Release](https://img.shields.io/github/v/release/joelp172/uptime-robot-operator)](https://github.com/joelp172/uptime-robot-operator/releases/latest)
[![License](https://img.shields.io/github/license/joelp172/uptime-robot-operator)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/joelp172/uptime-robot-operator)](https://goreportcard.com/report/github.com/joelp172/uptime-robot-operator)

Manage [UptimeRobot](https://uptimerobot.com/?red=joelpi) monitors as Kubernetes resources. Automatic drift detection, self-healing, and GitOps-ready.

## Features

- Declarative monitor configuration via CRDs
- Drift detection and automatic correction
- All monitor types: HTTPS, Keyword, Ping, Port, Heartbeat, DNS
- Maintenance window scheduling
- Alert contact management
- **Adopt existing monitors** - Migrate monitors created outside Kubernetes without losing history
- **Prometheus metrics** - API performance, reconciliation duration, error tracking

## Security

All images are:
- **Signed with Cosign** — Keyless signing via GitHub Actions OpenID Connect (OIDC)
- **Scanned for vulnerabilities** — Trivy scanning; critical/high severity blocks the build
- **SBOM included** — Software Bill of Materials (SBOM) in SPDX and CycloneDX formats

See [SECURITY.md](SECURITY.md) for verification instructions and deployment best practices.

## Quick Start

Install the operator:

```bash
kubectl apply -f https://github.com/joelp172/uptime-robot-operator/releases/latest/download/install.yaml
```

Create your first monitor:

```bash
# Store your API key
kubectl create secret generic uptimerobot-api-key \
  --namespace uptime-robot-system \
  --from-literal=apiKey=YOUR_API_KEY

# Configure account
kubectl apply -f - <<EOF
apiVersion: uptimerobot.com/v1alpha1
kind: Account
metadata:
  name: default
spec:
  isDefault: true
  apiKeySecretRef:
    name: uptimerobot-api-key
    key: apiKey
EOF

# Get your contact ID
kubectl get account default -o jsonpath='{.status.alertContacts[0].id}'

# Create contact (replace YOUR_CONTACT_ID)
kubectl apply -f - <<EOF
apiVersion: uptimerobot.com/v1alpha1
kind: Contact
metadata:
  name: default
spec:
  isDefault: true
  contact:
    id: "YOUR_CONTACT_ID"
EOF

# Create monitor
kubectl apply -f - <<EOF
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: my-website
spec:
  monitor:
    name: My Website
    url: https://example.com
    interval: 5m
EOF
```

## Documentation

### User Documentation

Guides focused on day-to-day usage and operations.

| Document | Purpose |
|----------|---------|
| [Installation](docs/installation.md) | Install via kubectl or Helm |
| [Getting Started](docs/getting-started.md) | Create your first monitor (tutorial) |
| [Monitors](docs/monitors.md) | Configure monitor types and alerts |
| [Monitor Groups](docs/monitor-groups.md) | Group monitors and manage membership declaratively |
| [Maintenance Windows](docs/maintenance-windows.md) | Schedule planned downtime |
| [Slack Alerting](docs/slack-alerting.md) | End-to-end Slack alert routing for monitors |
| [Migration Guide](docs/migration-guide.md) | Adopt existing UptimeRobot resources |
| [Troubleshooting](docs/troubleshooting.md) | Diagnose and fix common issues |

#### CRD Guides

Explicit documentation for each CRD type:

| CRD | Guide |
|-----|-------|
| `Account` | [Configure Accounts](docs/accounts.md) |
| `Contact` | [Configure Contacts](docs/contacts.md) |
| `Monitor` | [Configure Monitors](docs/monitors.md) |
| `MonitorGroup` | [Configure Monitor Groups](docs/monitor-groups.md) |
| `MaintenanceWindow` | [Maintenance Windows](docs/maintenance-windows.md) |
| `SlackIntegration` | [Configure Slack Integrations](docs/slack-integrations.md) |

### Technical Documentation

Design, API, and contributor-focused documentation.

| Document | Purpose |
|----------|---------|
| [Architecture](docs/architecture.md) | System architecture and data flows |
| [Metrics](docs/metrics.md) | Prometheus metrics and Grafana dashboards |
| [API Versioning](docs/api-versioning.md) | API stability, deprecation, and upgrade policy |
| [API Reference](docs/api-reference.md) | Complete CRD field reference |
| [Development](docs/development.md) | Local development and testing |
| [Security](SECURITY.md) | Image verification and deployment security practices |

## Argo CD health checks for CRDs

Argo CD treats unknown CRDs as `Healthy` by default. To reflect `Contact` and `Monitor` reconciliation state in Argo CD health, add the following to `argocd-cm` (`data` section). The two snippets below are intentionally identical — `argocd-cm` requires a separate `resource.customizations.health.<group>_<kind>` key per CRD, so the same logic is repeated once per kind:

```yaml
resource.customizations.health.uptimerobot.com_Contact: |
  hs = {}
  if obj.status == nil or obj.status.conditions == nil then
    hs.status = "Progressing"
    hs.message = "Waiting for reconciliation"
    return hs
  end

  local ready = nil
  local synced = nil
  for _, c in ipairs(obj.status.conditions) do
    if c.type == "Ready" then ready = c end
    if c.type == "Synced" then synced = c end
  end

  if ready ~= nil and ready.status == "False" then
    hs.status = "Degraded"
    hs.message = ready.message or "Ready=False"
    return hs
  end

  if synced ~= nil and synced.status == "False" then
    hs.status = "Degraded"
    hs.message = synced.message or "Synced=False"
    return hs
  end

  if ready ~= nil and ready.status == "True" then
    hs.status = "Healthy"
    hs.message = ready.message or "Ready=True"
    return hs
  end

  hs.status = "Progressing"
  hs.message = "Waiting for reconciliation"
  return hs

resource.customizations.health.uptimerobot.com_Monitor: |
  hs = {}
  if obj.status == nil or obj.status.conditions == nil then
    hs.status = "Progressing"
    hs.message = "Waiting for reconciliation"
    return hs
  end

  local ready = nil
  local synced = nil
  for _, c in ipairs(obj.status.conditions) do
    if c.type == "Ready" then ready = c end
    if c.type == "Synced" then synced = c end
  end

  if ready ~= nil and ready.status == "False" then
    hs.status = "Degraded"
    hs.message = ready.message or "Ready=False"
    return hs
  end

  if synced ~= nil and synced.status == "False" then
    hs.status = "Degraded"
    hs.message = synced.message or "Synced=False"
    return hs
  end

  if ready ~= nil and ready.status == "True" then
    hs.status = "Healthy"
    hs.message = ready.message or "Ready=True"
    return hs
  end

  hs.status = "Progressing"
  hs.message = "Waiting for reconciliation"
  return hs
```

## Monitor Types

| Type | Use Case |
|------|----------|
| HTTPS | HTTP/HTTPS endpoints |
| Keyword | Page content verification |
| Ping | ICMP availability |
| Port | TCP port connectivity |
| Heartbeat | Cron jobs and scheduled tasks |
| DNS | DNS record validation |

## How It Works

The operator reconciles Monitor resources with UptimeRobot via the API. It detects drift (manual changes in UptimeRobot) and corrects them to match your Kubernetes configuration. When you delete a Monitor resource, the operator removes it from UptimeRobot (configurable via `prune` field).

## Observability

The operator exposes custom Prometheus metrics for monitoring API performance, reconciliation behavior, and resource health:

- **API Metrics**: Request rate, latency percentiles, error rate, retry patterns
- **Reconciliation Metrics**: Duration, error rate by controller and error type
- **Resource Metrics**: Monitor counts by type/status, maintenance windows, monitor groups

See [docs/metrics.md](docs/metrics.md) for complete documentation and a sample Grafana dashboard.

```bash
# Enable metrics endpoint (chart/manifests default to :8443; :8080 is also valid)
kubectl patch deployment -n uptime-robot-system uptime-robot-controller-manager \
  --type=json -p='[{"op": "add", "path": "/spec/template/spec/containers/0/args/-", "value": "--metrics-bind-address=:8443"}]'

# Access metrics
kubectl port-forward -n uptime-robot-system deployment/uptime-robot-controller-manager 8443:8443
curl http://localhost:8443/metrics | grep uptimerobot_
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and PR guidelines.

## License

Apache License 2.0 - see [LICENSE](LICENSE)
