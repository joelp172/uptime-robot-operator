# API Reference

Complete reference for all Custom Resource Definitions provided by the Uptime Robot Operator.

## Account

Connects the operator to your UptimeRobot account via API key.

**Scope:** Cluster (no namespace required)

**Note:** The Secret referenced by `apiKeySecretRef` must be in the `uptime-robot-system` namespace.

### Spec

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `isDefault` | boolean | No | `false` | Use this account when monitors don't specify one |
| `apiKeySecretRef.name` | string | Yes | - | Name of the Secret containing the API key |
| `apiKeySecretRef.key` | string | Yes | - | Key within the Secret that holds the API key |

### Status

| Field | Type | Description |
|-------|------|-------------|
| `ready` | boolean | Whether the account is successfully connected |
| `email` | string | Email address associated with the UptimeRobot account |
| `alertContacts` | array | List of available alert contacts (see below) |

#### AlertContactInfo

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique contact ID (use this in Contact resources) |
| `friendlyName` | string | Display name (may be empty) |
| `type` | string | Contact type (Email, SMS, MobileApp, etc.) |
| `value` | string | Contact value (email address, phone number, etc.) |

### Example

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

---

## Contact

References an existing alert contact in your UptimeRobot account.

**Scope:** Cluster (no namespace required)

**Note:** The operator does not create contacts in UptimeRobot. You must create contacts in the UptimeRobot dashboard first, then reference them here.

### Spec

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `isDefault` | boolean | No | `false` | Use this contact when monitors don't specify one |
| `account.name` | string | No | default account | Account to use for API access |
| `contact.id` | string | No* | - | UptimeRobot contact ID |
| `contact.name` | string | No* | - | Contact friendlyName (must match exactly) |

*Either `id` or `name` is required, but not both. Use `id` for contacts without a friendlyName.

### Status

| Field | Type | Description |
|-------|------|-------------|
| `ready` | boolean | Whether the contact was found in UptimeRobot |
| `id` | string | Resolved contact ID |

### Examples

Reference by ID (recommended):

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

Reference by friendlyName:

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: Contact
metadata:
  name: my-phone
spec:
  contact:
    name: "iPhone"
```

---

## Monitor

Defines an UptimeRobot monitor.

**Scope:** Namespaced

### Spec

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `syncInterval` | duration | No | `24h` | How often to reconcile with UptimeRobot API |
| `prune` | boolean | No | `true` | Delete monitor from UptimeRobot when CR is deleted |
| `account.name` | string | No | default account | Account to use for API access |
| `contacts` | array | No | default contact | Alert contacts to notify |
| `sourceRef` | object | No | - | Optional source object reference (kind/name/apiGroup) |
| `monitor` | MonitorValues | Yes | - | Monitor configuration (see below) |

### MonitorValues

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | Yes | - | Display name in UptimeRobot |
| `url` | string | Conditional | - | URL or IP to monitor (not required for Heartbeat monitors) |
| `type` | string | No | `HTTPS` | Monitor type (see Monitor Types) |
| `interval` | duration | No | `60s` | Check interval |
| `timeout` | duration | No | `30s` | Request timeout |
| `gracePeriod` | duration | No | `60s` | Wait time before alerting (max 24h) |
| `status` | integer | No | `1` | 0 = paused, 1 = running |
| `method` | string | No | `HEAD` | HTTP method (HEAD, GET, POST, etc.) |
| `keyword` | object | No | - | Keyword monitor config |
| `dns` | object | No | - | DNS monitor config |
| `heartbeat` | object | No | - | Heartbeat monitor config |
| `port` | object | No | - | Port monitor config |
| `auth` | object | No | - | HTTP authentication config |
| `post` | object | No | - | POST request body config |
| `tags` | []string | No | - | Tags to assign in UptimeRobot |
| `customHttpHeaders` | map[string]string | No | - | Custom HTTP headers |
| `successHttpResponseCodes` | []string | No | - | Success HTTP codes (e.g. `2xx`, `200`) |
| `checkSSLErrors` | boolean | No | - | Enable SSL/domain error checks |
| `sslExpirationReminder` | boolean | No | - | Notify before SSL cert expiry |
| `domainExpirationReminder` | boolean | No | - | Notify before domain expiry |
| `followRedirections` | boolean | No | - | Follow HTTP redirects |
| `responseTimeThreshold` | integer | No | - | Response time threshold in ms (0-60000) |
| `region` | string | No | - | Region: `na`, `eu`, `as`, `oc` |
| `groupId` | integer | No | - | UptimeRobot group ID (0 = none) |
| `maintenanceWindowIds` | []integer | No | - | Maintenance window IDs to assign |

### Auth

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `auth.type` | string | Yes | `Basic` or `Digest` |
| `auth.username` | string | No | Username for HTTP auth |
| `auth.password` | string | No | Password for HTTP auth |
| `auth.secretName` | string | No | Secret containing credentials |
| `auth.usernameKey` | string | No | Secret key for username |
| `auth.passwordKey` | string | No | Secret key for password |

```yaml
spec:
  monitor:
    name: Authenticated Endpoint
    url: https://secure.example.com/health
    type: HTTPS
    auth:
      type: Basic
      secretName: http-auth
      usernameKey: username
      passwordKey: password
