# Configure Monitor Groups

Create and manage UptimeRobot monitor groups using the `MonitorGroup` custom resource.

## Prerequisites

- You have installed the operator.
- You have an `Account` resource that is `Ready`.
- You have one or more `Monitor` resources in the same namespace.

## How monitor groups work

- `MonitorGroup` is a namespaced resource.
- `spec.monitors` references `Monitor` resources by name in the same namespace.
- The controller resolves each referenced monitor from `status.id`.
- If a monitor is missing or not ready, the controller skips it until a later reconcile.

## Create a monitor group from monitor references

This example groups two monitors named `api-health` and `web-frontend`.

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: MonitorGroup
metadata:
  name: production-services
  namespace: uptime-robot-system
spec:
  account:
    name: default
  friendlyName: production-services
  prune: true
  syncInterval: 24h
  monitors:
    - name: api-health
    - name: web-frontend
```

Apply:

```bash
kubectl apply -f monitorgroup-production-services.yaml
```

Verify:

```bash
kubectl get monitorgroup production-services -n uptime-robot-system
kubectl get monitorgroup production-services -n uptime-robot-system -o jsonpath='{.status.ready}{"\t"}{.status.id}{"\t"}{.status.monitorCount}{"\n"}'
```

## Pull monitors from existing group IDs

Use `pullFromGroups` when you want this group to include monitors from existing UptimeRobot groups.

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: MonitorGroup
metadata:
  name: shared-services
  namespace: uptime-robot-system
spec:
  account:
    name: default
  friendlyName: shared-services
  prune: true
  syncInterval: 24h
  pullFromGroups:
    - 12345
    - 67890
```

Apply:

```bash
kubectl apply -f monitorgroup-shared-services.yaml
```

## Combine both approaches

You can use both fields in one resource:

- `monitors` for explicit monitor references from Kubernetes.
- `pullFromGroups` for existing UptimeRobot group IDs.

## Monitor-level `groupId` vs `MonitorGroup`

You can also set `spec.monitor.groupId` on a `Monitor`.  
Use one primary approach per use case to avoid confusion:

- Use `MonitorGroup` when you want group membership managed centrally.
- Use monitor-level `groupId` when group assignment is specific to each monitor resource.

## Troubleshooting

- Group is not ready:
  Check `kubectl describe monitorgroup <name> -n <namespace>` for account/API errors.
- `monitorCount` is lower than expected:
  Verify referenced monitors exist, are ready, and have `status.id` populated.
- Group not updated after monitor changes:
  Confirm monitors are in the same namespace as the `MonitorGroup`.

