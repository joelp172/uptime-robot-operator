# Migration Guide: Adopt Existing UptimeRobot Resources

Use this guide to migrate from manually managed UptimeRobot resources to Kubernetes-managed resources with this operator.

## Prerequisites

- Operator installed ([installation guide](installation.md))
- `Account` resource configured and `Ready=true`
- Access to your existing UptimeRobot resources

## What Can Be Adopted

Only one resource type currently supports direct adoption:

| Resource Type | Direct Adoption | Method |
|---|---|---|
| `Monitor` | Yes | `metadata.annotations["uptimerobot.com/adopt-id"]` |
| `MaintenanceWindow` | No | Recreate from spec (no adopt annotation yet) |
| `MonitorGroup` | No | Recreate from spec (no adopt annotation yet) |

## Step 1: Inventory Existing Resources

Find the monitor(s) you want to migrate:

- In UptimeRobot dashboard: open the monitor details and capture the monitor ID
- Or use your existing API/UI workflow to export monitor names, IDs, and types

Record:

- Monitor ID
- Monitor type (`HTTPS`, `Keyword`, `Ping`, `Port`, `Heartbeat`, `DNS`)
- Friendly name and URL/target

## Step 2: Adopt a Monitor

Create a `Monitor` resource with the `uptimerobot.com/adopt-id` annotation:

```yaml
---
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: existing-api-monitor
  annotations:
    uptimerobot.com/adopt-id: "123456789"
spec:
  prune: false
  monitor:
    name: API Health Check
    url: https://api.example.com/health
    type: HTTPS
```

Notes:

- Set `spec.prune: false` during migration to avoid accidental deletion of the upstream monitor.
- The monitor `type` in your spec must match the existing UptimeRobot monitor type.
- On successful adoption, the operator updates the existing monitor to match your spec.

## Step 3: Validate Adoption

Set your target cluster context and confirm reconciliation succeeded:

```bash
export KUBE_CONTEXT=<cluster-context>
kubectl --context="$KUBE_CONTEXT" wait --for=jsonpath='{.status.ready}'=true monitor/existing-api-monitor --timeout=120s
kubectl --context="$KUBE_CONTEXT" get monitor existing-api-monitor -o jsonpath='{.status.ready}{"\n"}'
kubectl --context="$KUBE_CONTEXT" get monitor existing-api-monitor -o jsonpath='{.status.id}{"\n"}'
```

Expected:

- `status.ready` is `true`
- `status.id` matches the adopted monitor ID

Optional checks:

```bash
kubectl --context="$KUBE_CONTEXT" describe monitor existing-api-monitor
kubectl --context="$KUBE_CONTEXT" logs -n uptime-robot-system deployment/uptime-robot-controller-manager
```

## Gotchas

- Type mismatches fail adoption. Example: existing monitor is `Keyword`, spec uses `HTTPS`.
- Spec values are authoritative after adoption. The operator will reconcile drift back to spec.
- `prune: true` (default) deletes the upstream monitor when the CR is deleted.
- If contacts/accounts are not ready, monitor reconciliation can be delayed.

## Rollback (Stop Managing Without Deleting Upstream)

To stop managing an adopted monitor while preserving it in UptimeRobot:

1. Ensure `spec.prune=false`:

```bash
kubectl --context="$KUBE_CONTEXT" patch monitor existing-api-monitor --type merge -p '{"spec":{"prune":false}}'
```

2. Delete the Kubernetes resource:

```bash
kubectl --context="$KUBE_CONTEXT" delete monitor existing-api-monitor
```

Result: the monitor resource is removed from Kubernetes, and the upstream UptimeRobot monitor remains.

## Resource Types Without Adoption (Current State)

`MaintenanceWindow` and `MonitorGroup` do not currently support `adopt-id` style imports. For these:

- Recreate configuration as Kubernetes resources
- Keep `prune: false` while validating migration
- Remove manually managed legacy resources only after you verify parity
