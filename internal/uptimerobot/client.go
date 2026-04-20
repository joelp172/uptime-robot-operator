/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package uptimerobot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	uptimerobotv1 "github.com/joelp172/uptime-robot-operator/api/v1alpha1"
	"github.com/joelp172/uptime-robot-operator/internal/uptimerobot/urtypes"
)

const apiMonitorType = "API"

// slackIntegrationType is the API-side string identifying a Slack integration.
const slackIntegrationType = "Slack"

const (
	// DefaultRateLimit is the default maximum number of API requests per second.
	DefaultRateLimit = 10
	// DefaultHTTPTimeout is the default timeout for individual HTTP requests.
	DefaultHTTPTimeout = 30 * time.Second
)

// globalLimiters is a process-wide registry of rate limiters keyed by a hash
// of the API key and the configured rate. This ensures all Client instances
// created for the same API key and rate share a single limiter, so concurrent
// reconcilers cannot each claim a fresh burst allowance.
//
// Entries are never evicted. This is acceptable because cardinality is bounded
// by the number of distinct (API key, rate limit) pairs, which is typically one
// per operator process (one Account with one configured rate).
var globalLimiters sync.Map

// limiterKey returns the registry key for a (apiKey, rateLimit) pair.
// The API key is hashed to avoid holding secrets as plaintext map keys.
func limiterKey(apiKey string, rateLimit int) string {
	h := sha256.Sum256([]byte(apiKey)) //nolint:gosec // not password hashing; used as a map key to avoid storing the API key in plaintext
	return fmt.Sprintf("%x:%d", h, rateLimit)
}

// getSharedLimiter returns the existing limiter for (apiKey, rateLimit),
// creating and storing one if it does not yet exist.
func getSharedLimiter(apiKey string, rateLimit int) *rate.Limiter {
	key := limiterKey(apiKey, rateLimit)
	// Fast path: avoid allocating a new limiter when one already exists.
	if v, ok := globalLimiters.Load(key); ok {
		return v.(*rate.Limiter)
	}
	l := rate.NewLimiter(rate.Limit(rateLimit), rateLimit)
	actual, _ := globalLimiters.LoadOrStore(key, l)
	return actual.(*rate.Limiter)
}

// clientConfig holds parsed numeric/duration environment variable overrides.
// Parsed once at first use to avoid repeated parsing and log spam on every reconcile.
// UPTIME_ROBOT_API is intentionally NOT cached here because controller tests
// change it between reconciles to simulate failure paths.
type clientConfig struct {
	maxRetries  int
	baseDelay   time.Duration
	rateLimit   int
	httpTimeout time.Duration
}

var (
	parsedConfig    clientConfig
	parseConfigOnce sync.Once
)

func getClientConfig() clientConfig {
	parseConfigOnce.Do(func() {
		parsedConfig.maxRetries = DefaultMaxRetries
		if env := os.Getenv("UPTIME_ROBOT_MAX_RETRIES"); env != "" {
			if n, err := strconv.Atoi(env); err == nil && n > 0 {
				parsedConfig.maxRetries = n
			} else {
				fmt.Fprintf(os.Stderr, "WARNING: invalid UPTIME_ROBOT_MAX_RETRIES=%q (must be a positive integer), using default %d\n", env, DefaultMaxRetries)
			}
		}

		parsedConfig.baseDelay = DefaultBaseDelay
		if env := os.Getenv("UPTIME_ROBOT_BASE_DELAY"); env != "" {
			if d, err := time.ParseDuration(env); err == nil && d > 0 {
				parsedConfig.baseDelay = d
			} else {
				fmt.Fprintf(os.Stderr, "WARNING: invalid UPTIME_ROBOT_BASE_DELAY=%q (must be a positive Go duration string, e.g. \"1s\"), using default %v\n", env, DefaultBaseDelay)
			}
		}

		parsedConfig.rateLimit = DefaultRateLimit
		if env := os.Getenv("UPTIME_ROBOT_RATE_LIMIT"); env != "" {
			if n, err := strconv.Atoi(env); err == nil && n > 0 {
				parsedConfig.rateLimit = n
			} else {
				fmt.Fprintf(os.Stderr, "WARNING: invalid UPTIME_ROBOT_RATE_LIMIT=%q (must be a positive integer), using default %d\n", env, DefaultRateLimit)
			}
		}

		parsedConfig.httpTimeout = DefaultHTTPTimeout
		if env := os.Getenv("UPTIME_ROBOT_HTTP_TIMEOUT"); env != "" {
			if d, err := time.ParseDuration(env); err == nil && d > 0 {
				parsedConfig.httpTimeout = d
			} else {
				fmt.Fprintf(os.Stderr, "WARNING: invalid UPTIME_ROBOT_HTTP_TIMEOUT=%q (must be a positive Go duration string, e.g. \"30s\"), using default %v\n", env, DefaultHTTPTimeout)
			}
		}
	})
	return parsedConfig
}