```

### POST

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `post.postType` | string | No | `KeyValue` or `RawData` |
| `post.contentType` | string | No | `text/html` or `application/json` |
| `post.value` | string | No | Request body content |

```yaml
spec:
  monitor:
    name: POST Endpoint
    url: https://api.example.com/submit
    type: HTTPS
    method: POST
    post:
      postType: RawData
      contentType: application/json
      value: '{"status":"ok"}'
```

### Monitor Types

#### HTTPS

Standard HTTP/HTTPS endpoint monitoring.

```yaml
spec:
  monitor:
    name: My API
    url: https://api.example.com/health
    type: HTTPS
    interval: 5m
    method: GET
    tags:
      - production
      - public-api
```

#### Keyword

Check for specific text in page content.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `keyword.type` | string | Yes | `Exists` or `NotExists` |
| `keyword.value` | string | Yes | Text to search for |
| `keyword.caseSensitive` | boolean | No | Case-sensitive matching (default: false) |

```yaml
spec:
  monitor:
    name: Status Page
    url: https://status.example.com
    type: Keyword
    interval: 5m
    keyword:
      type: Exists
      value: "All Systems Operational"
      caseSensitive: false
```

#### DNS

Verify DNS records resolve to expected values. For DNS monitors, set `monitor.url` to
a hostname or IP with no scheme (e.g., `dns.google.com` or `8.8.8.8`).

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `dns.a` | []string | No | Expected A record values |
| `dns.aaaa` | []string | No | Expected AAAA record values |
| `dns.cname` | []string | No | Expected CNAME record values |
| `dns.mx` | []string | No | Expected MX record values |
| `dns.ns` | []string | No | Expected NS record values |
| `dns.txt` | []string | No | Expected TXT record values |
| `dns.srv` | []string | No | Expected SRV record values |
| `dns.ptr` | []string | No | Expected PTR record values |
| `dns.soa` | []string | No | Expected SOA record values |
| `dns.spf` | []string | No | Expected SPF record values |
| `dns.sslExpirationPeriodDays` | []int | No | SSL expiry reminder offsets (0-365) |

```yaml
spec:
  monitor:
    name: DNS Check
    url: example.com
    type: DNS
    interval: 5m
    dns:
      a:
        - "93.184.216.34"
      sslExpirationPeriodDays:
        - 7
```

#### Heartbeat

Expects periodic pings from your services or cron jobs. Unlike other monitor types, Heartbeat monitors do not require a `url` field - UptimeRobot generates a unique webhook URL after creation.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `heartbeat.interval` | duration | No | Expected interval between pings (default: 60s) |

```yaml
spec:
  monitor:
    name: Backup Job
    type: Heartbeat
    interval: 1h
    heartbeat:
      interval: 1h
```

After the monitor is created, retrieve the webhook URL from the status:

```bash
kubectl get monitor backup-job -o jsonpath='{.status.heartbeatURL}'
```

The URL format is `https://heartbeat.uptimerobot.com/m{id}-{token}`. Your services or cron jobs should send HTTP requests to this URL at the specified interval to indicate they are alive.

#### Port

TCP port monitoring.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `port.number` | integer | Yes | Port number (0-65535) |

```yaml
spec:
  monitor:
    name: Database Port
    url: db.example.com
    type: Port
    port:
      number: 5432
```

#### Ping

ICMP ping monitoring.

```yaml
spec:
  monitor:
    name: Server Ping
    url: 192.168.1.1
    type: Ping
    interval: 5m
```

### Contact Reference

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `contacts[].name` | string | Yes | - | Name of the Contact resource |
| `contacts[].threshold` | duration | No | `1m` | Wait time before notifying |
| `contacts[].recurrence` | duration | No | `0` | Repeat notification interval (0 = no repeat) |

```yaml
spec:
  contacts:
    - name: my-email
      threshold: 5m
      recurrence: 30m
  monitor:
    name: Critical Service
    url: https://example.com
```

### Maintenance Windows

Assign maintenance windows to monitors to prevent alerts during scheduled maintenance periods. Maintenance windows must be created in your UptimeRobot account first.

