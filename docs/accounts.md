# Configure Accounts

Configure the `Account` custom resource to connect the operator to UptimeRobot.

## What the Account resource does

- Stores how the operator authenticates to UptimeRobot.
- Exposes account metadata and available alert contacts in status.
- Can be marked as the default account for other resources.

`Account` is cluster-scoped.

## Prerequisites

- You have installed the operator.
- You have an UptimeRobot API key.
- You can create secrets in the `uptime-robot-system` namespace.

## Create an API key secret

```bash
kubectl create secret generic uptimerobot-api-key \
  --namespace uptime-robot-system \
  --from-literal=apiKey=YOUR_API_KEY
```

## Create an Account resource

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

Apply:

```bash
kubectl apply -f account.yaml
```

## Verify readiness

```bash
kubectl get account default -o jsonpath='{.status.ready}{"\n"}'
```

List available alert contacts:

```bash
kubectl get account default -o jsonpath='{range .status.alertContacts[*]}{.id}{"\t"}{.friendlyName}{"\t"}{.type}{"\n"}{end}'
```

## Use a non-default account

If you manage multiple accounts, create additional `Account` resources and reference
one explicitly from other resources:

```yaml
spec:
  account:
    name: production
```

## Troubleshooting

- `Account` is not ready:
  Check `kubectl describe account <name>` for API key or connectivity errors.
- No contacts in `status.alertContacts`:
  Confirm the API key has access to alert contacts in UptimeRobot.

## Related guides

- [Configure Contacts](contacts.md)
- [Configure Monitors](monitors.md)
- [Configure Slack Integrations](slack-integrations.md)
- [API Reference](api-reference.md)
