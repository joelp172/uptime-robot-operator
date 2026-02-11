# Troubleshooting Guide

This guide provides step-by-step solutions for common issues encountered with the Uptime Robot Operator.

## Table of Contents

- [General Debugging](#general-debugging)
- [API Key Issues](#api-key-issues)
- [Rate Limiting](#rate-limiting)
- [Finalizer Stuck](#finalizer-stuck)
- [Monitor Not Syncing](#monitor-not-syncing)
- [Webhook Certificate Issues](#webhook-certificate-issues)
- [Resource Stuck in "Not Ready"](#resource-stuck-in-not-ready)
- [Duplicate Monitors](#duplicate-monitors)
- [Account Dependencies](#account-dependencies)
- [Contact Dependencies](#contact-dependencies)
- [MaintenanceWindow Issues](#maintenancewindow-issues)

## General Debugging

### Check Resource Status

```bash
# Check Monitor status
kubectl get monitor -A
kubectl describe monitor <monitor-name> -n <namespace>

# Check Account status
kubectl get account
kubectl describe account <account-name>

# Check Contact status
kubectl get contact -A
kubectl describe contact <contact-name> -n <namespace>

# Check MaintenanceWindow status
kubectl get maintenancewindow -A
kubectl describe maintenancewindow <mw-name> -n <namespace>
```

### View Events

Events provide real-time feedback on operator actions:

```bash
# View events for a specific resource
kubectl get events --field-selector involvedObject.name=<resource-name> -n <namespace>

# View all operator events in namespace
kubectl get events -n <namespace> --sort-by='.lastTimestamp'

# Watch events in real-time
kubectl get events -n <namespace> --watch
```

### Check Controller Logs

```bash
# Get operator pod name
kubectl get pods -n uptime-robot-system

# View controller logs
kubectl logs -n uptime-robot-system <operator-pod-name> --follow

# Filter logs for specific controller
kubectl logs -n uptime-robot-system <operator-pod-name> | grep MonitorController

# View logs for specific resource
kubectl logs -n uptime-robot-system <operator-pod-name> | grep "monitor-name"
```

### Common Log Patterns

```bash
# Find errors
kubectl logs -n uptime-robot-system <operator-pod-name> | grep -i error

# Find rate limit messages
kubectl logs -n uptime-robot-system <operator-pod-name> | grep -i "rate limit"

# Find API failures
kubectl logs -n uptime-robot-system <operator-pod-name> | grep -i "api.*failed"

# Find drift detection
kubectl logs -n uptime-robot-system <operator-pod-name> | grep -i drift
```

## API Key Issues

### Symptoms

- Account status shows `Ready: false`
- Error messages like "Invalid API key" or "Unauthorized"
- Events show "Failed to authenticate with UptimeRobot API"
- Controller logs show 401 errors

### Diagnosis

```bash
# Check Account resource
kubectl describe account <account-name>

# Check Secret exists
kubectl get secret <secret-name> -n uptime-robot-system

# Verify Secret contains apiKey
kubectl get secret <secret-name> -n uptime-robot-system -o jsonpath='{.data}'
```

### Solutions

#### 1. Verify API Key Format

UptimeRobot API keys start with `u` or `m`:
- Main API key: `u<numbers>-<hash>` (e.g., `u1234567-abc123def456`)
- Monitor-specific API key: `m<numbers>-<hash>` (not recommended for operator use)

```bash
# Decode the API key from Secret
kubectl get secret <secret-name> -n uptime-robot-system -o jsonpath='{.data.apiKey}' | base64 -d
```

**Expected format:** `u1234567-abc123def456...`

#### 2. Recreate Secret with Valid Key

```bash
# Delete old secret
kubectl delete secret uptimerobot-api-key -n uptime-robot-system

# Create new secret with correct key
kubectl create secret generic uptimerobot-api-key \
  --namespace uptime-robot-system \
  --from-literal=apiKey=YOUR_VALID_API_KEY
```

#### 3. Update Account Reference

Ensure the Account references the correct Secret:

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: Account
metadata:
  name: default
spec:
  isDefault: true
  apiKeySecretRef:
    name: uptimerobot-api-key  # Must match Secret name
    key: apiKey                # Must match Secret key
```

#### 4. Verify API Key in UptimeRobot

1. Log in to [UptimeRobot](https://uptimerobot.com)
2. Go to **My Settings** → **API Settings**
3. Copy your **Main API Key**
4. Verify it matches the key in your Secret

#### 5. Test API Key Manually

```bash
# Test the API key directly
API_KEY=$(kubectl get secret uptimerobot-api-key -n uptime-robot-system -o jsonpath='{.data.apiKey}' | base64 -d)

curl -X POST https://api.uptimerobot.com/v2/getAccountDetails \
  -H "Content-Type: application/json" \
  -d "{\"api_key\": \"${API_KEY}\"}"
```

**Expected response:** Account details with email and contact information.  
**Error response:** `{"stat": "fail", "error": {"message": "api_key is wrong"}}`

## Rate Limiting

### Symptoms

- Intermittent reconciliation failures
- Controller logs show "rate limit exceeded" or HTTP 429 errors
- Monitors sync slowly or sporadically
- Events show "Too many API requests"

### Diagnosis

```bash
# Check controller logs for rate limit messages
kubectl logs -n uptime-robot-system <operator-pod-name> | grep -i "rate\|429"

# Check syncInterval settings
kubectl get monitors -A -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.spec.syncInterval}{"\n"}{end}'
```

### UptimeRobot API Limits

- **Free accounts:** 10 requests per minute
- **Pro accounts:** Higher limits (varies by plan)

### Solutions

#### 1. Increase syncInterval

Reduce API call frequency by increasing `syncInterval`:

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: my-monitor
spec:
  syncInterval: 24h  # Default: 24h, adjust as needed (e.g., 48h, 72h)
  monitor:
    name: My Monitor
    url: https://example.com
```

```bash
# Patch all monitors to use longer sync interval
kubectl get monitors -A -o name | xargs -I {} kubectl patch {} --type=merge -p '{"spec":{"syncInterval":"48h"}}'
```

#### 2. Stagger Monitor Creation

Avoid creating many monitors simultaneously:

```bash
# Create monitors with delays
for monitor in monitor1 monitor2 monitor3; do
  kubectl apply -f ${monitor}.yaml
  sleep 10  # Wait 10 seconds between creates
done
```

#### 3. Check for Unnecessary Reconciliations

Look for rapid reconciliation loops:

```bash
# Monitor reconciliation frequency
kubectl logs -n uptime-robot-system <operator-pod-name> --tail=100 | grep "Reconciling Monitor"
```

If you see the same monitor reconciling every few seconds, check for:
- Spec changes causing updates
- Status thrashing
- Webhook validation failures

#### 4. Upgrade UptimeRobot Account

Consider upgrading to a Pro account for higher rate limits.

## Finalizer Stuck

### Symptoms

- Resource stuck in "Terminating" state
- `kubectl delete` hangs or times out
- `deletionTimestamp` set but resource not removed
- Finalizer present in metadata

### Diagnosis

```bash
# Check if resource is stuck terminating
kubectl get monitors -A | grep Terminating

# Check finalizers
kubectl get monitor <monitor-name> -n <namespace> -o jsonpath='{.metadata.finalizers}'
```

### Solutions

#### 1. Check Controller is Running

```bash
# Verify operator pod is running
kubectl get pods -n uptime-robot-system

# If not running, check deployment
kubectl get deployment -n uptime-robot-system
kubectl describe deployment uptime-robot-operator -n uptime-robot-system
```

#### 2. Check Controller Logs

The controller should process the deletion:

```bash
kubectl logs -n uptime-robot-system <operator-pod-name> | grep "delete\|finalizer"
```

#### 3. Force Remove Finalizer (Last Resort)

⚠️ **Warning:** This bypasses the controller's cleanup logic. The monitor may remain in UptimeRobot.

```bash
# Patch to remove finalizer
kubectl patch monitor <monitor-name> -n <namespace> \
  --type json \
  -p='[{"op": "remove", "path": "/metadata/finalizers"}]'
```

#### 4. Manual Cleanup

If you force-removed the finalizer but the monitor still exists in UptimeRobot:

1. Log in to [UptimeRobot](https://uptimerobot.com)
2. Go to **Monitors**
3. Find and delete the orphaned monitor manually
4. Or use the API:

```bash
API_KEY="your-api-key"
MONITOR_ID="123456789"

curl -X POST https://api.uptimerobot.com/v2/deleteMonitor \
  -H "Content-Type: application/json" \
  -d "{\"api_key\": \"${API_KEY}\", \"id\": \"${MONITOR_ID}\"}"
```

## Monitor Not Syncing

### Symptoms

- Monitor status shows `Ready: false` or remains stale
- Changes to spec not reflected in UptimeRobot
- Last sync time is outdated
- No events generated

### Diagnosis

```bash
# Check Monitor status
kubectl describe monitor <monitor-name> -n <namespace>

# Check events
kubectl get events --field-selector involvedObject.name=<monitor-name> -n <namespace>

# Check controller logs
kubectl logs -n uptime-robot-system <operator-pod-name> | grep <monitor-name>
```

### Common Causes and Solutions

#### 1. Account Not Ready

**Check:**
```bash
kubectl get account <account-name> -o jsonpath='{.status.ready}'
```

**Fix:** Resolve Account issues first (see [Account Dependencies](#account-dependencies))

#### 2. Contacts Not Ready

**Check:**
```bash
# List contacts referenced in Monitor spec
kubectl get monitor <monitor-name> -n <namespace> -o jsonpath='{.spec.contacts[*].name}'

# Check each contact's status
kubectl get contact <contact-name> -n <namespace> -o jsonpath='{.status.ready}'
```

**Fix:** Ensure all referenced Contacts are ready (see [Contact Dependencies](#contact-dependencies))

#### 3. Invalid Monitor Configuration

**Check validation errors:**
```bash
kubectl describe monitor <monitor-name> -n <namespace> | grep -A 5 "Events:"
```

**Common issues:**
- Invalid URL format
- Unsupported monitor type
- Missing required fields
- Invalid interval value

**Fix:** Correct the spec according to [API Reference](api-reference.md)

#### 4. API Connection Issues

**Check controller logs for API errors:**
```bash
kubectl logs -n uptime-robot-system <operator-pod-name> | grep -i "api.*error"
```

**Possible causes:**
- Network connectivity issues
- UptimeRobot API outage
- Invalid API key
- Rate limiting

#### 5. Reconciliation Loop Disabled

**Check syncInterval:**
```bash
kubectl get monitor <monitor-name> -n <namespace> -o jsonpath='{.spec.syncInterval}'
```

If `syncInterval` is very high (e.g., `9999h`), the monitor won't sync frequently.

**Fix:** Set a reasonable `syncInterval`:
```bash
kubectl patch monitor <monitor-name> -n <namespace> \
  --type merge \
  -p '{"spec":{"syncInterval":"24h"}}'
```

#### 6. Force Immediate Reconciliation

Trigger reconciliation by updating an annotation:

```bash
kubectl annotate monitor <monitor-name> -n <namespace> \
  reconcile="$(date +%s)" --overwrite
```

## Webhook Certificate Issues

### Symptoms

- Cannot create or update resources
- Error: "Internal error occurred: failed calling webhook"
- Error: "x509: certificate signed by unknown authority"
- ValidatingWebhookConfiguration or MutatingWebhookConfiguration errors

### Diagnosis

```bash
# Check webhook configurations
kubectl get validatingwebhookconfigurations
kubectl get mutatingwebhookconfigurations

# Check cert-manager is running
kubectl get pods -n cert-manager

# Check certificates
kubectl get certificate -n uptime-robot-system
kubectl describe certificate -n uptime-robot-system
```

### Solutions

#### 1. Verify cert-manager is Installed

The operator requires cert-manager for webhook certificates.

```bash
# Check cert-manager pods
kubectl get pods -n cert-manager

# If not installed, install cert-manager
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.14.0/cert-manager.yaml
```

#### 2. Check Certificate Status

```bash
# Get certificate status
kubectl get certificate -n uptime-robot-system

# Describe certificate for issues
kubectl describe certificate uptime-robot-operator-serving-cert -n uptime-robot-system
```

**Expected status:** `READY: True`

#### 3. Delete and Recreate Certificate

If the certificate is not ready:

```bash
# Delete certificate (cert-manager will recreate)
kubectl delete certificate uptime-robot-operator-serving-cert -n uptime-robot-system

# Wait for recreation (takes a few seconds)
kubectl wait --for=condition=Ready certificate/uptime-robot-operator-serving-cert -n uptime-robot-system --timeout=60s
```

#### 4. Restart Operator

After certificate is ready, restart the operator:

```bash
kubectl rollout restart deployment uptime-robot-operator -n uptime-robot-system
kubectl rollout status deployment uptime-robot-operator -n uptime-robot-system
```

#### 5. Verify Webhook Service

```bash
# Check webhook service exists
kubectl get service uptime-robot-operator-webhook-service -n uptime-robot-system

# Check endpoints
kubectl get endpoints uptime-robot-operator-webhook-service -n uptime-robot-system
```

#### 6. Test Webhook Directly

```bash
# Get webhook port
kubectl get service uptime-robot-operator-webhook-service -n uptime-robot-system

# Port-forward to webhook
kubectl port-forward -n uptime-robot-system svc/uptime-robot-operator-webhook-service 9443:443

# In another terminal, test (should get 404 for root path, which is OK)
curl -k https://localhost:9443/
```

#### 7. Disable Webhooks Temporarily (Emergency)

⚠️ **Warning:** This disables validation and defaulting. Use only for emergency recovery.

```bash
# Delete webhook configurations
kubectl delete validatingwebhookconfiguration uptime-robot-operator-validating-webhook-configuration
kubectl delete mutatingwebhookconfiguration uptime-robot-operator-mutating-webhook-configuration
```

To re-enable, reinstall the operator or reapply the webhook configurations.

## Resource Stuck in "Not Ready"

### Symptoms

- Resource status shows `Ready: false` indefinitely
- Status conditions indicate dependency issues
- No progress despite waiting

### Diagnosis

```bash
# Check resource status and conditions
kubectl describe <resource-type> <resource-name> -n <namespace>

# Check status specifically
kubectl get <resource-type> <resource-name> -n <namespace> -o jsonpath='{.status}'
```

### Solutions

#### Account Not Ready

**Check:**
```bash
kubectl get account <account-name> -o yaml
```

**Common issues:**
- Invalid API key (see [API Key Issues](#api-key-issues))
- Secret not found or missing key
- Network/API connectivity issues

**Fix:**
```yaml
# Ensure Account spec is correct
apiVersion: uptimerobot.com/v1alpha1
kind: Account
metadata:
  name: default
spec:
  isDefault: true
  apiKeySecretRef:
    name: uptimerobot-api-key  # Must exist
    key: apiKey                # Must exist in Secret
```

#### Contact Not Ready

**Check:**
```bash
kubectl describe contact <contact-name> -n <namespace>
```

**Common issues:**
- Account not ready
- Contact ID doesn't exist in UptimeRobot account
- Contact ID format invalid

**Fix:**
```bash
# Get available contacts from Account
kubectl get account <account-name> -o jsonpath='{.status.alertContacts[*].id}'

# Use valid contact ID in Contact resource
apiVersion: uptimerobot.com/v1alpha1
kind: Contact
metadata:
  name: my-contact
spec:
  contact:
    id: "1234567"  # Must match an ID from Account.status.alertContacts
```

#### Monitor Not Ready

**Check dependencies:**
```bash
# Check Account
kubectl get account <account-name> -o jsonpath='{.status.ready}'

# Check Contacts
kubectl get contact -n <namespace>
```

**Verify all dependencies are ready before expecting Monitor to become ready.**

#### Dependency Resolution Order

Resources must be created in this order:

1. **Account** (with valid API key Secret)
2. **Contact** (after Account is ready)
3. **Monitor** (after Account and Contacts are ready)
4. **MaintenanceWindow** (after Account is ready; Monitors should exist)

```bash
# Check readiness in order
kubectl get account
kubectl get contact -A
kubectl get monitor -A
kubectl get maintenancewindow -A
```

## Duplicate Monitors

### Symptoms

- Multiple monitors with the same name in UptimeRobot
- Monitor spec doesn't match UptimeRobot state
- Adoption annotation not working
- Multiple Monitor resources pointing to the same UptimeRobot monitor

### Diagnosis

```bash
# List monitors with IDs
kubectl get monitors -A -o custom-columns=NAME:.metadata.name,NAMESPACE:.metadata.namespace,ID:.status.id,READY:.status.ready

# Check for adoption annotation
kubectl get monitor <monitor-name> -n <namespace> -o jsonpath='{.metadata.annotations}'
```

### Understanding Monitor Adoption

The operator supports adopting existing UptimeRobot monitors using the `uptimerobot.com/adopt-id` annotation. This prevents duplicates when migrating existing monitors to Kubernetes management.

### Solutions

#### 1. Adopt Existing Monitor

If you have an existing monitor in UptimeRobot you want to manage with Kubernetes:

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: existing-monitor
  annotations:
    uptimerobot.com/adopt-id: "123456789"  # Your existing monitor ID
spec:
  prune: false  # Recommended: Don't delete if K8s resource is removed
  monitor:
    name: Existing Monitor
    url: https://example.com
```

**Steps:**
1. Get the monitor ID from UptimeRobot (visible in the monitor's details)
2. Create the Monitor resource with the `uptimerobot.com/adopt-id` annotation
3. The operator will verify the monitor exists and adopt it
4. After successful adoption, the operator updates the monitor to match your spec
5. The annotation can be removed after adoption (ID is stored in `status.id`)

#### 2. Find Monitor ID in UptimeRobot

**Via UI:**
1. Log in to [UptimeRobot](https://uptimerobot.com)
2. Go to **Monitors**
3. Click on the monitor
4. The ID is in the URL: `https://uptimerobot.com/dashboard.php#MONITOR_ID`

**Via API:**
```bash
API_KEY="your-api-key"

curl -X POST https://api.uptimerobot.com/v2/getMonitors \
  -H "Content-Type: application/json" \
  -d "{\"api_key\": \"${API_KEY}\", \"format\": \"json\"}" \
  | jq '.monitors[] | {id, friendly_name, url}'
```

#### 3. Delete Duplicate Monitors

If you accidentally created duplicates:

**Option A: Keep UptimeRobot monitor, adopt it**
```bash
# Delete K8s resource (won't delete from UptimeRobot if prune=false)
kubectl delete monitor <duplicate-monitor> -n <namespace>

# Recreate with adoption
kubectl apply -f - <<EOF
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: <monitor-name>
  annotations:
    uptimerobot.com/adopt-id: "<monitor-id>"
spec:
  prune: false
  monitor:
    name: My Monitor
    url: https://example.com
EOF
```

**Option B: Delete from UptimeRobot and K8s, start fresh**
```bash
# Ensure prune is enabled
kubectl patch monitor <monitor-name> -n <namespace> \
  --type merge \
  -p '{"spec":{"prune":true}}'

# Delete K8s resource (will delete from UptimeRobot)
kubectl delete monitor <monitor-name> -n <namespace>

# Verify deletion in UptimeRobot
# Then create new monitor without adoption
```

#### 4. Prevent Duplicate Creation

- Always check if a monitor exists in UptimeRobot before creating
- Use adoption for existing monitors
- Set `prune: false` if you want to preserve monitors when K8s resources are deleted

#### 5. Handle Multiple Adopters

If multiple Monitor resources try to adopt the same ID:

```bash
# Find monitors with same ID
kubectl get monitors -A -o json | jq '.items[] | select(.status.id=="123456789") | {name: .metadata.name, namespace: .metadata.namespace}'
```

**Resolution:**
- Keep only one Monitor resource per UptimeRobot monitor ID
- Delete extra Monitor resources
- Ensure the remaining Monitor has the correct spec

## Account Dependencies

### Problem: Resources Can't Find Account

**Symptoms:**
- Monitor, Contact, or MaintenanceWindow shows "Account not found"
- Status shows `Ready: false` with Account-related error

**Diagnosis:**
```bash
# List all accounts
kubectl get account

# Check resource's account reference
kubectl get monitor <monitor-name> -n <namespace> -o jsonpath='{.spec.account.name}'
```

**Solution:**

Accounts are cluster-scoped. Reference them by name:

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: my-monitor
  namespace: default
spec:
  account:
    name: default  # References cluster-scoped Account named "default"
  monitor:
    name: My Monitor
    url: https://example.com
```

If `account.name` is empty, the operator looks for an Account with `isDefault: true`:

```bash
# Check for default account
kubectl get account -o jsonpath='{.items[?(@.spec.isDefault==true)].metadata.name}'
```

**Create a default Account:**
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

### Problem: Multiple Default Accounts

**Symptoms:**
- Warning in logs: "Multiple default accounts found"
- Unpredictable Account selection

**Diagnosis:**
```bash
# Find all default accounts
kubectl get account -o jsonpath='{.items[?(@.spec.isDefault==true)].metadata.name}'
```

**Solution:**

Only one Account should have `isDefault: true`:

```bash
# Remove default from all accounts
kubectl get account -o name | xargs -I {} kubectl patch {} --type=merge -p '{"spec":{"isDefault":false}}'

# Set one account as default
kubectl patch account default --type=merge -p '{"spec":{"isDefault":true}}'
```

## Contact Dependencies

### Problem: Contact ID Not Found

**Symptoms:**
- Contact status shows `Ready: false`
- Error: "Contact ID does not exist in Account"
- Events show "Invalid contact ID"

**Diagnosis:**
```bash
# Check Contact resource
kubectl describe contact <contact-name> -n <namespace>

# Check Account status for available contacts
kubectl get account <account-name> -o jsonpath='{.status.alertContacts}'
```

**Root Cause:**

Contact resources reference existing alert contacts in your UptimeRobot account. You must first create the contact in UptimeRobot (via UI or API), then reference its ID in the Contact resource.

**Solution:**

#### Step 1: Find Available Contact IDs

```bash
# List all alert contacts from Account
kubectl get account <account-name> -o jsonpath='{.status.alertContacts[*]}' | jq .
```

Example output:
```json
[
  {
    "id": "1234567",
    "friendlyName": "John's Email",
    "type": "EMAIL",
    "value": "john@example.com"
  },
  {
    "id": "2345678",
    "friendlyName": "Ops Slack",
    "type": "SLACK",
    "value": "https://hooks.slack.com/..."
  }
]
```

#### Step 2: Create Contact with Valid ID

```yaml
apiVersion: uptimerobot.com/v1alpha1
kind: Contact
metadata:
  name: ops-slack
  namespace: default
spec:
  contact:
    id: "2345678"  # Use ID from Account.status.alertContacts
```

#### Step 3: Add Contacts in UptimeRobot UI

If no contacts exist in your Account status:

1. Log in to [UptimeRobot](https://uptimerobot.com)
2. Go to **My Settings** → **Alert Contacts**
3. Add a new alert contact (email, SMS, Slack, etc.)
4. Wait for Account reconciliation to update (or force it):
   ```bash
   kubectl annotate account <account-name> reconcile="$(date +%s)" --overwrite
   ```
5. Verify contact appears in Account status:
   ```bash
   kubectl get account <account-name> -o jsonpath='{.status.alertContacts}'
   ```

### Problem: Contact Not Ready, Account is Ready

**Diagnosis:**
```bash
# Check Account is ready
kubectl get account <account-name> -o jsonpath='{.status.ready}'

# Check Contact details
kubectl describe contact <contact-name> -n <namespace>
```

**Possible causes:**
1. Contact ID doesn't exist in Account
2. Contact ID format is incorrect (should be a string of digits)
3. API issues preventing Contact validation

**Solution:**
```bash
# Verify Contact ID exists in Account
CONTACT_ID=$(kubectl get contact <contact-name> -n <namespace> -o jsonpath='{.spec.contact.id}')
kubectl get account <account-name> -o jsonpath='{.status.alertContacts[?(@.id=="'$CONTACT_ID'")]}'
```

If empty, the Contact ID doesn't exist. Choose a valid ID from the Account status.

## MaintenanceWindow Issues

### Problem: MaintenanceWindow Not Ready

**Symptoms:**
- MaintenanceWindow status shows `Ready: false`
- Events show dependency or validation errors

**Diagnosis:**
```bash
kubectl describe maintenancewindow <mw-name> -n <namespace>
```

### Common Issues and Solutions

#### 1. Account Not Ready

**Check:**
```bash
kubectl get account <account-name> -o jsonpath='{.status.ready}'
```

**Fix:** Resolve Account issues first (see [Account Dependencies](#account-dependencies))

#### 2. Referenced Monitors Don't Exist

**Check:**
```bash
# List referenced monitors
kubectl get maintenancewindow <mw-name> -n <namespace> -o jsonpath='{.spec.monitorRefs[*].name}'

# Check if they exist
kubectl get monitor <monitor-name> -n <namespace>
```

**Fix:** Create referenced monitors first, or remove them from `monitorRefs` if not needed.

**Note:** Referenced monitors don't need to be ready, they just need to exist.

#### 3. Invalid Interval Configuration

**Common mistakes:**
- `days` field missing for weekly/monthly intervals
- `days` field present for once/daily intervals  
- `startDate` missing for once interval
- Invalid day values (weekly: 0-6, monthly: 1-31 or -1)

**Example validation error:**
```
error: days field is required and must not be empty when interval is weekly or monthly
```

**Fix:**

```yaml
# Weekly interval - requires days
apiVersion: uptimerobot.com/v1alpha1
kind: MaintenanceWindow
metadata:
  name: weekly-maintenance
spec:
  interval: weekly
  days: [0, 6]  # Sunday and Saturday
  startTime: "02:00:00"
  duration: 2h

---
# Monthly interval - requires days
apiVersion: uptimerobot.com/v1alpha1
kind: MaintenanceWindow
metadata:
  name: monthly-maintenance
spec:
  interval: monthly
  days: [1, 15, -1]  # 1st, 15th, and last day of month
  startTime: "03:00:00"
  duration: 4h

---
# Once interval - requires startDate
apiVersion: uptimerobot.com/v1alpha1
kind: MaintenanceWindow
metadata:
  name: planned-maintenance
spec:
  interval: once
  startDate: "2026-03-15"
  startTime: "10:00:00"
  duration: 6h

---
# Daily interval - no days field
apiVersion: uptimerobot.com/v1alpha1
kind: MaintenanceWindow
metadata:
  name: daily-backup
spec:
  interval: daily
  startTime: "01:00:00"
  duration: 1h
```

#### 4. Monitor Assignment Issues

**Problem:** Monitors not assigned to maintenance window

**Check:**
```bash
# Check monitor count
kubectl get maintenancewindow <mw-name> -n <namespace> -o jsonpath='{.status.monitorCount}'
```

**Causes:**
- Referenced monitors don't exist
- `autoAddMonitors` is false and `monitorRefs` is empty
- Monitors don't have IDs yet (not ready)

**Fix:**

Use `autoAddMonitors` to automatically add all monitors:
```yaml
spec:
  autoAddMonitors: true
```

Or explicitly reference monitors:
```yaml
spec:
  monitorRefs:
    - name: api-monitor
    - name: web-monitor
```

#### 5. Time Format Issues

**Symptoms:**
- Validation error about time format
- Error creating maintenance window

**Fix:**

Ensure correct formats:
- `startTime`: `HH:mm:ss` (e.g., `"09:30:00"`, `"23:45:00"`)
- `startDate`: `YYYY-MM-DD` (e.g., `"2026-12-31"`)
- `duration`: Go duration (e.g., `30m`, `1h`, `2h30m`)

```yaml
spec:
  startTime: "09:30:00"      # NOT "9:30" or "09:30"
  startDate: "2026-03-15"    # NOT "2026-3-15" or "03/15/2026"
  duration: 2h30m            # NOT "2.5h" or "150m"
```

## Advanced Debugging

### Enable Verbose Logging

Edit the operator deployment to increase log verbosity:

```bash
kubectl edit deployment uptime-robot-operator -n uptime-robot-system
```

Add or modify the `--zap-log-level` flag:

```yaml
containers:
- name: manager
  args:
  - --health-probe-bind-address=:8081
  - --metrics-bind-address=:8080
  - --zap-log-level=debug  # Change from 'info' to 'debug'
```

### Check API Communication

Use a debug pod to test UptimeRobot API connectivity:

```bash
# Create debug pod
kubectl run -it --rm debug --image=curlimages/curl --restart=Never -- sh

# In the pod, test API
API_KEY="your-api-key"
curl -X POST https://api.uptimerobot.com/v2/getAccountDetails \
  -H "Content-Type: application/json" \
  -d "{\"api_key\": \"${API_KEY}\"}"
```

### Inspect Resource YAML

Get the full resource definition to see all fields:

```bash
# Get full resource YAML
kubectl get monitor <monitor-name> -n <namespace> -o yaml

# Show only status
kubectl get monitor <monitor-name> -n <namespace> -o jsonpath='{.status}' | jq .

# Show only spec
kubectl get monitor <monitor-name> -n <namespace> -o jsonpath='{.spec}' | jq .
```

### Check Resource Watch Events

The controller watches for resource changes. Verify events are triggering:

```bash
# Watch Monitor changes
kubectl get monitors -A --watch

# In another terminal, update a monitor
kubectl annotate monitor <monitor-name> -n <namespace> test=value

# You should see the update in the watch output
```

## Getting Help

If you've tried the solutions above and still have issues:

1. **Check existing issues:** [GitHub Issues](https://github.com/joelp172/uptime-robot-operator/issues)
2. **Gather diagnostic info:**
   ```bash
   # Create a diagnostic bundle
   kubectl get monitors -A -o yaml > monitors.yaml
   kubectl get accounts -o yaml > accounts.yaml
   kubectl get contacts -A -o yaml > contacts.yaml
   kubectl logs -n uptime-robot-system deployment/uptime-robot-operator --tail=500 > operator.log
   kubectl get events -A --sort-by='.lastTimestamp' | tail -50 > events.txt
   ```
3. **Open a new issue:** Include your diagnostic info, steps to reproduce, and expected vs actual behavior
4. **Redact sensitive data:** Remove API keys, URLs, and other sensitive information before sharing

## See Also

- [Architecture](architecture.md) - Understand how the operator works
- [API Reference](api-reference.md) - Complete field documentation
- [Getting Started](getting-started.md) - Setup tutorial
- [Development](development.md) - Contributing guide