// NewClient creates a new UptimeRobot API v3 client.
// The following environment variables can override defaults (useful for testing):
//   - UPTIME_ROBOT_API: base URL for the UptimeRobot API (read on every call)
//   - UPTIME_ROBOT_MAX_RETRIES: maximum number of retry attempts (positive integer)
//   - UPTIME_ROBOT_BASE_DELAY: base delay between retries (Go duration string, e.g. "1ms")
//   - UPTIME_ROBOT_RATE_LIMIT: maximum API requests per second (positive integer, default 10)
//   - UPTIME_ROBOT_HTTP_TIMEOUT: timeout for individual HTTP requests (Go duration string, e.g. "30s", default 30s)
//
// Numeric/duration env vars are parsed once at first call; invalid values log a
// warning to stderr and fall back to defaults. UPTIME_ROBOT_API is read on every
// call so tests can change it between reconciles.
//
// Clients sharing the same API key and rate limit share a single process-wide
// rate limiter, preventing concurrent reconcilers from each claiming a fresh burst.
func NewClient(apiKey string) Client {
	cfg := getClientConfig()

	api := "https://api.uptimerobot.com/v3"
	if env := os.Getenv("UPTIME_ROBOT_API"); env != "" {
		api = strings.TrimSuffix(env, "/")
	}

	return Client{
		url:            api,
		apiKey:         apiKey,
		httpClient:     &http.Client{Timeout: cfg.httpTimeout},
		maxRetries:     cfg.maxRetries,
		baseDelay:      cfg.baseDelay,
		maxDelay:       DefaultMaxDelay,
		jitterFraction: DefaultJitterFraction,
		// Burst equals the rate, so up to rateLimit requests can fire immediately;
		// after that, requests are admitted at a steady rate of rateLimit per second.
		// The limiter is shared across all clients for the same API key so that
		// concurrent reconcilers cannot each claim a fresh burst allowance.
		limiter: getSharedLimiter(apiKey, cfg.rateLimit),
	}
}

// Client is the UptimeRobot API v3 client.
type Client struct {
	url    string
	apiKey string

	// httpClient is the dedicated HTTP client used for all API requests.
	// It has a configurable timeout to prevent goroutines from blocking indefinitely.
	httpClient *http.Client

	// limiter throttles outbound API requests to prevent quota exhaustion.
	// Shared across all Client instances for the same API key.
	limiter *rate.Limiter

	// Optional retry overrides for testing. Zero values use package defaults.
	maxRetries     int
	baseDelay      time.Duration
	maxDelay       time.Duration
	jitterFraction float64
}

var (
	ErrStatus              = errors.New("error code from Uptime Robot API")
	ErrResponse            = errors.New("received fail from Uptime Robot API")
	ErrMonitorNotFound     = errors.New("monitor not found")
	ErrContactNotFound     = errors.New("contact not found")
	ErrIntegrationNotFound = errors.New("integration not found")
	ErrNotFound            = errors.New("resource not found")
)

// IsNotFound checks if an error indicates a resource was not found (404).
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	// Check for 404 status code in error message
	return strings.Contains(err.Error(), "404") ||
		errors.Is(err, ErrNotFound) ||
		errors.Is(err, ErrMonitorNotFound) ||
		errors.Is(err, ErrContactNotFound) ||
		errors.Is(err, ErrIntegrationNotFound)
}

// newRequest creates a new HTTP request with v3 API authentication.
func (c Client) newRequest(ctx context.Context, method, endpoint string, body any) (*http.Request, error) {
	u := c.url + "/" + endpoint

	var bodyReader io.Reader
	var jsonBody []byte
	if body != nil {
		var err error
		jsonBody, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, bodyReader)
	if err != nil {
		return nil, err
	}

	// Set GetBody for retry support (allows request body to be re-read on retry)
	if jsonBody != nil {
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(jsonBody)), nil
		}
	}

	// v3 uses Bearer token authentication
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cache-Control", "no-cache")

	return req, nil
}

// do executes an HTTP request and returns the response with retry logic.
func (c Client) do(req *http.Request) (*http.Response, error) {
	return c.doWithRetry(req.Context(), req)
}

// doJSON executes an HTTP request and decodes the JSON response.
func (c Client) doJSON(ctx context.Context, method, endpoint string, body any, result any) error {
	req, err := c.newRequest(ctx, method, endpoint, body)
	if err != nil {
		return err
	}

	res, err := c.do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()

	if result != nil {
		if err := json.NewDecoder(res.Body).Decode(result); err != nil {
			return err
		}
	}

	return nil
}

// doGetJSON executes a GET request to a full URL and decodes the JSON response.
// Used for following nextLink in paginated list responses.
func (c Client) doGetJSON(ctx context.Context, fullURL string, result any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cache-Control", "no-cache")

	res, err := c.do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()

	if result != nil {
		if err := json.NewDecoder(res.Body).Decode(result); err != nil {
			return err
		}
	}
	return nil
}

// listAllMonitors fetches all monitors by following nextLink pagination.
// Used when adopting on 409 so the duplicate monitor is found even if on a later page.
func (c Client) listAllMonitors(ctx context.Context) ([]MonitorResponse, error) {
	var all []MonitorResponse
	var resp MonitorsListResponse
	if err := c.doJSON(ctx, http.MethodGet, "monitors", nil, &resp); err != nil {
		return nil, err
	}
	all = append(all, resp.Monitors...)
	for resp.NextLink != nil && *resp.NextLink != "" {
		nextURL := *resp.NextLink
		if !strings.HasPrefix(nextURL, "http") {
			nextURL = strings.TrimSuffix(c.url, "/") + "/" + strings.TrimPrefix(nextURL, "/")
		}
		resp = MonitorsListResponse{}
		if err := c.doGetJSON(ctx, nextURL, &resp); err != nil {
			return nil, err
		}
		all = append(all, resp.Monitors...)
	}
	return all, nil
}

