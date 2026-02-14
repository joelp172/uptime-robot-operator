# Uptime Robot Operator Helm Chart

Deploy the Uptime Robot Operator using Helm.

## Prerequisites

- Kubernetes 1.19+
- Helm 3.0+

## Install

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

## Uninstall

```bash
helm uninstall uptime-robot-operator
```

CRDs are preserved to prevent data loss. To remove them:

```bash
kubectl delete crd accounts.uptimerobot.com contacts.uptimerobot.com monitors.uptimerobot.com maintenancewindows.uptimerobot.com
```

## Parameters

### Global Parameters

| Name                      | Description                                      | Value                                            |
|---------------------------|--------------------------------------------------|--------------------------------------------------|
| `replicaCount`            | Number of operator replicas                      | `1`                                              |
| `namespaceOverride`       | Override the namespace for installation          | `""`                                             |
| `namespace.create`        | Create the namespace as part of the chart        | `true`                                           |

### Image Parameters

| Name                      | Description                                      | Value                                            |
|---------------------------|--------------------------------------------------|--------------------------------------------------|
| `image.repository`        | Container image repository                       | `ghcr.io/joelp172/uptime-robot-operator`        |
| `image.pullPolicy`        | Image pull policy                                | `IfNotPresent`                                   |
| `image.tag`               | Overrides the image tag (defaults to chart appVersion) | `""`                                      |
| `imagePullSecrets`        | Image pull secrets                               | `[]`                                             |

### ServiceAccount Parameters

| Name                              | Description                                      | Value   |
|-----------------------------------|--------------------------------------------------|---------|
| `serviceAccount.create`           | Specifies whether a service account should be created | `true` |
| `serviceAccount.automount`        | Automatically mount service account credentials  | `true`  |
| `serviceAccount.annotations`      | Annotations to add to the service account        | `{}`    |
| `serviceAccount.name`             | The name of the service account to use           | `""`    |

### Security Parameters

| Name                              | Description                                      | Value            |
|-----------------------------------|--------------------------------------------------|------------------|
| `podSecurityContext.runAsNonRoot` | Run containers as non-root user                  | `true`           |
| `podSecurityContext.seccompProfile.type` | Seccomp profile type                    | `RuntimeDefault` |
| `securityContext.allowPrivilegeEscalation` | Allow privilege escalation         | `false`          |
| `securityContext.capabilities.drop` | Linux capabilities to drop                 | `["ALL"]`        |
| `securityContext.readOnlyRootFilesystem` | Mount root filesystem as read-only   | `true`           |

### Resource Parameters

| Name                              | Description                                      | Value            |
|-----------------------------------|--------------------------------------------------|------------------|
| `resources`                       | Resource limits and requests                     | `{}`             |

Resources are not set by default. Configure as needed for your environment:

```yaml
resources:
  limits:
    cpu: "1"
    memory: 512Mi
  requests:
    cpu: 10m
    memory: 64Mi
```

### Probe Parameters

| Name                                          | Description                        | Value        |
|-----------------------------------------------|-----------------------------------|--------------|
| `livenessProbe.httpGet.path`                  | Liveness probe path               | `/healthz`   |
| `livenessProbe.httpGet.port`                  | Liveness probe port               | `8081`       |
| `livenessProbe.initialDelaySeconds`           | Initial delay for liveness probe  | `15`         |
| `livenessProbe.periodSeconds`                 | Period for liveness probe         | `20`         |
| `readinessProbe.httpGet.path`                 | Readiness probe path              | `/readyz`    |
| `readinessProbe.httpGet.port`                 | Readiness probe port              | `8081`       |
| `readinessProbe.initialDelaySeconds`          | Initial delay for readiness probe | `5`          |
| `readinessProbe.periodSeconds`                | Period for readiness probe        | `10`         |

### Operator Configuration

| Name                              | Description                                      | Value            |
|-----------------------------------|--------------------------------------------------|------------------|
| `leaderElection.enabled`          | Enable leader election                           | `true`           |
| `healthProbeBindAddress`          | Health probe bind address                        | `:8081`          |

### Metrics Parameters

| Name                              | Description                                      | Value            |
|-----------------------------------|--------------------------------------------------|------------------|
| `metrics.enabled`                 | Enable metrics service                           | `true`           |
| `metrics.port`                    | Metrics service port                             | `8443`           |
| `metrics.type`                    | Metrics service type                             | `ClusterIP`      |

