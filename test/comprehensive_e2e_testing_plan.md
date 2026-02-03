---
name: Comprehensive E2E Testing
overview: Enhance the uptime-robot-operator e2e tests to comprehensively validate all monitor types and fields by expanding the MonitorResponse struct, adding API validation helpers, and creating field-level assertions for each monitor type.
todos:
  - id: expand-monitor-response
    content: Expand MonitorResponse struct in v3types.go to include all fields from monitor-response.json schema
    status: pending
  - id: add-api-helpers
    content: Create test/e2e/api_helpers.go with getMonitorFromAPI() and validation helpers
    status: pending
  - id: test-https-full
    content: Add HTTPS monitor test with all common fields (interval, timeout, gracePeriod, headers, SSL options, etc.)
    status: pending
  - id: test-https-auth
    content: Add HTTPS monitor test with Basic and Digest authentication
    status: pending
  - id: test-https-post
    content: Add HTTPS monitor test with POST/PUT methods and request body
    status: pending
  - id: test-keyword-full
    content: Add Keyword monitor tests for Exists/NotExists with case sensitivity options
    status: pending
  - id: test-ping
    content: Add Ping monitor test
    status: pending
  - id: test-port
    content: Add Port monitor test with port number validation
    status: pending
  - id: test-heartbeat
    content: Enhance Heartbeat monitor test to validate heartbeatURL in status
    status: pending
  - id: test-dns
    content: Add DNS monitor test with record type validation
    status: pending
  - id: test-contacts
    content: Add test for contact assignment with threshold and recurrence validation
    status: pending
  - id: update-makefile
    content: Ensure make test-e2e-real runs all new tests with appropriate timeout
    status: pending
isProject: false
---

# Comprehensive E2E Testing for Uptime Robot Operator

## Problem Summary

The current e2e tests have significant gaps:

- **Only 3 of 6 monitor types tested**: HTTPS, Keyword, Heartbeat (missing: Ping, Port, DNS)
- **No field validation**: Tests only check `status.ready == true`, never verify actual API values
- **Incomplete response types**: `MonitorResponse` struct has 6 fields, API returns 50+ fields per [monitor-response.json](api-spec/monitor-response.json)

## Implementation Strategy

### Phase 1: Expand MonitorResponse Struct

Update [internal/uptimerobot/v3types.go](internal/uptimerobot/v3types.go) to include all fields from the API schema:

```go
type MonitorResponse struct {
    ID                        int                        `json:"id"`
    FriendlyName              string                     `json:"friendlyName"`
    URL                       string                     `json:"url"`
    Type                      string                     `json:"type"`
    Status                    string                     `json:"status"`
    Interval                  int                        `json:"interval"`
    Timeout                   *int                       `json:"timeout,omitempty"`
    GracePeriod               *int                       `json:"gracePeriod,omitempty"`
    HTTPMethodType            string                     `json:"httpMethodType,omitempty"`
    HTTPUsername              string                     `json:"httpUsername,omitempty"`
    AuthType                  string                     `json:"authType,omitempty"`
    KeywordType               string                     `json:"keywordType,omitempty"`
    KeywordCaseType           int                        `json:"keywordCaseType,omitempty"`
    KeywordValue              string                     `json:"keywordValue,omitempty"`
    Port                      *int                       `json:"port,omitempty"`
    CustomHTTPHeaders         map[string]string          `json:"customHttpHeaders,omitempty"`
    SuccessHTTPResponseCodes  []string                   `json:"successHttpResponseCodes,omitempty"`
    CheckSSLErrors            *bool                      `json:"checkSSLErrors,omitempty"`
    SSLExpirationReminder     *bool                      `json:"sslExpirationReminder,omitempty"`
    DomainExpirationReminder  *bool                      `json:"domainExpirationReminder,omitempty"`
    FollowRedirections        *bool                      `json:"followRedirections,omitempty"`
    ResponseTimeThreshold     *int                       `json:"responseTimeThreshold,omitempty"`
    Config                    *MonitorConfigResponse     `json:"config,omitempty"`
    Tags                      []TagResponse              `json:"tags,omitempty"`
    AssignedAlertContacts     []AssignedAlertContactResp `json:"assignedAlertContacts,omitempty"`
    RegionalData              *RegionalDataResponse      `json:"regionalData,omitempty"`
    GroupID                   *int                       `json:"groupId,omitempty"`
    // ... additional fields
}
```

### Phase 2: Create E2E Validation Helpers

Add new file [test/e2e/api_helpers.go](test/e2e/api_helpers.go) with direct API verification functions:

- `getMonitorFromAPI(apiKey, monitorID string) (*MonitorResponse, error)` - Fetch monitor directly from UptimeRobot
- `validateMonitorFields(expected MonitorSpec, actual *MonitorResponse) []error` - Compare spec to actual API response
- Helper matchers for Gomega assertions

### Phase 3: Test Each Monitor Type with All Fields

Create comprehensive test contexts in [test/e2e/crd_reconciliation_test.go](test/e2e/crd_reconciliation_test.go):

#### 1. HTTPS Monitor (Fully Featured)

```yaml
spec:
  monitor:
    name: "E2E HTTPS Full"
    url: https://httpbin.org/get
    type: HTTPS
    interval: 5m
    timeout: 30s
    gracePeriod: 60s
    method: GET
    followRedirections: true
    checkSSLErrors: true
    sslExpirationReminder: true
    domainExpirationReminder: true
    successHttpResponseCodes: ["2xx", "3xx"]
    responseTimeThreshold: 5000
    customHttpHeaders:
      X-Custom: "test-value"
    tags: ["e2e", "https"]
```