// buildCreateMonitorRequest converts internal types to v3 API request format.
func (c Client) buildCreateMonitorRequest(monitor uptimerobotv1.MonitorValues, contacts uptimerobotv1.MonitorContacts) CreateMonitorRequest {
	effectiveType := monitorTypeForRequest(monitor)

	// Calculate grace period (default 60s, max 86400s)
	gracePeriod := 60
	if monitor.GracePeriod != nil {
		gracePeriod = int(monitor.GracePeriod.Seconds())
	}
	if gracePeriod > 86400 {
		gracePeriod = 86400
	}
	if gracePeriod < 0 {
		gracePeriod = 0
	}

	req := CreateMonitorRequest{
		FriendlyName: monitor.Name,
		URL:          monitor.URL,
		Type:         effectiveType,
		Interval:     int(monitor.Interval.Seconds()),
		Timeout:      int(monitor.Timeout.Seconds()),
		GracePeriod:  gracePeriod,
		HTTPMethod:   httpMethodToString(monitor.Method),
	}

	// Handle auth
	if monitor.Auth != nil {
		req.HTTPAuthType = authTypeToString(monitor.Auth.Type)
		req.HTTPUsername = monitor.Auth.Username
		req.HTTPPassword = monitor.Auth.Password
	}

	// Handle POST data
	switch monitor.Method {
	case urtypes.MethodHEAD, urtypes.MethodGET:
		// No POST data for HEAD/GET
	default:
		if monitor.POST != nil {
			req.PostType = postTypeToString(monitor.POST.Type)
			req.PostValue = monitor.POST.Value
		}
	}

	// Handle keyword monitors
	if monitor.Type == urtypes.TypeKeyword && monitor.Keyword != nil {
		req.KeywordType = keywordTypeToString(monitor.Keyword.Type)
		caseType := 0 // 0 = CaseInsensitive (default)
		if monitor.Keyword.CaseSensitive != nil && *monitor.Keyword.CaseSensitive {
			caseType = 1 // 1 = CaseSensitive
		}
		req.KeywordCaseType = &caseType
		req.KeywordValue = monitor.Keyword.Value
	}

	// Handle port monitors
	if monitor.Type == urtypes.TypePort && monitor.Port != nil {
		req.Port = int(monitor.Port.Number)
	}

	// Handle DNS monitors - v3 API requires a config object with dnsRecords
	if effectiveType == urtypes.APITypeDNS && monitor.DNS != nil {
		req.Config = &MonitorConfig{
			DNSRecords: &DNSRecordsConfig{
				A:     monitor.DNS.A,
				AAAA:  monitor.DNS.AAAA,
				CNAME: monitor.DNS.CNAME,
				MX:    monitor.DNS.MX,
				NS:    monitor.DNS.NS,
				TXT:   monitor.DNS.TXT,
				SRV:   monitor.DNS.SRV,
				PTR:   monitor.DNS.PTR,
				SOA:   monitor.DNS.SOA,
				SPF:   monitor.DNS.SPF,
			},
			SSLExpirationPeriodDays: monitor.DNS.SSLExpirationPeriodDays,
		}
	}

	// Handle Heartbeat monitors - v3 API may require a config object
	if effectiveType == urtypes.APITypeHeartbeat {
		req.Config = &MonitorConfig{}
	}

	// Handle API assertions for CRD HTTPS monitors (mapped to upstream API type "API").
	if monitor.APIAssertions != nil && len(monitor.APIAssertions.Checks) > 0 {
		if req.Config == nil {
			req.Config = &MonitorConfig{}
		}
		req.Config.APIAssertions = buildAPIAssertionsConfig(monitor.APIAssertions)
	}

	// Convert contacts to v3 format
	req.AssignedAlertContacts = contactsToV3Format(contacts)

	// New v3 API fields
	if len(monitor.Tags) > 0 {
		req.TagNames = monitor.Tags
	}
	if len(monitor.CustomHTTPHeaders) > 0 {
		req.CustomHTTPHeaders = monitor.CustomHTTPHeaders
	}
	if len(monitor.SuccessHTTPResponseCodes) > 0 {
		req.SuccessHTTPResponseCodes = monitor.SuccessHTTPResponseCodes
	}
	if monitor.CheckSSLErrors != nil {
		req.CheckSSLErrors = monitor.CheckSSLErrors
	}
	if monitor.SSLExpirationReminder != nil {
		req.SSLExpirationReminder = monitor.SSLExpirationReminder
	}
	if monitor.DomainExpirationReminder != nil {
		req.DomainExpirationReminder = monitor.DomainExpirationReminder
	}
	if monitor.FollowRedirections != nil {
		req.FollowRedirections = monitor.FollowRedirections
	}
	if monitor.ResponseTimeThreshold != nil {
		req.ResponseTimeThreshold = monitor.ResponseTimeThreshold
	}
	if monitor.Region != "" {
		req.RegionalData = monitor.Region
	}
	if monitor.GroupID != nil {
		req.GroupID = monitor.GroupID
	}
	if len(monitor.MaintenanceWindowIDs) > 0 {
		req.MaintenanceWindowsIds = monitor.MaintenanceWindowIDs
	}

	return req
}

