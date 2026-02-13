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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	uptimerobotv1 "github.com/joelp172/uptime-robot-operator/api/v1alpha1"
	"github.com/joelp172/uptime-robot-operator/internal/uptimerobot/urtypes"
)

// NewClient creates a new UptimeRobot API v3 client.
func NewClient(apiKey string) Client {
	api := "https://api.uptimerobot.com/v3"
	if env := os.Getenv("UPTIME_ROBOT_API"); env != "" {
		api = strings.TrimSuffix(env, "/")
	}

	return Client{url: api, apiKey: apiKey}
}

// Client is the UptimeRobot API v3 client.
type Client struct {
	url    string
	apiKey string
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
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, bodyReader)
	if err != nil {
		return nil, err
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
		Type:         monitor.Type.ToAPIString(),
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
	if monitor.Type == urtypes.TypeDNS && monitor.DNS != nil {
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
	if monitor.Type == urtypes.TypeHeartbeat {
		req.Config = &MonitorConfig{}
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
		Timeout:      int(monitor.Timeout.Seconds()),
		GracePeriod:  gracePeriod,
		// Note: Status is not supported in v3 PATCH requests - use pause/resume endpoints instead
		HTTPMethod: httpMethodToString(monitor.Method),
	}

	// UptimeRobot v3 rejects URL updates for DNS monitors.
	if monitor.Type != urtypes.TypeDNS && monitor.Type != urtypes.TypeHeartbeat {
		req.URL = monitor.URL
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
	if monitor.Type == urtypes.TypeDNS && monitor.DNS != nil {
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
	if monitor.Type == urtypes.TypeHeartbeat {
		req.Config = &MonitorConfig{}
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
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return CreateMonitorResult{}, err
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()

	if res.StatusCode == http.StatusCreated || res.StatusCode == http.StatusOK {
		var resp MonitorCreateResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return CreateMonitorResult{}, err
		}
		return CreateMonitorResult{
			ID:  strconv.Itoa(resp.ID),
			URL: resp.URL,
		}, nil
	}

	if res.StatusCode == http.StatusConflict {
		// 409 Duplicate: resolve existing monitor ID and adopt it so reconciliation can continue.
		if id := parseMonitorIDFrom409Body(body); id != "" {
			m, getErr := c.GetMonitor(ctx, id)
			if getErr == nil {
				return CreateMonitorResult{ID: id, URL: m.URL}, nil
			}
			return CreateMonitorResult{ID: id}, nil
		}

		// Try by URL first (most reliable for HTTP/HTTPS duplicates), then by name. Retry once on rate limit.
		tryFind := func() (CreateMonitorResult, bool) {
			if monitor.URL != "" {
				if id, findErr := c.FindMonitorByURL(ctx, monitor.URL); findErr == nil {
					// Fetch full monitor details to get URL (especially important for heartbeat monitors)
					if m, getErr := c.GetMonitor(ctx, id); getErr == nil {
						return CreateMonitorResult{ID: id, URL: m.URL}, true
					}
					// Fallback: return just ID if GetMonitor fails
					return CreateMonitorResult{ID: id}, true
				}
			}
			if m, findErr := c.FindMonitorByName(ctx, monitor.Name); findErr == nil {
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
	return CreateMonitorResult{}, fmt.Errorf("%w: %s - %s", ErrStatus, res.Status, string(body))
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

	return "", ErrContactNotFound
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

// CreateSlackIntegration creates a Slack integration using the v3 API.
// POST /integrations
func (c Client) CreateSlackIntegration(ctx context.Context, data SlackIntegrationData) (IntegrationResponse, error) {
	var result IntegrationResponse
	req := CreateSlackIntegrationRequest{
		Type: "Slack",
		Data: data,
	}
	err := c.doJSON(ctx, http.MethodPost, "integrations", req, &result)
	return result, err
}

// ListIntegrations lists integrations using the v3 API.
// GET /integrations
func (c Client) ListIntegrations(ctx context.Context) ([]IntegrationResponse, error) {
	var result IntegrationsListResponse
	err := c.doJSON(ctx, http.MethodGet, "integrations", nil, &result)
	if err != nil {
		return nil, err
	}
	return result.Integrations, nil
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
	return c.doJSON(ctx, http.MethodDelete, endpoint, nil, nil)
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
	return c.doJSON(ctx, http.MethodDelete, endpointPath, nil, nil)
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
