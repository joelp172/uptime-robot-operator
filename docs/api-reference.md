# API Reference

Complete field reference for all Custom Resource Definitions.

## Account

Connects the operator to your UptimeRobot account.

**Scope:** Cluster-scoped (no namespace)

### Spec

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `isDefault` | boolean | No | `false` | Use this account when monitors don't specify one |
| `apiKeySecretRef.name` | string | Yes | - | Secret name containing API key (must be in `uptime-robot-system` namespace) |
| `apiKeySecretRef.key` | string | Yes | - | Key within Secret containing API key |

### Status

| Field | Type | Description |
|-------|------|-------------|
| `ready` | boolean | Account successfully connected |
| `email` | string | Email address of UptimeRobot account |
| `alertContacts[]` | array | Available alert contacts |
| `alertContacts[].id` | string | Contact ID (use in Contact resources) |
| `alertContacts[].friendlyName` | string | Display name |
| `alertContacts[].type` | string | Contact type (Email, SMS, MobileApp, etc.) |
| `alertContacts[].value` | string | Contact value (email, phone, etc.) |

---

## Contact

References an existing alert contact in UptimeRobot.

**Scope:** Cluster-scoped (no namespace)

**Note:** Contacts must be created in UptimeRobot dashboard first.

### Spec

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `isDefault` | boolean | No | `false` | Use this contact when monitors don't specify one |
| `account.name` | string | No | default account | Account to use |
| `contact.id` | string | No* | - | UptimeRobot contact ID |
| `contact.name` | string | No* | - | Contact friendlyName (must match exactly) |

*Either `id` or `name` required, not both.

### Status

| Field | Type | Description |
|-------|------|-------------|
| `ready` | boolean | Contact found in UptimeRobot |
| `id` | string | Resolved contact ID |

---

## Monitor

Defines an UptimeRobot monitor.

**Scope:** Namespaced

### Spec

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `syncInterval` | duration | No | `24h` | Reconciliation frequency |
| `prune` | boolean | No | `true` | Delete from UptimeRobot when CR deleted |
| `account.name` | string | No | default | Account to use |
| `contacts[]` | array | No | default contact | Alert contacts |
| `contacts[].name` | string | Yes | - | Contact resource name |
| `contacts[].threshold` | duration | No | `1m` | Wait before first alert |
| `contacts[].recurrence` | duration | No | `0` | Repeat interval (0 = no repeat) |
| `sourceRef` | object | No | - | Optional source reference |
| `monitor` | MonitorValues | Yes | - | Monitor configuration |

### MonitorValues

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | Yes | - | Display name |
| `url` | string | Conditional | - | URL or IP (not required for Heartbeat) |
| `type` | string | No | `HTTPS` | `HTTPS`, `Keyword`, `Ping`, `Port`, `Heartbeat`, `DNS` |
| `interval` | duration | No | `60s` | Check interval |
| `timeout` | duration | No | `30s` | Request timeout |
| `gracePeriod` | duration | No | `60s` | Wait before alerting (max 24h) |
| `status` | integer | No | `1` | 0=paused, 1=running |
| `method` | string | No | `HEAD` | HTTP method |
| `keyword` | object | No | - | Keyword monitor config |
| `dns` | object | No | - | DNS monitor config |
| `heartbeat` | object | No | - | Heartbeat monitor config |
| `port` | object | No | - | Port monitor config |
| `auth` | object | No | - | HTTP auth config |
| `post` | object | No | - | POST body config |
| `tags` | []string | No | - | Tags |
| `customHttpHeaders` | map[string]string | No | - | Custom headers |
| `successHttpResponseCodes` | []string | No | - | Success codes (e.g. `2xx`, `200`) |
| `checkSSLErrors` | boolean | No | - | Enable SSL/domain checks |
| `sslExpirationReminder` | boolean | No | - | Notify before SSL expiry |
| `domainExpirationReminder` | boolean | No | - | Notify before domain expiry |
| `followRedirections` | boolean | No | - | Follow redirects |
| `responseTimeThreshold` | integer | No | - | Response time threshold (ms, 0-60000) |
| `region` | string | No | - | Region: `na`, `eu`, `as`, `oc` |
| `groupId` | integer | No | - | UptimeRobot group ID (0=none) |
| `maintenanceWindowIds` | []integer | No | - | Maintenance window IDs |

### Auth

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | Yes | `Basic` or `Digest` |
| `username` | string | No | Username |
| `password` | string | No | Password |
| `secretName` | string | No | Secret containing credentials |
| `usernameKey` | string | No | Secret key for username |
| `passwordKey` | string | No | Secret key for password |

