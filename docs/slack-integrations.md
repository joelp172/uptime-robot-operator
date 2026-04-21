# Configure Slack Integrations

Configure the `SlackIntegration` custom resource to manage Slack webhook
integrations in UptimeRobot.

## What the SlackIntegration resource does

- Creates and updates a Slack integration in UptimeRobot.
- Stores integration state and resolved ID in status.
- Makes Slack-backed contacts discoverable through `Account.status.alertContacts`.

`SlackIntegration` is namespaced.

## Prerequisites

- You have installed the operator.
- You have an `Account` resource that is `Ready`.
- You have a Slack incoming webhook URL.

## Create the webhook secret

```bash
kubectl create secret generic platform-alerts-webhook \
  -n uptime-robot-system \
  --from-literal=webhookURL='https://example.invalid/your-slack-webhook-url'
```

## Create a SlackIntegration resource

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

Apply and verify:

```bash
kubectl apply -f slackintegration.yaml
kubectl get slackintegration platform-alerts -n uptime-robot-system -o jsonpath='{.status.ready}{"\t"}{.status.id}{"\n"}'
```

## Connect Slack integration to monitors

`Monitor` resources do not reference `SlackIntegration` directly. Use this flow:

1. Create `SlackIntegration`.
2. Create a `Contact` that points to the Slack contact ID or friendly name.
3. Reference that `Contact` in `Monitor.spec.contacts`.

For the full end-to-end workflow, see
[Configure Slack Alerting for Monitors](slack-alerting.md).

## Troubleshooting

- `SlackIntegration` not ready:
  Check `kubectl describe slackintegration <name> -n <namespace>` for
  account, webhook, or API validation errors.
- Slack contact missing from account status:
  Wait for reconciliation and confirm the referenced account is `Ready`.

## Related guides

- [Configure Slack Alerting for Monitors](slack-alerting.md)
- [Configure Contacts](contacts.md)
- [Configure Monitors](monitors.md)
- [API Reference](api-reference.md)
