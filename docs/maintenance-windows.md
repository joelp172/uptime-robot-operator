# Maintenance Windows

Schedule planned downtime to prevent false alerts during deployments or maintenance.

## Quick Example

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: MaintenanceWindow
metadata:
  name: weekly-deployment
spec:
  name: "Weekly Deployment Window"
  interval: weekly
  startDate: "2026-02-10"
  startTime: "02:00:00"
  duration: 1h
  days: [2, 4]  # Tuesday and Thursday
  monitorRefs:
    - name: my-website
```

## Interval Types

### One-Time

Single maintenance window:

```yaml
spec:
  name: "Emergency Maintenance"
  interval: once
  startDate: "2026-03-15"
  startTime: "03:00:00"
  duration: 2h
  autoAddMonitors: true
```

### Daily

Runs every day:

```yaml
spec:
  name: "Daily Backup Window"
  interval: daily
  startDate: "2026-02-01"
  startTime: "02:00:00"
  duration: 30m
  monitorRefs:
    - name: database
```

### Weekly

Specific days of the week:

```yaml
spec:
  name: "Weekly Deployment"
  interval: weekly
  startDate: "2026-02-10"
  startTime: "02:00:00"
  duration: 1h
  days: [2, 4]  # Tuesday (2) and Thursday (4)
  monitorRefs:
    - name: api
    - name: frontend
```

Days: 0=Sunday, 1=Monday, 2=Tuesday, 3=Wednesday, 4=Thursday, 5=Friday, 6=Saturday

### Monthly

Specific days of the month:

```yaml
spec:
  name: "Monthly Maintenance"
  interval: monthly
  startDate: "2026-02-01"
  startTime: "05:00:00"
  duration: 4h
  days: [1, 15, -1]  # 1st, 15th, and last day
  monitorRefs:
    - name: my-website
```

Days: 1-31 for specific days, -1 for last day of month

## Monitor Selection

### Auto-Add All Monitors

```yaml
spec:
  name: "Emergency Maintenance"
  interval: once
  startDate: "2026-03-15"
  startTime: "03:00:00"
  duration: 2h
  autoAddMonitors: true
```

### Specific Monitors

```yaml
spec:
  name: "API Maintenance"
  interval: weekly
  startDate: "2026-02-10"
  startTime: "02:00:00"
  duration: 1h
  days: [0]  # Sunday
  monitorRefs:
    - name: api-health
    - name: api-metrics
```

Monitors must be in the same namespace as the MaintenanceWindow.

## Duration Format

Use Go duration format:

| Format | Duration |
|--------|----------|
| `30m` | 30 minutes |
| `1h` | 1 hour |
| `2h30m` | 2 hours 30 minutes |
| `24h` | 24 hours |

## Common Patterns

### Weekend Maintenance

```yaml
spec:
  name: "Weekend Maintenance"
  interval: weekly
  startDate: "2026-02-08"
  startTime: "00:00:00"
  duration: 48h
  days: [6, 0]  # Saturday and Sunday
  autoAddMonitors: true
```

### Business Hours Deployment

```yaml
spec:
  name: "Weekday Deployments"
  interval: weekly
  startDate: "2026-02-10"
  startTime: "14:00:00"
  duration: 2h
  days: [1, 2, 3, 4, 5]  # Monday-Friday
  monitorRefs:
    - name: production-api
```

### End of Month Maintenance

```yaml
spec:
  name: "Month-End Processing"
  interval: monthly
  startDate: "2026-02-01"
  startTime: "22:00:00"
  duration: 6h
  days: [-1]  # Last day of month
  monitorRefs:
    - name: billing-system
```

## Prune Behaviour

Control whether maintenance windows are deleted from UptimeRobot when the resource is deleted:

```yaml
spec:
  prune: true  # Default: delete from UptimeRobot when CR is deleted
  name: "Temporary Maintenance"
  interval: once
  startDate: "2026-03-15"
  startTime: "03:00:00"
  duration: 2h
```

Set `prune: false` to keep the maintenance window in UptimeRobot after deleting the Kubernetes resource.

## Verification

Check maintenance window status:

```bash
kubectl get maintenancewindows
```

View details:

```bash
kubectl describe maintenancewindow weekly-deployment
```

Check which monitors are assigned:

```bash
kubectl get maintenancewindow weekly-deployment -o jsonpath='{.status.monitorCount}'
```

## Troubleshooting

### Maintenance Window Not Ready

Check the status:

```bash
kubectl describe maintenancewindow weekly-deployment
```

Common issues:
- Invalid date format (must be YYYY-MM-DD)
- Invalid time format (must be HH:mm:ss)
- Missing `days` field for weekly/monthly intervals
- Referenced monitors don't exist

### Monitors Not Assigned

Verify monitors exist in the same namespace:

```bash
kubectl get monitors -n <namespace>
```

Check operator logs:

```bash
kubectl logs -n uptime-robot-system deployment/uptime-robot-controller-manager
```

## Next Steps

- [Monitor Configuration](monitors.md) - Configure different monitor types
- [API Reference](api-reference.md) - Complete field reference