### POST

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `postType` | string | No | `KeyValue` or `RawData` |
| `contentType` | string | No | `text/html` or `application/json` |
| `value` | string | No | Request body |

### Keyword

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | Yes | `Exists` or `NotExists` |
| `value` | string | Yes | Text to search for |
| `caseSensitive` | boolean | No | Case-sensitive matching |

### DNS

| Field | Type | Description |
|-------|------|-------------|
| `a` | []string | Expected A records |
| `aaaa` | []string | Expected AAAA records |
| `cname` | []string | Expected CNAME records |
| `mx` | []string | Expected MX records |
| `ns` | []string | Expected NS records |
| `txt` | []string | Expected TXT records |
| `srv` | []string | Expected SRV records |
| `ptr` | []string | Expected PTR records |
| `soa` | []string | Expected SOA records |
| `spf` | []string | Expected SPF records |
| `sslExpirationPeriodDays` | []int | SSL expiry reminder offsets (0-365) |

### Heartbeat

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `interval` | duration | No | Expected ping interval (default: 60s) |

### Port

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `number` | integer | Yes | Port number (0-65535) |

### Status

| Field | Type | Description |
|-------|------|-------------|
| `ready` | boolean | Monitor exists in UptimeRobot |
| `id` | string | UptimeRobot monitor ID |
| `heartbeatURL` | string | Webhook URL (Heartbeat monitors only) |
| `type` | string | Monitor type |
| `status` | integer | Current status code |

---

## MaintenanceWindow

Schedule planned downtime.

**Scope:** Namespaced

### Spec

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `syncInterval` | duration | No | `24h` | Reconciliation frequency |
| `prune` | boolean | No | `true` | Delete from UptimeRobot when CR deleted |
| `account.name` | string | No | default | Account to use |
| `name` | string | Yes | - | Friendly name (max 255 chars) |
| `interval` | string | Yes | - | `once`, `daily`, `weekly`, `monthly` |
| `startDate` | string | Yes | - | Start date (YYYY-MM-DD) |
| `startTime` | string | Yes | - | Start time (HH:mm:ss) |
| `duration` | duration | Yes | - | Duration (e.g. `30m`, `1h`, `2h30m`) |
| `days` | []int | Conditional | - | Days for weekly/monthly (see below) |
| `autoAddMonitors` | boolean | No | `false` | Add all monitors automatically |
| `monitorRefs` | []LocalObjectReference | No | - | Specific monitors to add |

**Days field:**
- Weekly: 0=Sunday, 1=Monday, ..., 6=Saturday
- Monthly: 1-31 for specific days, -1 for last day

### Status

| Field | Type | Description |
|-------|------|-------------|
| `ready` | boolean | Maintenance window created |
| `id` | string | UptimeRobot maintenance window ID |
| `monitorCount` | integer | Number of assigned monitors |

---

## SlackIntegration

Creates and manages a Slack integration in UptimeRobot.

**Scope:** Namespaced

### Spec

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `syncInterval` | duration | No | `24h` | Reconciliation frequency |
| `prune` | boolean | No | `true` | Delete integration from UptimeRobot when CR is deleted |
| `account.name` | string | No | default | Account to use |
| `integration` | object | Yes | - | Slack integration configuration |

### Integration

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `friendlyName` | string | No | - | Display name (max 60) |
| `enableNotificationsFor` | string | No | `UpAndDown` | `UpAndDown`, `Down`, `Up`, `None` |
| `sslExpirationReminder` | boolean | No | `false` | Notify for SSL/domain expiration |
| `webhookURL` | string | Conditional | - | Slack webhook URL (max 1500) |
| `secretName` | string | Conditional | - | Secret name containing webhook URL |
| `webhookURLKey` | string | No | `webhookURL` | Secret key containing webhook URL |
| `customValue` | string | No | - | Extra message text (max 5000) |

Validation:
- Specify exactly one of `webhookURL` or `secretName`.

### Status

| Field | Type | Description |
|-------|------|-------------|
| `ready` | boolean | Integration exists in UptimeRobot |
| `id` | string | UptimeRobot integration ID |
| `type` | string | Integration type (`Slack`) |

---

## Duration Format

All duration fields use Go duration format:

| Format | Duration |
|--------|----------|
| `30s` | 30 seconds |
| `5m` | 5 minutes |
| `1h` | 1 hour |
| `24h` | 24 hours |
| `1h30m` | 1 hour 30 minutes |

---

## Examples

See the how-to guides for complete examples:

- [Getting Started](getting-started.md) - First monitor
- [Monitors](monitors.md) - All monitor types
- [Maintenance Windows](maintenance-windows.md) - Scheduling downtime
