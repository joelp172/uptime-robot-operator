# Getting Started

Create your first UptimeRobot monitor in Kubernetes. By the end, you'll have a working monitor that alerts you when your website goes down.

## Prerequisites

- Kubernetes cluster v1.19+
- kubectl configured
- Operator installed ([installation guide](installation.md))
- UptimeRobot API key ([get one here](https://uptimerobot.com/?red=joelpi) - Integrations > API)

## Step 1: Store Your API Key

Create a Secret with your UptimeRobot API key:

```bash
kubectl create secret generic uptimerobot-api-key \
  --namespace uptime-robot-system \
  --from-literal=apiKey=YOUR_API_KEY
```

Replace `YOUR_API_KEY` with your actual key from UptimeRobot.

## Step 2: Configure Your Account

Create an Account resource that connects the operator to UptimeRobot:

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

Wait for the account to be ready:

```bash
kubectl wait --for=condition=Ready account/default --timeout=30s
```

## Step 3: Set Up Alert Contact

Get your contact ID from the account:

```bash
kubectl get account default -o jsonpath='{.status.alertContacts[0].id}'
```

Create a Contact resource with this ID:

```bash
kubectl apply -f - <<EOF
apiVersion: uptimerobot.com/v1alpha1
kind: Contact
metadata:
  name: default
spec:
  isDefault: true
  contact:
    id: "PASTE_YOUR_CONTACT_ID_HERE"
EOF
```

Replace `PASTE_YOUR_CONTACT_ID_HERE` with the ID from the previous command.

## Step 4: Create a Monitor

Create an HTTPS monitor for your website:

```bash
kubectl apply -f - <<EOF
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
EOF
```

The monitor automatically uses your default contact for alerts.

## Step 5: Verify

Check that your monitor is running:

```bash
kubectl get monitor my-website
```

You should see:

```
NAME         READY   FRIENDLY NAME   URL                   AGE
my-website   true    My Website      https://example.com   30s
```

Log in to [UptimeRobot Dashboard](https://dashboard.uptimerobot.com/) to see your monitor.

## What You've Achieved

You now have:
- A monitor checking your website every 5 minutes
- Automatic alerts when the site goes down
- GitOps-ready configuration (commit the YAML to git)

## Next Steps

- [Configure different monitor types](monitors.md) - Keyword, DNS, Heartbeat, Port, Ping
- [Schedule maintenance windows](maintenance-windows.md) - Prevent false alerts during deployments
- [API Reference](api-reference.md) - All available configuration options