// buildUpdateMonitorRequest converts internal types to v3 API update request format.
func (c Client) buildUpdateMonitorRequest(monitor uptimerobotv1.MonitorValues, contacts uptimerobotv1.MonitorContacts) UpdateMonitorRequest {
	effectiveType := monitorTypeForRequest(monitor)

	// Calculate grace period (default 60s, max 86400s)
	gracePeriod := 60
	if monitor.GracePeriod != nil {
		gracePeriod = int(monitor.GracePeriod.Seconds())
	}
	if gracePeriod > 86400 {
		gracePeriod = 86400
	}
	if gracePeriod < 0 {
		gracePeriod = 0
	}

	req := UpdateMonitorRequest{
		FriendlyName: monitor.Name,
		Interval:     int(monitor.Interval.Seconds()),
		GracePeriod:  gracePeriod,
		// Note: Status is not supported in v3 PATCH requests - use pause/resume endpoints instead
	}

	// UptimeRobot v3 rejects URL updates for DNS/Heartbeat/PING monitors.
	if effectiveType != urtypes.APITypeDNS && effectiveType != urtypes.APITypeHeartbeat && effectiveType != urtypes.APITypePing {
		req.URL = monitor.URL
	}
	if effectiveType == urtypes.APITypeHTTP || effectiveType == urtypes.APITypeKeyword || effectiveType == urtypes.APITypeAPI {
		req.HTTPMethod = httpMethodToString(monitor.Method)
	}
	if effectiveType == urtypes.APITypeHTTP || effectiveType == urtypes.APITypeKeyword || effectiveType == urtypes.APITypePort || effectiveType == urtypes.APITypeAPI {
		req.Timeout = int(monitor.Timeout.Seconds())
	}

	// Handle auth
	if monitor.Auth != nil {
		req.HTTPAuthType = authTypeToString(monitor.Auth.Type)
		req.HTTPUsername = monitor.Auth.Username
		req.HTTPPassword = monitor.Auth.Password
	}

	// Handle POST data
	switch monitor.Method {
	case urtypes.MethodHEAD, urtypes.MethodGET:
		// No POST data for HEAD/GET
	default:
		if monitor.POST != nil {
			req.PostType = postTypeToString(monitor.POST.Type)
			req.PostValue = monitor.POST.Value
		}
	}

	// Handle keyword monitors
	if monitor.Type == urtypes.TypeKeyword && monitor.Keyword != nil {
		req.KeywordType = keywordTypeToString(monitor.Keyword.Type)
		caseType := 0 // 0 = CaseInsensitive (default)
		if monitor.Keyword.CaseSensitive != nil && *monitor.Keyword.CaseSensitive {
			caseType = 1 // 1 = CaseSensitive
		}
		req.KeywordCaseType = &caseType
		req.KeywordValue = monitor.Keyword.Value
	}

	// Handle port monitors
	if monitor.Type == urtypes.TypePort && monitor.Port != nil {
		req.Port = int(monitor.Port.Number)
	}

	// Handle DNS monitors - v3 API requires a config object with dnsRecords
	if effectiveType == urtypes.APITypeDNS && monitor.DNS != nil {
		req.Config = &MonitorConfig{
			DNSRecords: &DNSRecordsConfig{
				A:     monitor.DNS.A,
				AAAA:  monitor.DNS.AAAA,
				CNAME: monitor.DNS.CNAME,
				MX:    monitor.DNS.MX,
				NS:    monitor.DNS.NS,
				TXT:   monitor.DNS.TXT,
				SRV:   monitor.DNS.SRV,
				PTR:   monitor.DNS.PTR,
				SOA:   monitor.DNS.SOA,
				SPF:   monitor.DNS.SPF,
			},
			SSLExpirationPeriodDays: monitor.DNS.SSLExpirationPeriodDays,
		}
	}

	// Handle Heartbeat monitors - v3 API may require a config object
	if effectiveType == urtypes.APITypeHeartbeat {
		req.Config = &MonitorConfig{}
	}

	// Handle API assertions for CRD HTTPS monitors (mapped to upstream API type "API").
	if monitor.APIAssertions != nil && len(monitor.APIAssertions.Checks) > 0 {
		if req.Config == nil {
			req.Config = &MonitorConfig{}
		}
		req.Config.APIAssertions = buildAPIAssertionsConfig(monitor.APIAssertions)
	}

	// Convert contacts to v3 format
	req.AssignedAlertContacts = contactsToV3Format(contacts)

	// New v3 API fields
	if len(monitor.Tags) > 0 {
		req.TagNames = monitor.Tags
	}
	if len(monitor.CustomHTTPHeaders) > 0 {
		req.CustomHTTPHeaders = monitor.CustomHTTPHeaders
	}
	if len(monitor.SuccessHTTPResponseCodes) > 0 {
		req.SuccessHTTPResponseCodes = monitor.SuccessHTTPResponseCodes
	}
	if monitor.CheckSSLErrors != nil {
		req.CheckSSLErrors = monitor.CheckSSLErrors
	}
	if monitor.SSLExpirationReminder != nil {
		req.SSLExpirationReminder = monitor.SSLExpirationReminder
	}
	if monitor.DomainExpirationReminder != nil {
		req.DomainExpirationReminder = monitor.DomainExpirationReminder
	}
	if monitor.FollowRedirections != nil {
		req.FollowRedirections = monitor.FollowRedirections
	}
	if monitor.ResponseTimeThreshold != nil {
		req.ResponseTimeThreshold = monitor.ResponseTimeThreshold
	}
	if monitor.Region != "" {
		req.RegionalData = monitor.Region
	}
	if monitor.GroupID != nil {
		req.GroupID = monitor.GroupID
	}
	if len(monitor.MaintenanceWindowIDs) > 0 {
		req.MaintenanceWindowsIds = monitor.MaintenanceWindowIDs
	}

	return req
}

// contactsToV3Format converts MonitorContacts to v3 API format.
// Note: v3 API uses assignedAlertContacts with alertContactId (string), threshold, and recurrence.
func contactsToV3Format(contacts uptimerobotv1.MonitorContacts) []AssignedAlertContactRequest {
	result := make([]AssignedAlertContactRequest, 0, len(contacts))
	for _, c := range contacts {
		// Skip contacts without valid IDs
		if c.ID == "" {
			continue
		}
		// Calculate threshold in seconds (per-contact wait time before alerting)
		threshold := int(c.Threshold.Seconds())
		if threshold < 0 {
			threshold = 0
		}
		result = append(result, AssignedAlertContactRequest{
			AlertContactID: c.ID, // v3 API uses alertContactId as a string
			Threshold:      threshold,
			Recurrence:     int(c.Recurrence.Round(time.Minute).Minutes()),
		})
	}
	return result
}

// CreateMonitorResult contains the result of creating a monitor.
type CreateMonitorResult struct {
	ID  string
	URL string // Contains the heartbeat URL for heartbeat monitors
}