To find maintenance window IDs:
1. Log in to [UptimeRobot](https://uptimerobot.com)
2. Navigate to **Maintenance Windows**
3. The ID is visible in the URL or window details

```yaml
spec:
  monitor:
    name: Production API
    url: https://api.example.com
    maintenanceWindowIds:
      - 12345  # Weekly maintenance window
      - 67890  # Emergency maintenance window
```

### Status

| Field | Type | Description |
|-------|------|-------------|
| `ready` | boolean | Whether the monitor exists in UptimeRobot |
| `id` | string | UptimeRobot monitor ID |
| `heartbeatURL` | string | Webhook URL for Heartbeat monitors (only populated for type: Heartbeat) |
| `type` | string | Monitor type |
| `status` | integer | Current status code |

### Full Example

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: production-api
spec:
  syncInterval: 5m
  prune: true
  contacts:
    - name: ops-team
      threshold: 2m
      recurrence: 15m
  monitor:
    name: Production API
    url: https://api.example.com/health
    type: HTTPS
    interval: 1m
    timeout: 10s
    gracePeriod: 2m
    method: GET
    tags:
      - production
      - api
    maintenanceWindowIds:
      - 12345
```

---

## MaintenanceWindow

Schedule planned downtime periods to prevent false alerts during deployments or maintenance.

**Scope:** Namespaced

### Spec

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `syncInterval` | duration | No | `24h` | How often to reconcile with UptimeRobot API |
| `prune` | boolean | No | `true` | Delete maintenance window from UptimeRobot when CR is deleted |
| `account.name` | string | No | default account | Account to use for API access |
| `name` | string | Yes | - | Friendly name of the maintenance window (max 255 chars) |
| `interval` | string | Yes | - | Recurrence pattern: `once`, `daily`, `weekly`, `monthly` |
| `startDate` | string | Yes | - | Start date in YYYY-MM-DD format |
| `startTime` | string | Yes | - | Start time in HH:mm:ss format |
| `duration` | duration | Yes | - | Duration of the maintenance window (e.g., "30m", "1h", "2h30m") |
| `days` | []int | Conditional | - | Days for weekly/monthly intervals (see below) |
| `autoAddMonitors` | boolean | No | `false` | Automatically add all monitors to this window |
| `monitorRefs` | []LocalObjectReference | No | - | List of Monitor resources to add to this window |

### Days Field

The `days` field is required for `weekly` and `monthly` intervals:

- **Weekly**: Day of week (0=Sunday, 1=Monday, ..., 6=Saturday)
- **Monthly**: Day of month (1-31, or -1 for last day of month)

### Status

| Field | Type | Description |
|-------|------|-------------|
| `ready` | boolean | Whether the maintenance window is successfully created |
| `id` | string | UptimeRobot maintenance window ID |
| `monitorCount` | integer | Number of monitors assigned to this window |

### Examples

#### One-time Maintenance Window

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: MaintenanceWindow
metadata:
  name: emergency-maintenance
spec:
  name: "Emergency Database Upgrade"
  interval: once
  startDate: "2026-03-15"
  startTime: "03:00:00"
  duration: 2h
  autoAddMonitors: true
```

#### Daily Maintenance Window

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: MaintenanceWindow
metadata:
  name: daily-backup-window
spec:
  name: "Daily Backup Window"
  interval: daily
  startDate: "2026-02-01"
  startTime: "02:00:00"
  duration: 30m
  monitorRefs:
    - name: database-monitor
    - name: storage-monitor
```

#### Weekly Deployment Window

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: MaintenanceWindow
metadata:
  name: weekly-deployment-window
spec:
  name: "Weekly Deployment Window"
  interval: weekly
  startDate: "2026-02-10"
  startTime: "02:00:00"
  duration: 1h
  days: [2, 4]  # Tuesday and Thursday
  monitorRefs:
    - name: production-api
    - name: frontend-app
```

#### Monthly Maintenance Window

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: MaintenanceWindow
metadata:
  name: monthly-maintenance
spec:
  name: "Monthly Maintenance"
  interval: monthly
  startDate: "2026-02-01"
  startTime: "05:00:00"
  duration: 4h
  days: [1, 15, -1]  # 1st, 15th, and last day of month
  autoAddMonitors: false
  monitorRefs:
    - name: my-website
```

### Monitor References

Maintenance windows can be assigned to monitors in two ways:

1. **Auto-add**: Set `autoAddMonitors: true` to include all monitors
2. **Explicit references**: List specific monitors in `monitorRefs`

Note: Monitors must be in the same namespace as the MaintenanceWindow resource.

### Validation Rules

- `days` is required when `interval` is `weekly` or `monthly`
- `days` must not be set when `interval` is `once` or `daily`
- For weekly intervals, `days` values must be 0-6
- For monthly intervals, `days` values must be 1-31 or -1
- `duration` must be at least 1 minute

---

## Duration Format

Duration fields accept Go duration strings:

| Unit | Example |
|------|---------|
| Seconds | `30s` |
| Minutes | `5m` |
| Hours | `24h` |
| Combined | `1h30m` |
