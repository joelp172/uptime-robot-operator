# Configure Monitors

Configure different monitor types, alert contacts, and monitoring behaviour.

## Monitor Types

### HTTPS

Monitor HTTP/HTTPS endpoints:

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: api-health
spec:
  monitor:
    name: API Health Check
    url: https://api.example.com/health
    type: HTTPS
    interval: 1m
    method: GET
    timeout: 10s
```

### Keyword

Check for specific text in page content:

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: status-page
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

Set `type: NotExists` to alert when text is found.

### DNS

Verify DNS records:

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: dns-check
spec:
  monitor:
    name: DNS Check
    url: example.com
    type: DNS
    interval: 5m
    dns:
      a:
        - "93.184.216.34"
```

Supported record types: `a`, `aaaa`, `cname`, `mx`, `ns`, `txt`, `srv`, `ptr`, `soa`, `spf`.

### Heartbeat

Monitor cron jobs and scheduled tasks. UptimeRobot generates a webhook URL that your service pings:

```yaml
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
  heartbeatURLPublish:
    type: Secret
    name: backup-job-heartbeat
    key: url
```

Get the webhook URL:

```bash
kubectl get monitor backup-job -o jsonpath='{.status.heartbeatURL}'
```

Your service should call this URL after each successful run.

You can inject it into workloads from the published Secret/ConfigMap:

```bash
kubectl get secret backup-job-heartbeat -o jsonpath='{.data.url}' | base64 -d
```

Use one Secret/ConfigMap per heartbeat monitor (default naming is `<monitor-name>-heartbeat-url`).
This avoids key collisions when multiple heartbeat monitors exist in the same namespace.

Operator-wide heartbeat base URL can be overridden with environment variable
`UPTIMEROBOT_HEARTBEAT_BASE_URL` (default: `https://heartbeat.uptimerobot.com`).

### Port

Monitor TCP port connectivity:

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: database
spec:
  monitor:
    name: Database Port
    url: db.example.com
    type: Port
    port:
      number: 5432
```

### Ping

ICMP ping monitoring:

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: server
spec:
  monitor:
    name: Server Ping
    url: 192.168.1.1
    type: Ping
    interval: 5m
```

## Alert Contacts

### Using Default Contact

Monitors automatically use the default contact:

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

### Multiple Contacts

Configure different notification timing:

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: critical-api
spec:
  contacts:
    - name: oncall-phone
      threshold: 1m
    - name: team-email
      threshold: 5m
      recurrence: 30m
    - name: manager
      threshold: 15m
  monitor:
    name: Critical API
    url: https://api.example.com
```

| Field | Description | Default |
|-------|-------------|---------|
| `name` | Contact resource name | Required |
| `threshold` | Wait time before first alert | `1m` |
| `recurrence` | Repeat alert interval (0 = no repeat) | `0` |

### Creating Contacts

Contacts reference existing alert contacts in UptimeRobot. Create them in the UptimeRobot dashboard first.

Find available contact IDs:

```bash
kubectl get account default -o jsonpath='{range .status.alertContacts[*]}{.id}{"\t"}{.type}{"\t"}{.value}{"\n"}{end}'
```

Create a Contact resource:

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: Contact
metadata:
  name: team-email
spec:
  contact:
    id: "1234567"
```

Or reference by friendly name:

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: Contact
metadata:
  name: my-phone
spec:
  contact:
    name: "iPhone"
```

## Authentication

### HTTP Basic Auth

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: authenticated-endpoint
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

Create the secret:

```bash
kubectl create secret generic http-auth \
  --from-literal=username=myuser \
  --from-literal=password=mypass