// CreateMonitor creates a new monitor using the v3 API.
// POST /monitors
func (c Client) CreateMonitor(ctx context.Context, monitor uptimerobotv1.MonitorValues, contacts uptimerobotv1.MonitorContacts) (CreateMonitorResult, error) {
	reqBody := c.buildCreateMonitorRequest(monitor, contacts)
	req, err := c.newRequest(ctx, http.MethodPost, "monitors", reqBody)
	if err != nil {
		return CreateMonitorResult{}, err
	}
	res, err := c.do(req)
	if err == nil {
		body, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		var resp MonitorCreateResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return CreateMonitorResult{}, err
		}
		return CreateMonitorResult{
			ID:  strconv.Itoa(resp.ID),
			URL: resp.URL,
		}, nil
	}

	if errors.Is(err, ErrStatus) && strings.Contains(err.Error(), "409 Conflict") {
		body := extractErrStatusBody(err)
		// 409 Duplicate: resolve existing monitor ID and adopt it so reconciliation can continue.
		if id := parseMonitorIDFrom409Body(body); id != "" {
			m, getErr := c.GetMonitor(ctx, id)
			if getErr == nil {
				return CreateMonitorResult{ID: id, URL: m.URL}, nil
			}
			return CreateMonitorResult{ID: id}, nil
		}

		// Try to resolve a duplicate monitor from list results using strict matching.
		// Never adopt by URL alone; that can incorrectly alias many CRs to one monitor.
		tryFind := func() (CreateMonitorResult, bool) {
			all, findErr := c.listAllMonitors(ctx)
			if findErr != nil {
				return CreateMonitorResult{}, false
			}
			if m, ok := selectDuplicateMonitorCandidate(all, monitor); ok {
				return CreateMonitorResult{ID: strconv.Itoa(m.ID), URL: m.URL}, true
			}
			return CreateMonitorResult{}, false
		}

		if result, ok := tryFind(); ok {
			return result, nil
		}

		// Retry once after a short delay (e.g. list may have hit 429 rate limit).
		select {
		case <-ctx.Done():
			return CreateMonitorResult{}, ctx.Err()
		case <-time.After(2 * time.Second):
		}

		if result, ok := tryFind(); ok {
			return result, nil
		}
	}
	return CreateMonitorResult{}, err
}

// parseMonitorIDFrom409Body extracts a monitor ID from a 409 response body if present.
// Handles top-level id, and nested data.id / monitor.id shapes.
func parseMonitorIDFrom409Body(body []byte) string {
	var withID struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(body, &withID); err == nil && withID.ID > 0 {
		return strconv.Itoa(withID.ID)
	}
	var nested struct {
		Data *struct {
			ID int `json:"id"`
		} `json:"data"`
		Monitor *struct {
			ID int `json:"id"`
		} `json:"monitor"`
	}
	if err := json.Unmarshal(body, &nested); err != nil {
		return ""
	}
	if nested.Data != nil && nested.Data.ID > 0 {
		return strconv.Itoa(nested.Data.ID)
	}
	if nested.Monitor != nil && nested.Monitor.ID > 0 {
		return strconv.Itoa(nested.Monitor.ID)
	}
	return ""
}

// normalizeURL trims trailing slash for consistent comparison with API-stored URLs.
func normalizeURL(u string) string {
	return strings.TrimSuffix(strings.TrimSpace(u), "/")
}

func extractErrStatusBody(err error) []byte {
	if err == nil {
		return nil
	}
	msg := err.Error()
	parts := strings.SplitN(msg, " - ", 2)
	if len(parts) < 2 {
		return nil
	}
	return []byte(parts[1])
}

func isSlackIntegrationAlreadyExists409(body []byte) bool {
	if len(body) == 0 {
		return false
	}

	var payload struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}
	return payload.Code == "021-001"
}

// selectDuplicateSlackIntegrationCandidate returns a single safe duplicate
// target for 409 adoption. Matching requires a Slack integration whose
// webhook URL AND FriendlyName both match the desired spec. Adoption is
// refused when FriendlyName is empty, since matching on webhook alone would
// silently alias distinct CRs across clusters that happen to share a channel.
func selectDuplicateSlackIntegrationCandidate(existing []IntegrationResponse, desired SlackIntegrationData) (*IntegrationResponse, bool) {
	targetWebhook := strings.TrimSpace(desired.WebhookURL)
	targetName := strings.TrimSpace(desired.FriendlyName)
	if targetWebhook == "" || targetName == "" {
		return nil, false
	}

	candidates := make([]IntegrationResponse, 0, 1)
	for i := range existing {
		integration := existing[i]
		if integration.Type == nil || *integration.Type != slackIntegrationType {
			continue
		}
		if strings.TrimSpace(integration.Value) != targetWebhook {
			continue
		}
		existingName := ""
		if integration.FriendlyName != nil {
			existingName = strings.TrimSpace(*integration.FriendlyName)
		}
		if existingName != targetName {
			continue
		}
		candidates = append(candidates, integration)
	}

	if len(candidates) != 1 {
		return nil, false
	}
	return &candidates[0], true
}

// selectDuplicateMonitorCandidate returns a single safe duplicate target for 409 adoption.
// Matching rules:
//   - Primary path: match by name (+ URL when provided), requiring a unique candidate.
//   - Fallback path: if URL is provided and name matching yields none/ambiguous,
//     allow adoption by unique URL + type (handles API duplicate rules where name can differ).
func selectDuplicateMonitorCandidate(existing []MonitorResponse, desired uptimerobotv1.MonitorValues) (*MonitorResponse, bool) {
	name := strings.TrimSpace(desired.Name)
	wantURL := normalizeURL(desired.URL)
	wantType := strings.TrimSpace(monitorTypeForRequest(desired))

	nameMatches := make([]MonitorResponse, 0, 1)
	if name != "" {
		for i := range existing {
			m := existing[i]
			if strings.TrimSpace(m.FriendlyName) != name {
				continue
			}
			if wantURL != "" && normalizeURL(m.URL) != wantURL {
				continue
			}
			nameMatches = append(nameMatches, m)
			if len(nameMatches) > 1 {
				break
			}
		}
		if len(nameMatches) == 1 {
			return &nameMatches[0], true
		}
	}

	if wantURL == "" {
		return nil, false
	}

	urlTypeMatches := make([]MonitorResponse, 0, 1)
	for i := range existing {
		m := existing[i]
		if normalizeURL(m.URL) != wantURL {
			continue
		}
		if wantType != "" && !strings.EqualFold(strings.TrimSpace(m.Type), wantType) {
			continue
		}
		urlTypeMatches = append(urlTypeMatches, m)
		if len(urlTypeMatches) > 1 {
			return nil, false
		}
	}

	if len(urlTypeMatches) != 1 {
		return nil, false
	}
	return &urlTypeMatches[0], true
}

