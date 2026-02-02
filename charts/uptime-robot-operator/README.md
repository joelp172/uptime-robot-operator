# Uptime Robot Operator Helm Chart

This Helm chart deploys the Uptime Robot Operator on a Kubernetes cluster using the Helm package manager.

## Prerequisites

- Kubernetes 1.19+
- Helm 3.0+
- UptimeRobot API key ([sign up free](https://uptimerobot.com/?red=joelpi) then go to Integrations > API)

## Installing the Chart

### From source

To install the chart with the release name `uptime-robot-operator` from the source repository:

```bash
git clone https://github.com/joelp172/uptime-robot-operator.git
cd uptime-robot-operator
helm install uptime-robot-operator ./charts/uptime-robot-operator
```

### From OCI registry

To install the chart from the OCI registry (requires Helm 3.8+):

```bash
# Replace <VERSION> with the desired chart version (e.g., v1.0.0)
helm install uptime-robot-operator oci://ghcr.io/joelp172/charts/uptime-robot-operator --version <VERSION>
```

To see available versions:

```bash
helm show chart oci://ghcr.io/joelp172/charts/uptime-robot-operator
```

The command deploys the Uptime Robot Operator on the Kubernetes cluster with default configuration. The [Parameters](#parameters) section lists the parameters that can be configured during installation.

## Uninstalling the Chart

To uninstall/delete the `uptime-robot-operator` deployment:

```bash
helm uninstall uptime-robot-operator
```

The command removes all the Kubernetes components associated with the chart. Note that CRDs are not removed by default to prevent data loss.

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

### Other Parameters

| Name                              | Description                                      | Value            |
|-----------------------------------|--------------------------------------------------|------------------|
| `nameOverride`                    | Override the name of the chart                   | `""`             |
| `fullnameOverride`                | Override the full name of the chart              | `""`             |

## Configuration Examples

### Custom Namespace

```bash
helm install uptime-robot-operator ./charts/uptime-robot-operator \
  --set namespaceOverride=my-namespace
```

### Custom Image Tag

```bash
helm install uptime-robot-operator ./charts/uptime-robot-operator \
  --set image.tag=v1.0.0
```

### Custom Resource Limits

```bash
helm install uptime-robot-operator ./charts/uptime-robot-operator \
  --set resources.limits.cpu=2 \
  --set resources.limits.memory=1Gi \
  --set resources.requests.cpu=100m \
  --set resources.requests.memory=128Mi
```

### Multiple Replicas (for High Availability)

```bash
helm install uptime-robot-operator ./charts/uptime-robot-operator \
  --set replicaCount=2
```

Note: When running multiple replicas, leader election is automatically enabled to ensure only one instance is active at a time.

### Using Values File

Create a `custom-values.yaml` file:

```yaml
image:
  tag: v1.0.0

resources:
  limits:
    cpu: 2
    memory: 1Gi
  requests:
    cpu: 100m
    memory: 128Mi

replicaCount: 2

nodeSelector:
  node-role.kubernetes.io/worker: "true"
```

Install the chart with custom values:

```bash
helm install uptime-robot-operator ./charts/uptime-robot-operator \
  -f custom-values.yaml
```

## Post-Installation Steps

After installing the chart, follow these steps to start monitoring your services:

1. **Create API Key Secret**

   ```bash
   kubectl create secret generic uptimerobot-api-key \
     --namespace uptime-robot-system \
     --from-literal=apiKey=YOUR_API_KEY
   ```

2. **Create Account Resource**

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

3. **Create Contact Resource**

   First, get your contact ID:
   ```bash
   kubectl get account default -o jsonpath='{.status.alertContacts[0].id}'
   ```

   Then create the Contact:
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

4. **Create Monitor Resource**

   ```yaml
   apiVersion: uptimerobot.com/v1alpha1
   kind: Monitor
   metadata:
     name: my-website
     namespace: default
   spec:
     monitor:
       name: My Website
       url: https://example.com
       interval: 5m
   ```

## Upgrading the Chart

To upgrade the `uptime-robot-operator` deployment:

```bash
helm upgrade uptime-robot-operator ./charts/uptime-robot-operator
```

## CRD Management

By default, the chart installs CRDs and preserves them when the chart is uninstalled. This prevents accidental data loss.

To completely remove CRDs (this will delete all Account, Contact, and Monitor resources):

```bash
kubectl delete crd accounts.uptimerobot.com
kubectl delete crd contacts.uptimerobot.com
kubectl delete crd monitors.uptimerobot.com
```

## Troubleshooting

### Verify Installation

```bash
kubectl get pods -n uptime-robot-system
kubectl get crd | grep uptimerobot.com
```

## More Information

- [Project Documentation](https://github.com/joelp172/uptime-robot-operator/tree/main/docs)
- [GitHub Repository](https://github.com/joelp172/uptime-robot-operator)
- [Issue Tracker](https://github.com/joelp172/uptime-robot-operator/issues)

## Licence

Apache Licence 2.0 - See [LICENCE](https://github.com/joelp172/uptime-robot-operator/blob/main/LICENSE) for details.
