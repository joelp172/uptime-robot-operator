# E2E Test Guide

## Test Execution Flow

### Overview

The `make test-e2e-real` command runs end-to-end tests against the real UptimeRobot API. Tests are executed in this order:

1. **Suite Setup** (BeforeSuite)
2. **CRD Reconciliation Tests** (ordered)
3. **MaintenanceWindow Tests** (ordered)
4. **Suite Teardown** (AfterSuite)

### Detailed Test Flow

#### 1. Suite Setup (BeforeSuite)

```bash
make test-e2e-real
```

1. Build operator Docker image
2. Load image into Kind cluster
3. Install CRDs (`make install`)
4. Deploy operator (`make deploy`)
5. Verify operator pod is running

#### 2. CRD Reconciliation Tests

**Setup (BeforeAll):**
- Create namespace `uptime-robot-system`
- Label namespace with pod security policy
- Create Secret with `UPTIME_ROBOT_API_KEY`
- Create Account CR (references secret)
- Wait for Account to be ready
- Get first alert contact ID from Account status
- Create default Contact CR (references existing contact)
- Wait for Contact to be ready

**Tests executed in order:**

1. **Account and Contact Setup**
   - Validates Account CR is ready
   - Validates Contact CR is ready with ID

2. **Monitor - HTTP Basic**
   - Create HTTP monitor
   - Wait for status.ready = true
   - Verify monitor exists in API
   - Validate fields match spec
   - Delete monitor, verify API cleanup

3. **Monitor - HTTPS Full**
   - Create HTTPS monitor with all fields
   - Validate timeout, HTTP method, custom headers
   - Verify SSL validation settings

4. **Monitor - HTTPS Auth**
   - Create HTTPS monitor with Basic auth
   - Validate auth credentials are sent to API

5. **Monitor - HTTPS POST**
   - Create HTTPS monitor with POST method
   - Validate request body is sent

6. **Monitor - Keyword Exists**
   - Create Keyword monitor (case-sensitive)
   - Validate keyword check config

7. **Monitor - Keyword NotExists**
   - Create Keyword monitor with NotExists check
   - Validate inverse keyword logic

8. **Monitor - Ping**
   - Create Ping monitor with IP address
   - Validate ping-specific config

9. **Monitor - Port**
   - Create Port monitor with host:port URL
   - Validate port number in API

10. **Monitor - DNS**
    - Create DNS monitor
    - Validate DNS record checks

11. **Monitor - Contact Assignment**
    - Create monitor with contact reference
    - Validate alert threshold and recurrence

**Cleanup (AfterAll):**
- Delete all test monitors
- Delete Contact CR
- Delete Account CR
- Delete Secret

#### 3. MaintenanceWindow Tests

**Setup (BeforeAll):**
- Create Secret with API key (separate from Monitor tests)
- Create Account CR
- Wait for Account ready
- Get first contact ID from Account
- Create Contact CR for maintenance window tests
- Wait for Contact ready

**Tests executed in order:**

1. **Once Interval**
   - Create maintenance window with interval=once
   - Validate startDate is used
   - Verify API accepts once interval