// FindMonitorByURL searches for a monitor by its URL, listing all pages so the monitor is found.
func (c Client) FindMonitorByURL(ctx context.Context, url string) (string, error) {
	all, err := c.listAllMonitors(ctx)
	if err != nil {
		return "", err
	}
	norm := normalizeURL(url)
	for _, m := range all {
		if normalizeURL(m.URL) == norm {
			return strconv.Itoa(m.ID), nil
		}
	}
	return "", ErrMonitorNotFound
}

// FindMonitorByName searches for a monitor by its friendly name, listing all pages so the monitor is found.
func (c Client) FindMonitorByName(ctx context.Context, name string) (*MonitorResponse, error) {
	all, err := c.listAllMonitors(ctx)
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].FriendlyName == name {
			return &all[i], nil
		}
	}
	return nil, ErrMonitorNotFound
}

// DeleteMonitor deletes a monitor using the v3 API.
// DELETE /monitors/{id}
func (c Client) DeleteMonitor(ctx context.Context, id string) error {
	endpoint := "monitors/" + id

	req, err := c.newRequest(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}

	res, err := c.do(req)
	if err != nil {
		// If deletion fails with not found, consider it already deleted
		if errors.Is(err, ErrStatus) && strings.Contains(err.Error(), "404") {
			return nil
		}
		// Check if monitor still exists
		if _, findErr := c.FindMonitorID(ctx, FindByID(id)); errors.Is(findErr, ErrMonitorNotFound) {
			return nil
		}
		return err
	}
	defer func() { _ = res.Body.Close() }()

	return nil
}

// EditMonitorResult contains the result of editing a monitor.
type EditMonitorResult struct {
	ID  string
	URL string // Contains the heartbeat URL for heartbeat monitors
}

// EditMonitor updates an existing monitor using the v3 API.
// PATCH /monitors/{id}
func (c Client) EditMonitor(ctx context.Context, id string, monitor uptimerobotv1.MonitorValues, contacts uptimerobotv1.MonitorContacts) (EditMonitorResult, error) {
	endpoint := "monitors/" + id
	reqBody := c.buildUpdateMonitorRequest(monitor, contacts)

	var resp MonitorUpdateResponse
	if err := c.doJSON(ctx, http.MethodPatch, endpoint, reqBody, &resp); err != nil {
		// If update fails because monitor doesn't exist (404), recreate it
		// Check using GetMonitor which uses GET /monitors/{id} directly
		if _, getErr := c.GetMonitor(ctx, id); errors.Is(getErr, ErrMonitorNotFound) {
			result, createErr := c.CreateMonitor(ctx, monitor, contacts)
			return EditMonitorResult(result), createErr
		}
		return EditMonitorResult{ID: id}, err
	}

	return EditMonitorResult{
		ID:  strconv.Itoa(resp.ID),
		URL: resp.URL,
	}, nil
}

// GetMonitor retrieves a single monitor by ID using the v3 API.
// GET /monitors/{id}
// Returns ErrMonitorNotFound if the monitor doesn't exist (404).
func (c Client) GetMonitor(ctx context.Context, id string) (*MonitorResponse, error) {
	endpoint := "monitors/" + id

	var resp MonitorResponse
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &resp); err != nil {
		// Check if it's a 404 error (monitor not found)
		if strings.Contains(err.Error(), "404") {
			return nil, ErrMonitorNotFound
		}
		return nil, err
	}

	return &resp, nil
}

// FindMonitorID finds a monitor ID using the v3 API.
// GET /monitors
func (c Client) FindMonitorID(ctx context.Context, opts ...FindOpt) (string, error) {
	params := make(url.Values)
	for _, opt := range opts {
		opt(params)
	}

	endpoint := "monitors"
	if len(params) > 0 {
		endpoint += "?" + params.Encode()
	}

	var resp MonitorsListResponse
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &resp); err != nil {
		return "", err
	}

	if len(resp.Monitors) == 0 {
		return "", ErrMonitorNotFound
	}

	return strconv.Itoa(resp.Monitors[0].ID), nil
}

// GetAlertContacts returns all alert contacts for the account.
// GET /user/alert-contacts
func (c Client) GetAlertContacts(ctx context.Context) ([]AlertContactResponse, error) {
	// v3 API returns array directly, not wrapped in an object
	var contacts []AlertContactResponse
	if err := c.doJSON(ctx, http.MethodGet, "user/alert-contacts", nil, &contacts); err != nil {
		return nil, err
	}
	return contacts, nil
}

// FindContactID finds an alert contact ID by friendly name using the v3 API.
// GET /user/alert-contacts
func (c Client) FindContactID(ctx context.Context, friendlyName string) (string, error) {
	contacts, err := c.GetAlertContacts(ctx)
	if err != nil {
		return "", err
	}

	for _, contact := range contacts {
		// FriendlyName can be null in the API response
		if contact.FriendlyName != nil && *contact.FriendlyName == friendlyName {
			return strconv.Itoa(contact.ID), nil
		}
	}

	integrationID, err := c.findSlackIntegrationIDByFriendlyName(ctx, friendlyName)
	if err == nil {
		return integrationID, nil
	}
	if !errors.Is(err, ErrIntegrationNotFound) {
		return "", err
	}

	return "", ErrContactNotFound
}