### CRDs

CRDs are managed using Helm's `crds/` directory mechanism. This means:

- CRDs are automatically installed on first `helm install`
- CRDs are **never** deleted on `helm uninstall` (to prevent data loss)
- CRDs are **not** upgraded on `helm upgrade` (Helm limitation)

To upgrade CRDs manually:

```bash
kubectl apply -f https://github.com/joelp172/uptime-robot-operator/releases/latest/download/install.yaml
```

Or extract from the chart:

```bash
kubectl apply -f charts/uptime-robot-operator/crds/
```

### Pod Assignment Parameters

| Name                              | Description                                      | Value            |
|-----------------------------------|--------------------------------------------------|------------------|
| `nodeSelector`                    | Node labels for pod assignment                   | `{}`             |
| `tolerations`                     | Tolerations for pod assignment                   | `[]`             |
| `affinity`                        | Affinity for pod assignment                      | `{}`             |
| `podAnnotations`                  | Pod annotations                                  | `{kubectl.kubernetes.io/default-container: manager}` |

### Network Policy Parameters

| Name                              | Description                                      | Value            |
|-----------------------------------|--------------------------------------------------|------------------|
| `networkPolicy.enabled`           | Enable NetworkPolicy for the operator            | `false`          |
| `networkPolicy.ingress`           | Ingress rules for the NetworkPolicy              | See values.yaml  |
| `networkPolicy.egress`            | Egress rules for the NetworkPolicy               | See values.yaml  |

The NetworkPolicy is disabled by default. When enabled, it restricts network traffic to and from the operator pod:

**Ingress (allowed):**
- Port 8081 (health probes)
- Port 8443 (metrics)
- Port 9443 (webhooks)

**Egress (allowed):**
- Port 443 (HTTPS for UptimeRobot API and Kubernetes API)
- Port 53 (DNS, both TCP and UDP)

**All other traffic is denied** when the NetworkPolicy is enabled.

**Important:** If you customize the ingress or egress rules and provide an empty list, all traffic in that direction will be blocked. This is intentional for security-by-default behavior.

To enable the NetworkPolicy:

```bash
helm install uptime-robot-operator ./charts/uptime-robot-operator \
  --set networkPolicy.enabled=true
```

You can customize the ingress and egress rules by providing a custom values file:

```yaml
networkPolicy:
  enabled: true
  ingress:
    - from:
      - namespaceSelector:
          matchLabels:
            monitoring: enabled
      ports:
        - port: 8443
          protocol: TCP
  egress:
    - to:
      - namespaceSelector: {}
      ports:
        - port: 443
          protocol: TCP
```

### Other Parameters

| Name                              | Description                                      | Value            |
|-----------------------------------|--------------------------------------------------|------------------|
| `nameOverride`                    | Override the name of the chart                   | `""`             |
| `fullnameOverride`                | Override the full name of the chart              | `""`             |

## Configuration

### Custom Values

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

nodeSelector:
  node-role.kubernetes.io/worker: "true"
```

Install with custom values:

```bash
helm install uptime-robot-operator ./charts/uptime-robot-operator -f values.yaml
```

### Common Overrides

```bash
# Custom namespace
helm install uptime-robot-operator ./charts/uptime-robot-operator \
  --set namespaceOverride=my-namespace

# Custom image tag
helm install uptime-robot-operator ./charts/uptime-robot-operator \
  --set image.tag=v1.0.0

# High availability (2 replicas)
helm install uptime-robot-operator ./charts/uptime-robot-operator \
  --set replicaCount=2
```

## Upgrade

```bash
helm upgrade uptime-robot-operator ./charts/uptime-robot-operator
```

**Note:** CRDs are not upgraded automatically. To upgrade CRDs:

```bash
kubectl apply -f https://github.com/joelp172/uptime-robot-operator/releases/latest/download/install.yaml
```

## Next Steps

After installation, see the [Getting Started Guide](https://github.com/joelp172/uptime-robot-operator/blob/main/docs/getting-started.md) to create your first monitor.

## More Information

- [Documentation](https://github.com/joelp172/uptime-robot-operator/tree/main/docs)
- [GitHub Repository](https://github.com/joelp172/uptime-robot-operator)
- [Issue Tracker](https://github.com/joelp172/uptime-robot-operator/issues)
