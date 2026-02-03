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
	ErrStatus          = errors.New("error code from Uptime Robot API")
	ErrResponse        = errors.New("received fail from Uptime Robot API")
	ErrMonitorNotFound = errors.New("monitor not found")
	ErrContactNotFound = errors.New("contact not found")
)

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

// do executes an HTTP request and returns the response.
func (c Client) do(req *http.Request) (*http.Response, error) {
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	if res.StatusCode >= 400 {
		defer func() { _ = res.Body.Close() }()
		body, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("%w: %s - %s", ErrStatus, res.Status, string(body))
	}

	return res, nil
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
		if monitor.Keyword.CaseSensitive != nil && *monitor.Keyword.CaseSensitive {
			req.KeywordCaseType = "CaseSensitive"
		} else {
			req.KeywordCaseType = "CaseInsensitive"
		}
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
		if monitor.Keyword.CaseSensitive != nil && *monitor.Keyword.CaseSensitive {
			req.KeywordCaseType = "CaseSensitive"
		} else {
			req.KeywordCaseType = "CaseInsensitive"
		}
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

func monitorMatchesExpected(existing *MonitorResponse, desired uptimerobotv1.MonitorValues) bool {
	if existing == nil {
		return false
	}
	if existing.Type != desired.Type.ToAPIString() {
		return false
	}
	if desired.URL != "" && existing.URL != desired.URL {
		return false
	}
	if desired.Interval != nil {
		desiredInterval := int(desired.Interval.Seconds())
		if desiredInterval > 0 && existing.Interval != desiredInterval {
			return false
		}
	}
	return true
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

	var resp MonitorCreateResponse
	if err := c.doJSON(ctx, http.MethodPost, "monitors", reqBody, &resp); err != nil {
		// If creation fails (e.g., 409 Conflict), check if monitor already exists by name
		if monitor.URL != "" {
			if id, findErr := c.FindMonitorByURL(ctx, monitor.URL); findErr == nil {
				return CreateMonitorResult{ID: id}, nil
			}
		}
		// For heartbeat monitors (no URL), try to find by name
		if m, findErr := c.FindMonitorByName(ctx, monitor.Name); findErr == nil {
			if !monitorMatchesExpected(m, monitor) {
				return CreateMonitorResult{}, fmt.Errorf("%w: monitor %q exists but does not match desired configuration", err, monitor.Name)
			}
			return CreateMonitorResult{ID: strconv.Itoa(m.ID), URL: m.URL}, nil
		}
		return CreateMonitorResult{}, err
	}

	return CreateMonitorResult{
		ID:  strconv.Itoa(resp.ID),
		URL: resp.URL,
	}, nil
}

// FindMonitorByURL searches for a monitor by its URL by fetching all monitors.
// This is used as a fallback when the v3 API doesn't support URL filtering via query params.
func (c Client) FindMonitorByURL(ctx context.Context, url string) (string, error) {
	var resp MonitorsListResponse
	if err := c.doJSON(ctx, http.MethodGet, "monitors", nil, &resp); err != nil {
		return "", err
	}

	for _, m := range resp.Monitors {
		if m.URL == url {
			return strconv.Itoa(m.ID), nil
		}
	}

	return "", ErrMonitorNotFound
}

// FindMonitorByName searches for a monitor by its friendly name.
// This is useful for heartbeat monitors which don't have a user-defined URL.
func (c Client) FindMonitorByName(ctx context.Context, name string) (*MonitorResponse, error) {
	var resp MonitorsListResponse
	if err := c.doJSON(ctx, http.MethodGet, "monitors", nil, &resp); err != nil {
		return nil, err
	}

	for _, m := range resp.Monitors {
		if m.FriendlyName == name {
			return &m, nil
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
