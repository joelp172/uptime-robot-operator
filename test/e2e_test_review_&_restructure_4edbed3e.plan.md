---
name: E2E Test Review & Restructure
overview: Review the current e2e test structure, identify gaps between expected and actual behavior, and propose splitting tests by controller type for better isolation and comprehensive API validation.
todos:
  - id: add-mw-api-helpers
    content: Add getMaintenanceWindowFromAPI() and WaitForMaintenanceWindowDeletedFromAPI() helpers
    status: completed
  - id: split-test-files
    content: Split crd_reconciliation_test.go into monitor_test.go and keep MW separate
    status: completed
  - id: enhance-monitor-validation
    content: Add comprehensive validation of ALL response fields for each monitor type
    status: completed
  - id: enhance-mw-validation
    content: Add API-level validation of MW fields and bidirectional monitor association
    status: completed
  - id: add-ginkgo-labels
    content: Add controller-specific labels for test isolation (monitor, maintenancewindow, account, contact)
    status: completed
isProject: false
---

# E2E Test Review and Restructure Plan

## Current State Analysis

### Monitor Tests ([test/e2e/crd_reconciliation_test.go](test/e2e/crd_reconciliation_test.go))

**What Currently Happens:**

1. Creates monitors with SOME parameters (not all available)
2. Waits for `status.ready == true`
3. Validates SOME API response fields via `getMonitorFromAPI()`
4. Deletes and verifies API cleanup

**Gaps Identified:**


| Monitor Type | Parameters Tested                                                    | Parameters Missing                                                         |
| ------------ | -------------------------------------------------------------------- | -------------------------------------------------------------------------- |
| HTTP/HTTPS   | name, url, interval, method, auth(basic), POST, headers, SSL options | Digest auth, PUT/PATCH/DELETE/OPTIONS, POST KeyValue type, region, groupId |
| Keyword      | type (Exists/NotExists), value, caseSensitive                        | Case-insensitive NotExists combination                                     |
| Ping         | url (IP address), interval                                           | Hostname URLs                                                              |
| Port         | url:port, port number                                                | Different port types (HTTP/FTP/SMTP/etc)                                   |
| DNS          | A records only                                                       | AAAA, CNAME, MX, NS, TXT, SRV, sslExpirationPeriodDays                     |
| Heartbeat    | interval                                                             | gracePeriod variations                                                     |


**Validation Gaps:**

- `responseTimeThreshold` not validated in response
- `tags` not validated in response
- `maintenanceWindowIds` not validated
- `successHttpResponseCodes` not always validated

### Maintenance Window Tests ([test/e2e/maintenancewindow_test.go](test/e2e/maintenancewindow_test.go))

**What Currently Happens:**

1. Creates MW with interval type parameters
2. Waits for `status.ready == true`
3. Checks `status.id` exists
4. Does NOT call API to verify MW details
5. Monitor association test creates monitors, adds via `monitorRefs`, but only checks `status.monitorCount` exists

**Critical Gaps:**

- No API-level validation of MW fields (duration, time, days, etc.)
- Monitor association is NOT verified via API (never calls `GetMaintenanceWindow()`)
- No test that creates a monitor WITH a MW ID to verify bidirectional association
- `autoAddMonitors` option not tested

---

## Your Understanding vs Current Implementation

### Monitors (Your Understanding is Correct)

```
Expected: Create → GET API response → Verify ALL requested fields → Delete → Verify deleted
Current:  Create → Verify SOME fields → Delete → Verify deleted
```

**What's Missing:**

- Comprehensive parameter testing per monitor type
- Validation of ALL response fields that were set in the request

### Maintenance Windows (Your Understanding is Correct)

```
Expected: Create MW → Create Monitor → Verify monitor has MW ID → Delete both
Current:  Create Monitors → Create MW with monitorRefs → Check status.monitorCount exists (no API call)
```

**What's Missing:**

- API call to verify MW actually contains the monitor IDs
- API call to verify Monitor response contains the MW ID in `maintenanceWindowIds`
- Bidirectional verification of the association

---

## Proposed Test Structure: Split by Controller

```
test/e2e/
├── e2e_suite_test.go          # Shared setup (image build, load, restart)
├── common_test.go             # Shared helpers, constants
├── account_test.go            # Account controller tests
├── contact_test.go            # Contact controller tests  
├── monitor_test.go            # Monitor controller tests (all types)
└── maintenancewindow_test.go  # MaintenanceWindow controller tests
```

