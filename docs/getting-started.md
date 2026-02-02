# Getting Started

This guide walks you through installing the Uptime Robot Operator and creating your first monitor.

## Prerequisites

- Kubernetes cluster v1.19+
- kubectl configured to access your cluster
- UptimeRobot account with API access

## Step 1: Get Your API Key

1. Log in to [UptimeRobot](https://uptimerobot.com/?red=joelpi) (or [sign up free](https://uptimerobot.com/?red=joelpi))
2. Navigate to **Integrations** > **API**
3. Create or copy your **Main API Key**

## Step 2: Install the Operator

Install the operator and CRDs:

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

## Step 3: Create the API Key Secret

Create a Secret in the `uptime-robot-system` namespace (where the operator runs):

```bash
kubectl create secret generic uptimerobot-api-key \
  --namespace uptime-robot-system \
  --from-literal=apiKey=YOUR_API_KEY_HERE
```

Replace `YOUR_API_KEY_HERE` with your actual API key.

**Important:** The Secret must be in the `uptime-robot-system` namespace. Account and Contact resources are cluster-scoped and reference this Secret.

## Step 4: Create an Account

The Account resource connects the operator to your UptimeRobot account. It is cluster-scoped (no namespace required):

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

Apply it:

```bash
kubectl apply -f - <<EOF
apiVersion: uptimerobot.com/v1alpha1
kind: Account
metadata:
  name: default
spec:
  isDefault: true
  apiKeySecretRef:
    name: uptimerobot-api-key
    key: apiKey
EOF
```

Verify the account is ready:

```bash
kubectl get accounts
```

Expected output:

```
NAME      READY   DEFAULT   EMAIL                    AGE
default   true    true      your@email.com           10s
```

## Step 5: Create a Contact

Monitors require an alert contact for notifications. First, find your contact IDs from the Account status:

```bash
kubectl get account default -o jsonpath='{range .status.alertContacts[*]}{.id}{"\t"}{.type}{"\t"}{.value}{"\n"}{end}'
```

Expected output:

```
1234567   Email   your@email.com
7654321   MobileApp   ...
```

Create a Contact resource using one of the IDs:

```bash
kubectl apply -f - <<EOF
apiVersion: uptimerobot.com/v1alpha1
kind: Contact
metadata:
  name: default
spec:
  isDefault: true
  contact:
    id: "1234567"
EOF
```

Replace `1234567` with your actual contact ID.

Verify the contact is ready:

```bash
kubectl get contacts
```

Expected output:

```
NAME      READY   DEFAULT   FRIENDLY NAME   AGE
default   true    true                      10s
```

## Step 6: Create Your First Monitor

Create a basic HTTPS monitor:

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: my-website
spec:
  monitor:
    name: My Website
    url: https://example.com
    type: HTTPS
    interval: 5m
```

Apply it:

```bash
kubectl apply -f - <<EOF
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: my-website
spec:
  monitor:
    name: My Website
    url: https://example.com
    type: HTTPS
    interval: 5m
EOF
```

The monitor uses the default contact for notifications (created in Step 5).

Verify the monitor was created:

```bash
kubectl get monitors
```

Expected output:

```
NAME         READY   FRIENDLY NAME   URL                   AGE
my-website   true    My Website      https://example.com   10s
```

The monitor now appears in your [UptimeRobot Dashboard](https://dashboard.uptimerobot.com/).

## Bonus: Create a Heartbeat Monitor

Heartbeat monitors are useful for cron jobs and scheduled tasks. Unlike other monitors, they don't require a URL - UptimeRobot generates a webhook URL for your services to ping:

```bash
kubectl apply -f - <<EOF
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: backup-job
spec:
  monitor:
    name: Daily Backup
    type: Heartbeat
    interval: 24h
    heartbeat:
      interval: 24h
EOF
```

Retrieve the generated webhook URL:

```bash
kubectl get monitor backup-job -o jsonpath='{.status.heartbeatURL}'
```

Your backup script should call this URL on completion. If UptimeRobot doesn't receive a ping within the interval, it triggers an alert.

## Next Steps

- [API Reference](api-reference.md) - Learn about all available fields
- [Configure Alerts](configure-alerts.md) - Set up alert notifications
- Explore other monitor types: Keyword, DNS, Heartbeat, Port, Ping

## Uninstall

Remove all monitors and the operator:

```bash
# Delete all monitors (this also deletes them from UptimeRobot if prune: true)
kubectl delete monitors --all

# Delete accounts and contacts
kubectl delete accounts --all
kubectl delete contacts --all

# Remove the operator
kubectl delete -f https://github.com/joelp172/uptime-robot-operator/releases/latest/download/install.yaml
```