**Validate**: All fields match in UptimeRobot response

#### 2. HTTPS with Authentication

```yaml
spec:
  monitor:
    name: "E2E HTTPS Auth"
    url: https://httpbin.org/basic-auth/user/pass
    type: HTTPS
    method: GET
    auth:
      type: Basic
      username: user
      password: pass
```

**Validate**: `authType`, `httpUsername` match

#### 3. HTTPS with POST Data

```yaml
spec:
  monitor:
    name: "E2E HTTPS POST"
    url: https://httpbin.org/post
    type: HTTPS
    method: POST
    post:
      postType: RawData
      contentType: application/json
      value: '{"test": "data"}'
```

**Validate**: `httpMethodType`, `postValueType`, `postValueData` match

#### 4. Keyword Monitor (All Options)

```yaml
spec:
  monitor:
    name: "E2E Keyword Full"
    url: https://example.com
    type: Keyword
    interval: 5m
    keyword:
      type: Exists
      value: "Example Domain"
      caseSensitive: true
    timeout: 30s
```

**Validate**: `keywordType`, `keywordValue`, `keywordCaseType` match

#### 5. Keyword NotExists

```yaml
spec:
  monitor:
    keyword:
      type: NotExists
      value: "This should not exist"
```

**Validate**: `keywordType: ALERT_NOT_EXISTS`

#### 6. Ping Monitor

```yaml
spec:
  monitor:
    name: "E2E Ping Monitor"
    url: 8.8.8.8
    type: Ping
    interval: 5m
```

**Validate**: `type: PING`, `url` match

#### 7. Port Monitor

```yaml
spec:
  monitor:
    name: "E2E Port Monitor"
    url: google.com
    type: Port
    interval: 5m
    port:
      number: 443
```

**Validate**: `type: PORT`, `port: 443`

#### 8. Heartbeat Monitor

```yaml
spec:
  monitor:
    name: "E2E Heartbeat Monitor"
    type: Heartbeat
    heartbeat:
      interval: 5m
```

**Validate**: `type: HEARTBEAT`, `status.heartbeatURL` populated

#### 9. DNS Monitor

```yaml
spec:
  monitor:
    name: "E2E DNS Monitor"
    url: google.com
    type: DNS
    interval: 5m
    dns:
      a: ["142.250.x.x"]  # Google's IP range
```

**Validate**: `type: DNS`, `config.dnsRecords.A` match

### Phase 4: Field Validation Pattern

For each test, follow this pattern:

```go
It("should create HTTPS monitor with all fields", func() {
    // 1. Create monitor via kubectl
    // 2. Wait for status.ready == true
    // 3. Get status.id
    // 4. Call UptimeRobot API directly with GetMonitor(id)
    // 5. Assert each field matches expected value
    
    By("verifying monitor fields in UptimeRobot")
    Eventually(func(g Gomega) {
        monitor, err := getMonitorFromAPI(apiKey, monitorID)
        g.Expect(err).NotTo(HaveOccurred())
        
        g.Expect(monitor.FriendlyName).To(Equal("E2E HTTPS Full"))
        g.Expect(monitor.URL).To(Equal("https://httpbin.org/get"))
        g.Expect(monitor.Type).To(Equal("HTTP"))
        g.Expect(monitor.Interval).To(Equal(300)) // 5m in seconds
        g.Expect(monitor.HTTPMethodType).To(Equal("GET"))
        g.Expect(*monitor.CheckSSLErrors).To(BeTrue())
        g.Expect(*monitor.FollowRedirections).To(BeTrue())
        g.Expect(monitor.CustomHTTPHeaders).To(HaveKeyWithValue("X-Custom", "test-value"))
        // ... etc
    }, 2*time.Minute, 10*time.Second).Should(Succeed())
})
```

### Phase 5: Additional Response Types

Add supporting response types for nested structures:

```go
type TagResponse struct {
    ID   int    `json:"id"`
    Name string `json:"name"`
}

type AssignedAlertContactResp struct {
    AlertContactID string `json:"alertContactId"`
    Threshold      int    `json:"threshold"`
    Recurrence     int    `json:"recurrence"`
}

type RegionalDataResponse struct {
    Region         []string `json:"REGION"`
    ManualSelected bool     `json:"MANUAL_SELECTED"`
    Infrastructure string   `json:"INFRASTRUCTURE"`
}

type MonitorConfigResponse struct {
    DNSRecords              *DNSRecordsConfig `json:"dnsRecords,omitempty"`
    SSLExpirationPeriodDays []int             `json:"sslExpirationPeriodDays,omitempty"`
}
```

## Files to Modify


| File                                                                       | Changes                                       |
| -------------------------------------------------------------------------- | --------------------------------------------- |
| [internal/uptimerobot/v3types.go](internal/uptimerobot/v3types.go)         | Expand `MonitorResponse` with all API fields  |
| [test/e2e/api_helpers.go](test/e2e/api_helpers.go)                         | New file for API validation helpers           |
| [test/e2e/crd_reconciliation_test.go](test/e2e/crd_reconciliation_test.go) | Add comprehensive tests for all monitor types |


## Test Execution

Tests will run via existing `make test-e2e-real` which requires `UPTIME_ROBOT_API_KEY`.

## Validation Rules to Test

- Keyword type requires `keyword` config
- Port type requires `port` config
- DNS type requires `dns` config
- Non-Heartbeat types require `url`
- Field values persist correctly through create/update cycles