func (c Client) findSlackIntegrationIDByFriendlyName(ctx context.Context, friendlyName string) (string, error) {
	integrations, err := c.ListIntegrations(ctx)
	if err != nil {
		return "", err
	}

	matches := make([]IntegrationResponse, 0, 1)
	for i := range integrations {
		integration := integrations[i]
		if integration.Type == nil || *integration.Type != slackIntegrationType {
			continue
		}
		if integration.FriendlyName == nil || *integration.FriendlyName != friendlyName {
			continue
		}
		matches = append(matches, integration)
	}

	switch len(matches) {
	case 0:
		return "", ErrIntegrationNotFound
	case 1:
		return strconv.Itoa(matches[0].ID), nil
	default:
		return "", fmt.Errorf("multiple Slack integrations found with friendly name %q", friendlyName)
	}
}

// GetAccountDetails retrieves account details using the v3 API.
// GET /user/me
func (c Client) GetAccountDetails(ctx context.Context) (string, error) {
	var resp UserMeResponse
	if err := c.doJSON(ctx, http.MethodGet, "user/me", nil, &resp); err != nil {
		return "", err
	}

	return resp.Email, nil
}

// Helper functions to convert internal types to v3 API strings

func httpMethodToString(m urtypes.HTTPMethod) string {
	switch m {
	case urtypes.MethodHEAD:
		return "HEAD"
	case urtypes.MethodGET:
		return "GET"
	case urtypes.MethodPOST:
		return "POST"
	case urtypes.MethodPUT:
		return "PUT"
	case urtypes.MethodPATCH:
		return "PATCH"
	case urtypes.MethodDELETE:
		return "DELETE"
	case urtypes.MethodOPTIONS:
		return "OPTIONS"
	default:
		return "HEAD"
	}
}

func authTypeToString(t urtypes.MonitorAuthType) string {
	switch t {
	case urtypes.AuthBasic:
		return "HTTP_BASIC"
	case urtypes.AuthDigest:
		return "DIGEST"
	default:
		return "NONE"
	}
}

func postTypeToString(t urtypes.POSTType) string {
	switch t {
	case urtypes.TypeKeyValue:
		return "KEY_VALUE"
	case urtypes.TypeRawData:
		return "RAW_JSON"
	default:
		return "KEY_VALUE"
	}
}

func keywordTypeToString(t urtypes.KeywordType) string {
	switch t {
	case urtypes.KeywordExists:
		return "ALERT_EXISTS"
	case urtypes.KeywordNotExists:
		return "ALERT_NOT_EXISTS"
	default:
		return "ALERT_EXISTS"
	}
}

// buildAPIAssertionsConfig converts MonitorAPIAssertions to API format.
func buildAPIAssertionsConfig(assertions *uptimerobotv1.MonitorAPIAssertions) *APIAssertionsConfig {
	if assertions == nil || len(assertions.Checks) == 0 {
		return nil
	}

	checks := make([]APIAssertionCheck, 0, len(assertions.Checks))
	for _, check := range assertions.Checks {
		apiCheck := APIAssertionCheck{
			Property:   check.Property,
			Comparison: check.Operator.ToAPIString(),
		}

		// is_null and is_not_null operators don't need a target value.
		if check.Value != "" {
			apiCheck.Target = parseAPIAssertionTarget(check.Operator, check.Value)
		}

		checks = append(checks, apiCheck)
	}

	return &APIAssertionsConfig{
		Logic:  assertions.Logic.ToAPIString(),
		Checks: checks,
	}
}

// monitorTypeForRequest maps CRD HTTPS+apiAssertions to UptimeRobot v3 type API.
// CRD validation restricts apiAssertions to HTTPS monitors, so this conversion is explicit.
func monitorTypeForRequest(monitor uptimerobotv1.MonitorValues) string {
	if monitor.APIAssertions != nil && len(monitor.APIAssertions.Checks) > 0 {
		return apiMonitorType
	}
	return monitor.Type.ToAPIString()
}

func parseAPIAssertionTarget(operator urtypes.AssertionOperator, value string) interface{} {
	switch operator {
	case urtypes.AssertionGreaterThan, urtypes.AssertionLessThan:
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
	}
	return value
}

// CreateSlackIntegration creates a Slack integration using the v3 API.
// POST /integrations
func (c Client) CreateSlackIntegration(ctx context.Context, data SlackIntegrationData) (IntegrationResponse, error) {
	var result IntegrationResponse
	req := CreateSlackIntegrationRequest{
		Type: slackIntegrationType,
		Data: data,
	}
	err := c.doJSON(ctx, http.MethodPost, "integrations", req, &result)
	if err == nil {
		return result, nil
	}

	if errors.Is(err, ErrStatus) && strings.Contains(err.Error(), "409 Conflict") {
		body := extractErrStatusBody(err)
		if isSlackIntegrationAlreadyExists409(body) {
			integrations, listErr := c.ListIntegrations(ctx)
			if listErr == nil {
				if integration, ok := selectDuplicateSlackIntegrationCandidate(integrations, data); ok {
					return *integration, nil
				}
			}
		}
	}

	return result, err
}

// ListIntegrations lists all integrations using the v3 API, following
// nextLink pagination so callers (notably 409 adoption) can find matches
// that live on later pages.
// GET /integrations
func (c Client) ListIntegrations(ctx context.Context) ([]IntegrationResponse, error) {
	var all []IntegrationResponse
	var resp IntegrationsListResponse
	if err := c.doJSON(ctx, http.MethodGet, "integrations", nil, &resp); err != nil {
		return nil, err
	}
	all = append(all, resp.Integrations...)
	for resp.NextLink != nil && *resp.NextLink != "" {
		nextURL := *resp.NextLink
		if !strings.HasPrefix(nextURL, "http") {
			nextURL = strings.TrimSuffix(c.url, "/") + "/" + strings.TrimPrefix(nextURL, "/")
		}
		resp = IntegrationsListResponse{}
		if err := c.doGetJSON(ctx, nextURL, &resp); err != nil {
			return nil, err
		}
		all = append(all, resp.Integrations...)
	}
	return all, nil
}

