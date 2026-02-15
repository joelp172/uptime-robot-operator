# Installation

Install the Uptime Robot Operator on your Kubernetes cluster.

## Prerequisites

- Kubernetes v1.19+
- kubectl configured
- Helm v3 (required to install cert-manager)
- cert-manager CRDs/controllers installed
- UptimeRobot API key ([get one here](https://uptimerobot.com/?red=joelpi) - Integrations > API)

Install cert-manager before installing the operator:

```bash
helm repo add jetstack https://charts.jetstack.io
helm repo update
helm upgrade --install cert-manager jetstack/cert-manager \
  --namespace cert-manager \
  --create-namespace \
  --set crds.enabled=true
kubectl wait --namespace cert-manager \
  --for=condition=Available deployment \
  cert-manager cert-manager-cainjector cert-manager-webhook \
  --timeout=180s
```

## Install with kubectl

Apply the latest release manifests:

```bash
kubectl apply -f https://github.com/joelp172/uptime-robot-operator/releases/latest/download/install.yaml
```

### Verify image signature (optional)

**Prerequisites:** [Cosign](https://docs.sigstore.dev/cosign/installation) installed.

Verify the image referenced by the install manifest before deployment:

```bash
curl -L -o install.yaml https://github.com/joelp172/uptime-robot-operator/releases/latest/download/install.yaml
IMAGE_REF=$(grep "image: ghcr.io/joelp172/uptime-robot-operator" install.yaml | head -n1 | awk '{print $2}')
cosign verify \
  --certificate-identity-regexp="^https://github.com/joelp172/uptime-robot-operator/" \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com" \
  "${IMAGE_REF}"
```

See [SECURITY.md](../SECURITY.md) for verification details and deployment best practices.

### Verify the operator is running

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
# cert-manager is required for webhook certificates
helm repo add jetstack https://charts.jetstack.io
helm repo update
helm upgrade --install cert-manager jetstack/cert-manager \
  --namespace cert-manager \
  --create-namespace \
  --set crds.enabled=true

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
# Delete all dependent resources first
kubectl delete maintenancewindows,monitors,monitorgroups,slackintegrations --all

# Delete remaining resources with account last
kubectl delete contacts,accounts --all

# Remove operator
kubectl delete -f https://github.com/joelp172/uptime-robot-operator/releases/latest/download/install.yaml
```

### Helm

```bash
# Delete all dependent resources first
kubectl delete maintenancewindows,monitors,monitorgroups,slackintegrations --all

# Delete remaining resources with account last
kubectl delete contacts,accounts --all

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
