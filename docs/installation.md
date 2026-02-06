# Installation

Install the Uptime Robot Operator on your Kubernetes cluster.

## Prerequisites

- Kubernetes v1.19+
- kubectl configured
- UptimeRobot API key ([get one here](https://uptimerobot.com/?red=joelpi) - Integrations > API)

## Install with kubectl

Apply the latest release manifests:

```bash
kubectl apply -f https://github.com/joelp172/uptime-robot-operator/releases/latest/download/install.yaml
```

Verify the operator is running:

```bash
kubectl get pods -n uptime-robot-system
```

Expected output:

```
NAME                                               READY   STATUS    RESTARTS   AGE
uptime-robot-controller-manager-xxx                1/1     Running   0          30s
```

## Install with Helm

### From OCI Registry

```bash
helm install uptime-robot-operator \
  oci://ghcr.io/joelp172/charts/uptime-robot-operator \
  --version v1.2.1
```

### From Source

```bash
git clone https://github.com/joelp172/uptime-robot-operator.git
cd uptime-robot-operator
helm install uptime-robot-operator ./charts/uptime-robot-operator
```

### Custom Configuration

Create a `values.yaml` file:

```yaml
resources:
  limits:
    cpu: 1
    memory: 512Mi
  requests:
    cpu: 10m
    memory: 64Mi

replicaCount: 2
```

Install with custom values:

```bash
helm install uptime-robot-operator ./charts/uptime-robot-operator -f values.yaml
```

See the [Helm Chart README](../charts/uptime-robot-operator/README.md) for all configuration options.

## Uninstall

### kubectl

```bash
# Delete all resources first
kubectl delete maintenancewindows,monitors,contacts,accounts --all

# Remove operator
kubectl delete -f https://github.com/joelp172/uptime-robot-operator/releases/latest/download/install.yaml
```

### Helm

```bash
# Delete all resources first
kubectl delete maintenancewindows,monitors,contacts,accounts --all

# Uninstall chart
helm uninstall uptime-robot-operator
```

CRDs are preserved to prevent data loss. To remove them:

```bash
kubectl delete crd accounts.uptimerobot.com
kubectl delete crd contacts.uptimerobot.com
kubectl delete crd monitors.uptimerobot.com
kubectl delete crd maintenancewindows.uptimerobot.com
```

## Next Steps

- [Getting Started Tutorial](getting-started.md) - Create your first monitor
- [API Reference](api-reference.md) - CRD field documentation