// DeleteIntegration deletes an integration by ID using the v3 API.
// DELETE /integrations/{id}
func (c Client) DeleteIntegration(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("integrations/%d", id)
	err := c.doJSON(ctx, http.MethodDelete, endpoint, nil, nil)
	if err != nil && IsNotFound(err) {
		return nil
	}
	return err
}

// CreateMaintenanceWindow creates a new maintenance window using the v3 API.
func (c Client) CreateMaintenanceWindow(ctx context.Context, req CreateMaintenanceWindowRequest) (MaintenanceWindowResponse, error) {
	var result MaintenanceWindowResponse
	err := c.doJSON(ctx, http.MethodPost, "maintenance-windows", req, &result)
	return result, err
}

// GetMaintenanceWindow retrieves a maintenance window by ID using the v3 API.
func (c Client) GetMaintenanceWindow(ctx context.Context, id string) (MaintenanceWindowResponse, error) {
	var result MaintenanceWindowResponse
	endpoint := fmt.Sprintf("maintenance-windows/%s", id)
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &result)
	return result, err
}

// UpdateMaintenanceWindow updates a maintenance window using the v3 API.
func (c Client) UpdateMaintenanceWindow(ctx context.Context, id string, req UpdateMaintenanceWindowRequest) (MaintenanceWindowResponse, error) {
	var result MaintenanceWindowResponse
	endpoint := fmt.Sprintf("maintenance-windows/%s", id)
	err := c.doJSON(ctx, http.MethodPatch, endpoint, req, &result)
	return result, err
}

// DeleteMaintenanceWindow deletes a maintenance window using the v3 API.
func (c Client) DeleteMaintenanceWindow(ctx context.Context, id string) error {
	endpoint := fmt.Sprintf("maintenance-windows/%s", id)
	err := c.doJSON(ctx, http.MethodDelete, endpoint, nil, nil)
	if err != nil && IsNotFound(err) {
		return nil
	}
	return err
}

// ListMaintenanceWindows lists all maintenance windows using the v3 API.
func (c Client) ListMaintenanceWindows(ctx context.Context) ([]MaintenanceWindowResponse, error) {
	var result MaintenanceWindowsListResponse
	err := c.doJSON(ctx, http.MethodGet, "maintenance-windows", nil, &result)
	if err != nil {
		return nil, err
	}
	return result.MaintenanceWindows, nil
}

// SpawnGroupInBackend provisions new collection via POST
func (c Client) SpawnGroupInBackend(ctx context.Context, wirePayload GroupCreationWireFormat) (GroupWireFormat, error) {
	var responsePayload GroupWireFormat
	transmitErr := c.doJSON(ctx, http.MethodPost, "monitor-groups", wirePayload, &responsePayload)
	return responsePayload, transmitErr
}

// FetchGroupFromBackend retrieves specific collection via GET
func (c Client) FetchGroupFromBackend(ctx context.Context, groupIDString string) (GroupWireFormat, error) {
	var responsePayload GroupWireFormat
	endpointPath := fmt.Sprintf("monitor-groups/%s", groupIDString)
	transmitErr := c.doJSON(ctx, http.MethodGet, endpointPath, nil, &responsePayload)
	return responsePayload, transmitErr
}

// MutateGroupInBackend applies changes to existing collection via PATCH
func (c Client) MutateGroupInBackend(ctx context.Context, groupIDString string, wirePayload GroupUpdateWireFormat) (GroupWireFormat, error) {
	var responsePayload GroupWireFormat
	endpointPath := fmt.Sprintf("monitor-groups/%s", groupIDString)
	transmitErr := c.doJSON(ctx, http.MethodPatch, endpointPath, wirePayload, &responsePayload)
	return responsePayload, transmitErr
}

// PurgeGroupFromBackend destroys collection via DELETE
func (c Client) PurgeGroupFromBackend(ctx context.Context, groupIDString string) error {
	endpointPath := fmt.Sprintf("monitor-groups/%s", groupIDString)
	err := c.doJSON(ctx, http.MethodDelete, endpointPath, nil, nil)
	if err != nil && IsNotFound(err) {
		return nil
	}
	return err
}

// EnumerateGroupsFromBackend fetches all collections via GET
func (c Client) EnumerateGroupsFromBackend(ctx context.Context) ([]GroupWireFormat, error) {
	var responsePayload GroupListWireFormat
	transmitErr := c.doJSON(ctx, http.MethodGet, "monitor-groups", nil, &responsePayload)
	if transmitErr != nil {
		return nil, transmitErr
	}
	return responsePayload.Groups, nil
}

// PauseMonitor pauses a monitor by ID using the v3 API.
// POST /monitors/{id}/pause
// This operation is idempotent - pausing an already paused monitor will return successfully.
func (c Client) PauseMonitor(ctx context.Context, id string) error {
	endpoint := fmt.Sprintf("monitors/%s/pause", id)
	return c.doJSON(ctx, http.MethodPost, endpoint, nil, nil)
}

// StartMonitor starts (resumes) a paused monitor by ID using the v3 API.
// POST /monitors/{id}/start
// This operation is idempotent - starting an already active monitor will return successfully.
func (c Client) StartMonitor(ctx context.Context, id string) error {
	endpoint := fmt.Sprintf("monitors/%s/start", id)
	return c.doJSON(ctx, http.MethodPost, endpoint, nil, nil)
}
