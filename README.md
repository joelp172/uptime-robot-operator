# Uptime Robot Operator

[![Build](https://github.com/joelp172/uptime-robot-operator/actions/workflows/build.yml/badge.svg)](https://github.com/joelp172/uptime-robot-operator/actions/workflows/build.yml)
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
  namespace: uptime-robot-system
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
  namespace: uptime-robot-system
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
  namespace: uptime-robot-system
spec:
  monitor:
    name: My Website
    url: https://example.com
    interval: 5m
EOF
```

## Documentation

| Document | Purpose |
|----------|---------|
| [Installation](docs/installation.md) | Install via kubectl or Helm |
| [Getting Started](docs/getting-started.md) | Create your first monitor (tutorial) |
| [Monitors](docs/monitors.md) | Configure monitor types and alerts |
| [Maintenance Windows](docs/maintenance-windows.md) | Schedule planned downtime |
| [API Reference](docs/api-reference.md) | Complete CRD field reference |
| [Development](docs/development.md) | Contributing and testing |

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

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and PR guidelines.

## License

Apache License 2.0 - see [LICENSE](LICENSE)
