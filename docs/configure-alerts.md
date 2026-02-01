# Configure Alert Contacts

This guide explains how to set up alert notifications for your monitors.

## Prerequisites

- Operator installed and running
- Account resource configured and ready
- Alert contacts already created in UptimeRobot dashboard

## Overview

The operator references existing alert contacts in your UptimeRobot account. It does not create new contacts. You must first create contacts (email, SMS, mobile app, etc.) in [UptimeRobot](https://uptimerobot.com/?red=joelpi) (Dashboard > Integrations), then reference them in Kubernetes.

## Step 1: Find Your Contact IDs

### Option A: From Account Status

The Account resource status lists all available contacts:

```bash
kubectl get account default -o jsonpath='{range .status.alertContacts[*]}{.id}{"\t"}{.type}{"\t"}{.friendlyName}{"\t"}{.value}{"\n"}{end}'
```

Example output:

```
1234567   Email              your@email.com
7654321   MobileApp   iPhone   abc123...
```

### Option B: From UptimeRobot API

```bash
curl -H "Authorization: Bearer YOUR_API_KEY" \
  https://api.uptimerobot.com/v3/user/alert-contacts
```

## Step 2: Create Contact Resource

Create a Contact resource that references your alert contact.

### Reference by ID (Recommended)

Use the contact ID when the contact has no friendlyName, or for reliability:

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: Contact
metadata:
  name: my-email
spec:
  isDefault: true
  contact:
    id: "1234567"
```

### Reference by friendlyName

Use friendlyName if it's set in UptimeRobot:

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: Contact
metadata:
  name: my-phone
spec:
  contact:
    name: "iPhone"
```

Apply the contact:

```bash
kubectl apply -f contact.yaml
```

Verify it's ready:

```bash
kubectl get contacts
```

```
NAME        READY   DEFAULT   FRIENDLY NAME   AGE
my-email    true    true                      10s
my-phone    true    false     iPhone          10s
```

## Step 3: Assign Contacts to Monitors

Reference Contact resources in your Monitor spec:

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: production-api
spec:
  contacts:
    - name: my-email
      threshold: 5m      # Wait 5 minutes before alerting
      recurrence: 30m    # Repeat alert every 30 minutes
    - name: my-phone
      threshold: 10m     # Alert phone after 10 minutes
  monitor:
    name: Production API
    url: https://api.example.com/health
    interval: 1m
```

### Contact Options

| Field | Description | Default |
|-------|-------------|---------|
| `name` | Name of the Contact resource | Required |
| `threshold` | Time to wait before sending first alert | 1m |
| `recurrence` | Interval for repeat alerts (0 = no repeat) | 0 |

## Using Default Contacts

Set `isDefault: true` on a Contact to use it automatically for monitors that don't specify contacts:

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: Contact
metadata:
  name: default-email
spec:
  isDefault: true
  contact:
    id: "1234567"
```

Monitors without a `contacts` field will use this contact.

## Multiple Contacts

Assign multiple contacts with different notification timing:

```yaml
spec:
  contacts:
    # Immediate alert to on-call
    - name: oncall-phone
      threshold: 1m
    # Email after 5 minutes if still down
    - name: team-email
      threshold: 5m
      recurrence: 1h
    # Escalate to manager after 15 minutes
    - name: manager-phone
      threshold: 15m
```

## Troubleshooting

### Contact Not Ready

If a Contact shows `READY: false`:

1. Verify the contact exists in UptimeRobot dashboard
2. Check that the ID or friendlyName matches exactly
3. Check operator logs:

```bash
kubectl logs -n uptime-robot-system deployment/uptime-robot-controller-manager
```

### Monitor Not Alerting

1. Verify the Contact resource is ready
2. Check that the contact is correctly referenced in the Monitor
3. Verify notification settings in UptimeRobot dashboard (some contacts require activation)
