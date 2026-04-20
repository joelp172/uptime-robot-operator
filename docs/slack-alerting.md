# Configure Slack Alerting for Monitors

Set up Slack notifications in UptimeRobot and attach them to monitors managed by this operator.

## Prerequisites

- You have installed the operator.
- You have an `Account` resource that is `Ready`.
- You have a Slack incoming webhook URL for your target channel.

## How Slack alerting works

- `SlackIntegration` manages the Slack webhook integration in UptimeRobot.
- `Monitor` resources do not reference `SlackIntegration` directly.
- `Monitor` resources send alerts through `Contact` resources in `spec.contacts`.
- `Contact` lookup by `spec.contact.name` first checks account alert contacts, then falls back to matching Slack integrations by friendly name.

Use this flow:

1. Create a `SlackIntegration` resource.
2. Confirm a Slack contact is visible in `Account.status.alertContacts` (the operator now includes matching Slack integrations there for discoverability).
3. Create a `Contact` resource that references that alert contact.
4. Reference that `Contact` from your `Monitor.spec.contacts`.

## Step 1: Create a SlackIntegration

Store your webhook URL in a Secret:

```bash
kubectl create secret generic platform-alerts-webhook \
  -n uptime-robot-system \
  --from-literal=webhookURL='https://example.invalid/your-slack-webhook-url'
```

Create the `SlackIntegration`:

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: SlackIntegration
metadata:
  name: platform-alerts
  namespace: uptime-robot-system
spec:
  account:
    name: default
  syncInterval: 24h
  prune: true
  integration:
    friendlyName: platform-alerts
    secretName: platform-alerts-webhook
    webhookURLKey: webhookURL
    enableNotificationsFor: UpAndDown
    sslExpirationReminder: true
```

Apply:

```bash
kubectl apply -f slackintegration.yaml
```

Verify:

```bash
kubectl get slackintegration platform-alerts -n uptime-robot-system
kubectl get slackintegration platform-alerts -n uptime-robot-system -o jsonpath='{.status.ready}{"\t"}{.status.id}{"\n"}'
```

## Step 2: Find the Slack alert contact ID

List contacts visible to your account:

```bash
kubectl get account default -o jsonpath='{range .status.alertContacts[*]}{.id}{"\t"}{.friendlyName}{"\t"}{.type}{"\n"}{end}'
```

Pick the Slack contact ID (or friendly name) you want monitors to use.

## Step 3: Create a Contact resource for Slack

Reference by ID:

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: Contact
metadata:
  name: platform-alerts-slack
spec:
  contact:
    id: "1234567"
```

Or reference by friendly name:

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: Contact
metadata:
  name: platform-alerts-slack
spec:
  contact:
    name: "platform-alerts"
```

Apply and verify:

```bash
kubectl apply -f contact-slack.yaml
kubectl get contact platform-alerts-slack -o jsonpath='{.status.ready}{"\t"}{.status.id}{"\n"}'
```

## Step 4: Wire the Contact into a Monitor

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: api-health
spec:
  account:
    name: default
  contacts:
    - name: platform-alerts-slack
      threshold: 1m
      recurrence: 15m
  monitor:
    name: API Health
    url: https://api.example.com/health
    type: HTTPS
    interval: 1m
    method: GET
```

Apply and verify:

```bash
kubectl apply -f monitor-api-health.yaml
kubectl get monitor api-health -o jsonpath='{.status.ready}{"\t"}{.status.id}{"\n"}'
```

## Troubleshooting

- `SlackIntegration` not ready:
  Check `kubectl describe slackintegration <name> -n <namespace>` for webhook/account errors.
- `Contact` not ready:
  Confirm the referenced contact name/ID exists in `Account.status.alertContacts`. If you have multiple Slack integrations with the same friendly name, use `spec.contact.id` to avoid ambiguity.
- `Monitor` has no Slack notifications:
  Confirm the monitor includes the Slack `Contact` in `spec.contacts`.