**Benefits:**

- Each controller's tests are isolated
- Changes to one controller's tests don't affect others
- Can run individual controller tests via labels
- Clearer ownership and maintenance

---

## Proposed Test Flow

### Monitor Tests (per type)

```go
// For each monitor type (HTTP, KEYWORD, PING, PORT, DNS, HEARTBEAT):
It("should create [TYPE] monitor with all parameters", func() {
    // 1. Create monitor with ALL available parameters for this type
    applyMonitor(fullMonitorYAML)
    
    // 2. Wait for ready
    monitorID := waitMonitorReadyAndGetID(name)
    
    // 3. GET from API and validate ALL fields
    monitor, err := getMonitorFromAPI(apiKey, monitorID)
    Expect(err).NotTo(HaveOccurred())
    
    // 4. Validate EVERY field that was set
    Expect(monitor.FriendlyName).To(Equal(expectedName))
    Expect(monitor.URL).To(Equal(expectedURL))
    Expect(monitor.Type).To(Equal(expectedType))
    Expect(monitor.Interval).To(Equal(expectedInterval))
    // ... validate ALL type-specific fields
    
    // 5. Delete
    deleteMonitor(name)
    
    // 6. Verify deleted from API
    WaitForMonitorDeletedFromAPI(apiKey, monitorID)
})
```

### Maintenance Window Tests

```go
It("should create [INTERVAL] maintenance window and associate monitors", func() {
    // 1. Create maintenance window
    applyMaintenanceWindow(mwYAML)
    mwID := waitMaintenanceWindowReadyAndGetID(mwName)
    
    // 2. GET from API and validate ALL fields
    mw, err := getMaintenanceWindowFromAPI(apiKey, mwID)
    Expect(mw.Name).To(Equal(expectedName))
    Expect(mw.Interval).To(Equal(expectedInterval))
    Expect(mw.Duration).To(Equal(expectedDuration))
    Expect(mw.Time).To(Equal(expectedTime))
    // ... validate days for weekly/monthly
    
    // 3. Create monitor
    applyMonitor(monitorYAML)
    monitorID := waitMonitorReadyAndGetID(monitorName)
    
    // 4. Update MW to add monitor (or create MW with monitorRefs)
    applyMaintenanceWindow(mwWithMonitorYAML)
    waitMaintenanceWindowReady(mwName)
    
    // 5. Verify MW contains monitor via API
    mw, _ = getMaintenanceWindowFromAPI(apiKey, mwID)
    Expect(mw.MonitorIDs).To(ContainElement(monitorID))
    
    // 6. Verify Monitor contains MW via API
    monitor, _ := getMonitorFromAPI(apiKey, monitorID)
    Expect(monitor.MaintenanceWindows).To(ContainElement(
        HaveField("ID", mwID),
    ))
    
    // 7. Delete both and verify
    deleteMaintenanceWindow(mwName)
    deleteMonitor(monitorName)
    WaitForMaintenanceWindowDeletedFromAPI(apiKey, mwID)
    WaitForMonitorDeletedFromAPI(apiKey, monitorID)
})
```

---

## Implementation Todos

### Phase 1: Add Missing API Validation Functions

- Add `getMaintenanceWindowFromAPI()` helper
- Add `WaitForMaintenanceWindowDeletedFromAPI()` helper
- Extend `MonitorResponse` struct if needed for full validation

### Phase 2: Split Test Files

- Extract monitor tests to `monitor_test.go`
- Keep maintenance window tests in `maintenancewindow_test.go`
- Create `common_test.go` for shared helpers

### Phase 3: Enhance Monitor Tests

- Add comprehensive parameter tests for each monitor type
- Validate ALL response fields that were set in request
- Add missing parameter combinations

### Phase 4: Enhance Maintenance Window Tests

- Add API-level validation of MW fields
- Add bidirectional monitor association verification
- Test `autoAddMonitors` option

### Phase 5: Add Ginkgo Labels for Isolation

- `Label("monitor")` for monitor tests
- `Label("maintenancewindow")` for MW tests
- Allow running: `ginkgo --label-filter="monitor"`

