# Uptime Robot Operator

[![Build](https://github.com/joelp172/uptime-robot-operator/actions/workflows/build.yml/badge.svg)](https://github.com/joelp172/uptime-robot-operator/actions/workflows/build.yml)

A Kubernetes operator that manages [UptimeRobot](https://uptimerobot.com/?red=joelpi) monitors declaratively using Custom Resources. Monitors are automatically reconciled to prevent configuration drift.

## Features

- Declarative monitor management via Kubernetes CRDs
- Automatic drift detection and correction
- Support for all UptimeRobot monitor types: HTTPS, Keyword, Ping, Port, Heartbeat, DNS
- Alert contact configuration
- Garbage collection of deleted monitors

## Quick Start

### Prerequisites

- Kubernetes cluster v1.19+
- kubectl configured to access your cluster
- UptimeRobot API key ([sign up free](https://uptimerobot.com/?red=joelpi) then go to Integrations > API)

### Install

```bash
kubectl apply -f https://raw.githubusercontent.com/joelp172/uptime-robot-operator/main/dist/install.yaml
```

### Configure API Key

Create a Secret in the `uptime-robot-system` namespace (where the operator runs):

```bash
kubectl create secret generic uptimerobot-api-key \
  --namespace uptime-robot-system \
  --from-literal=apiKey=YOUR_API_KEY
```

### Create Account

Account and Contact are cluster-scoped resources (no namespace required). The Secret they reference must be in `uptime-robot-system`.

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: Account
metadata:
  name: default
spec:
  isDefault: true
  apiKeySecretRef:
    name: uptimerobot-api-key
    key: apiKey
```

### Create Contact

First, find your alert contact ID from the Account status:

```bash
kubectl get account default -o jsonpath='{.status.alertContacts[0].id}'
```

Then create a Contact resource referencing it:

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: Contact
metadata:
  name: default
spec:
  isDefault: true
  contact:
    id: "YOUR_CONTACT_ID"
```

### Create Monitor

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: my-website
spec:
  monitor:
    name: My Website
    url: https://example.com
    interval: 5m
```

## Documentation

- [Getting Started Guide](docs/getting-started.md) - Full installation and configuration tutorial
- [API Reference](docs/api-reference.md) - Complete CRD specification
- [How to Configure Alerts](docs/configure-alerts.md) - Set up alert contacts and notifications

## Monitor Types

| Type | Description |
|------|-------------|
| HTTPS | HTTP/HTTPS endpoint monitoring |
| Keyword | Check for specific text in page content |
| Ping | ICMP ping monitoring |
| Port | TCP port monitoring |
| Heartbeat | Expects periodic pings from your services |
| DNS | DNS record verification |

## How It Works

The operator watches for Monitor custom resources and creates corresponding monitors in UptimeRobot via the API. It periodically reconciles the state to detect and correct drift (changes made outside Kubernetes).

When you delete a Monitor resource with `prune: true` (the default), the operator automatically deletes the corresponding monitor in UptimeRobot.

## Contributing

Contributions are welcome. Please open an issue or submit a pull request.

## License

Apache License 2.0. See [LICENSE](LICENSE) for details.