```

## Advanced Configuration

### Adopting Existing Monitors

The operator can adopt and manage existing UptimeRobot monitors that were created outside of Kubernetes (via the UptimeRobot UI, API, or other tooling). This enables migration to the operator without recreating monitors and losing historical data.

To adopt an existing monitor, use the `uptimerobot.com/adopt-id` annotation with the monitor's ID:

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: legacy-api
  annotations:
    uptimerobot.com/adopt-id: "123456789"  # Your existing monitor ID
spec:
  prune: false  # Recommended: Preserve monitor if resource is deleted
  monitor:
    name: Legacy API Monitor
    url: https://api.example.com/health
    type: HTTPS
    interval: 5m
```

**Adoption workflow:**

1. Find your existing monitor ID from the UptimeRobot dashboard (visible in the URL when viewing a monitor)
2. Create a Monitor resource with the `uptimerobot.com/adopt-id` annotation
3. Set `prune: false` to prevent accidental deletion if the resource is removed
4. The operator will verify the monitor exists and adopt it
5. After successful adoption, the operator will update the monitor with your spec values

**Important notes:**

- The monitor ID must exist in your UptimeRobot account
- After adoption, subsequent reconciles will update the monitor to match your spec
- The annotation can be removed after successful adoption (ID is stored in status)
- Set `prune: false` to preserve the monitor if the Kubernetes resource is accidentally deleted

**Example: Migrating from UI to GitOps**

```bash
# Step 1: Find your monitor ID in UptimeRobot dashboard
MONITOR_ID=123456789

# Step 2: Create Monitor resource with adoption annotation
cat <<EOF | kubectl apply -f -
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: production-api
  annotations:
    uptimerobot.com/adopt-id: "$MONITOR_ID"
spec:
  prune: false
  monitor:
    name: Production API
    url: https://api.example.com/health
    type: HTTPS
    interval: 5m
EOF

# Step 3: Verify adoption succeeded
kubectl get monitor production-api -o jsonpath='{.status.id}'
# Output: 123456789

# Step 4: (Optional) Remove annotation after successful adoption
kubectl annotate monitor production-api uptimerobot.com/adopt-id-
```

### Custom HTTP Headers

```yaml
spec:
  monitor:
    name: API with Headers
    url: https://api.example.com
    customHttpHeaders:
      X-API-Key: "secret-key"
      User-Agent: "UptimeRobot/1.0"
```

### POST Requests

```yaml
spec:
  monitor:
    name: POST Endpoint
    url: https://api.example.com/webhook
    method: POST
    post:
      postType: RawData
      contentType: application/json
      value: '{"status":"ok"}'
```

### SSL Validation

```yaml
spec:
  monitor:
    name: Secure API
    url: https://api.example.com
    checkSSLErrors: true
    sslExpirationReminder: true
```

### Response Time Threshold

Alert when response time exceeds threshold:

```yaml
spec:
  monitor:
    name: Fast API
    url: https://api.example.com
    responseTimeThreshold: 1000  # milliseconds
```

### Regional Monitoring

Select a specific monitoring region:

```yaml
spec:
  monitor:
    name: Regional API
    url: https://api.example.com
    region: eu  # na, eu, as, oc
```

### Tags

Organise monitors with tags:

```yaml
spec:
  monitor:
    name: Production API
    url: https://api.example.com
    tags:
      - production
      - critical
      - api
```

## Troubleshooting

### Monitor Not Ready

Check the monitor status:

```bash
kubectl describe monitor my-website
```

Look for error messages in the status conditions.

### Alerts Not Working

Verify the contact is ready:

```bash
kubectl get contacts
```

Check operator logs:

```bash
kubectl logs -n uptime-robot-system deployment/uptime-robot-controller-manager
```

### Drift Detection

The operator reconciles every 24 hours by default. To force immediate reconciliation:

```bash
kubectl annotate monitor my-website reconcile=now
```

Or change the sync interval:

```yaml
spec:
  syncInterval: 5m
  monitor:
    name: My Website
    url: https://example.com
```

## Next Steps

- [Maintenance Windows](maintenance-windows.md) - Schedule planned downtime
- [API Reference](api-reference.md) - Complete field reference
