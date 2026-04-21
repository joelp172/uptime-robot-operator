# Configure Contacts

Configure the `Contact` custom resource to route monitor alerts to existing
UptimeRobot alert contacts.

## What the Contact resource does

- Resolves an existing UptimeRobot alert contact by ID or friendly name.
- Lets you set a reusable default contact for monitors.
- Provides the resolved contact ID in status.

`Contact` is cluster-scoped. Contacts are not created in UptimeRobot by this
resource; they must already exist in your account.

## Prerequisites

- You have an `Account` resource that is `Ready`.
- The target alert contact already exists in UptimeRobot.

## Find available alert contacts

```bash
kubectl get account default -o jsonpath='{range .status.alertContacts[*]}{.id}{"\t"}{.friendlyName}{"\t"}{.type}{"\n"}{end}'
```

## Create a Contact by ID

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: Contact
metadata:
  name: team-email
spec:
  contact:
    id: "1234567"
```

## Create a Contact by friendly name

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: Contact
metadata:
  name: team-email
spec:
  contact:
    name: "Team Email"
```

Use either `id` or `name`, not both.

## Set a default contact

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: Contact
metadata:
  name: default
spec:
  isDefault: true
  contact:
    id: "1234567"
```

Monitors without `spec.contacts` use the default contact.

## Verify readiness

```bash
kubectl get contact team-email -o jsonpath='{.status.ready}{"\t"}{.status.id}{"\n"}'
```

## Use a Contact from a Monitor

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: api-health
spec:
  contacts:
    - name: team-email
      threshold: 1m
      recurrence: 15m
  monitor:
    name: API Health
    url: https://api.example.com/health
```

## Troubleshooting

- `Contact` not ready:
  Confirm the ID or friendly name exists in `Account.status.alertContacts`.
- Ambiguous friendly name:
  Use `spec.contact.id` instead of `spec.contact.name`.

## Related guides

- [Configure Accounts](accounts.md)
- [Configure Monitors](monitors.md)
- [Configure Slack Alerting for Monitors](slack-alerting.md)
- [API Reference](api-reference.md)