2. **Daily Interval**
   - Create daily maintenance window
   - Verify startDate is NOT sent to API (daily doesn't use it)

3. **Weekly Interval**
   - Create weekly maintenance window with days=[1,3,5]
   - Validate days are set correctly

4. **Monthly Interval**
   - Create monthly maintenance window with days=[-1] (last day)
   - Validate month-end handling

5. **Update - Name Change**
   - Create maintenance window
   - Update name field
   - Wait for status.ready = true
   - Verify update propagated to API

6. **Update - Schedule Change**
   - Create daily maintenance window
   - Update to weekly interval with days
   - Wait for status.ready = true
   - Verify interval change in spec

7. **Delete - With Prune=true**
   - Create maintenance window with prune=true
   - Delete CR
   - Verify removed from API

8. **Delete - With Prune=false**
   - Create maintenance window with prune=false
   - Delete CR
   - Verify remains in API (not deleted)

9. **Monitor References - Add Monitors**
   - Create 2 monitors
   - Create maintenance window with monitorRefs
   - Verify monitors are associated

10. **Monitor References - Update**
    - Create maintenance window with 1 monitor
    - Update to include 2 monitors
    - Verify monitor list updated

11. **Monitor References - Remove All**
    - Create maintenance window with monitors
    - Update monitorRefs to empty array
    - Verify all monitors removed

12. **Duration Handling**
    - Create maintenance window with fractional duration
    - Verify duration rounded correctly

**Cleanup (AfterAll):**
- Delete all test maintenance windows (bulk cleanup)
- Delete Contact CR
- Delete Account CR
- Delete Secret

#### 4. Suite Teardown (AfterSuite)

- Undeploy operator
- Uninstall CRDs
- Clean up namespaces

### Test Characteristics

**Ordered Execution:**
- Tests within each Context run sequentially
- BeforeAll/AfterAll run once per Context
- BeforeEach/AfterEach run around each test

**Resource Naming:**
- All resources use `testRunID` (timestamp) to avoid conflicts
- Format: `e2e-{resource-type}-e2e-{timestamp}`
- Example: `e2e-http-e2e-1770280022`

**Timeouts:**
- Poll timeout: 3 minutes (`e2ePollTimeout`)
- Poll interval: 5 seconds (`e2ePollInterval`)
- Tests use `Eventually()` with these values

### Common Test Patterns

**Create and Validate:**
```go
applyMonitor(yaml)           // Apply CR
waitMonitorReadyAndGetID()   // Wait for status.ready=true
getMonitorFromAPI()          // Query UptimeRobot API
ValidateMonitorFields()      // Compare CR spec vs API response
```

**Update and Validate:**
```go
applyMonitor(yaml)              // Create initial
waitMonitorReady()              // Wait for ready
applyMonitor(updatedYaml)       // Update spec
waitMonitorReady()              // Wait for reconciliation
// Verify update in API
```

**Delete and Cleanup:**
```go
deleteMonitor()                       // Delete CR
waitForMonitorDeletionFromAPI()      // Verify API cleanup (if prune=true)
```

## Debug Logging

The e2e tests include conditional debug logging that can be enabled via the `E2E_DEBUG` environment variable.

### Enabling Debug Logging

Set `E2E_DEBUG` to any of these values: `1`, `true`, or `yes`

```bash
# Run tests with debug logging
E2E_DEBUG=1 make test-e2e-real

# Or export it first
export E2E_DEBUG=1
make test-e2e-real
```

### What Gets Logged

When debug logging is enabled, you'll see detailed output for:

1. **API Calls**
   - Monitor ID being queried
   - API endpoint URL being used
   - Success/failure status

2. **Monitor Details**
   - FriendlyName, URL, Type
   - Interval, HTTP method
   - Contact assignments

3. **Field Validation**
   - Expected vs actual values for each field
   - Which validations passed/failed
   - Specific mismatches

### Example Debug Output

```
[DEBUG] Calling GetMonitor for ID=802290021
[DEBUG] Using API endpoint: https://api.uptimerobot.com/v3
[DEBUG] GetMonitor succeeded: Name=E2E Test HTTP Monitor, URL=https://example.com, Type=HTTP, Interval=300, Method=HEAD
[DEBUG] Contacts=1, First contact alertContactId=6.121911e+06 (type=float64)
[DEBUG] Validating HTTPS monitor fields: want(name="E2E Test HTTP Monitor", url="https://example.com", type="HTTP", interval=300, method="HEAD") got(name="E2E Test HTTP Monitor", url="https://example.com", type="HTTP", interval=300, method="HEAD")
[DEBUG] Validation passed
```

### Troubleshooting Common Issues

#### Test hangs at "verifying monitor fields"

Enable debug logging to see:
- If API calls are succeeding
- What the actual API response contains
- Which field validations are failing

```bash
E2E_DEBUG=1 make test-e2e-real
```

#### API timeouts

Debug logs will show:
- The API endpoint being used
- When GetMonitor calls fail
- Error messages from the API

#### Field validation failures

Debug logs show exact comparisons:
```
[DEBUG] Validating HTTPS monitor fields: want(...) got(...)
[DEBUG] Validation failed with 2 error(s)
```

### Disabling Debug Logging

Debug logging is disabled by default. To explicitly disable:

```bash
# Unset the variable
unset E2E_DEBUG

# Or set to empty/false
E2E_DEBUG= make test-e2e-real
E2E_DEBUG=0 make test-e2e-real
E2E_DEBUG=false make test-e2e-real
```

## Implementation Details

Debug logging is implemented in `test/e2e/api_helpers.go`:

- `debugEnabled()` - checks if E2E_DEBUG is set to a truthy value
- `debugLog()` - prints formatted messages to GinkgoWriter when enabled
- All API helper functions include debug logging at key points

The logging uses `GinkgoWriter` so output is synchronized with test output and only shown when tests run with `-v` (verbose) flag or when tests fail.
