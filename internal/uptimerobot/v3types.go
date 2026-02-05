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

import "encoding/json"

// MonitorConfig represents the config object for certain monitor types (DNS, Heartbeat, etc.)
// The v3 API uses a config object for type-specific settings.
type MonitorConfig struct {
	// DNSRecords contains expected DNS record values keyed by record type.
	DNSRecords *DNSRecordsConfig `json:"dnsRecords,omitempty"`

	// SSLExpirationPeriodDays - days before SSL expiration to notify.
	SSLExpirationPeriodDays []int `json:"sslExpirationPeriodDays,omitempty"`
}

// DNSRecordsConfig specifies expected DNS record values for each record type.
type DNSRecordsConfig struct {
	A      []string `json:"A,omitempty"`
	AAAA   []string `json:"AAAA,omitempty"`
	CNAME  []string `json:"CNAME,omitempty"`
	MX     []string `json:"MX,omitempty"`
	NS     []string `json:"NS,omitempty"`
	TXT    []string `json:"TXT,omitempty"`
	SRV    []string `json:"SRV,omitempty"`
	PTR    []string `json:"PTR,omitempty"`
	SOA    []string `json:"SOA,omitempty"`
	SPF    []string `json:"SPF,omitempty"`
	DNSKEY []string `json:"DNSKEY,omitempty"`
	DS     []string `json:"DS,omitempty"`
	NSEC   []string `json:"NSEC,omitempty"`
	NSEC3  []string `json:"NSEC3,omitempty"`
}

// CreateMonitorRequest represents the v3 API request payload for creating a monitor.
// Note: The v3 API uses camelCase field names.
type CreateMonitorRequest struct {
	FriendlyName          string                        `json:"friendlyName"`
	URL                   string                        `json:"url,omitempty"`
	Type                  string                        `json:"type"` // "HTTP", "KEYWORD", "PING", "PORT", "HEARTBEAT", "DNS"
	Interval              int                           `json:"interval"`
	Timeout               int                           `json:"timeout,omitempty"`
	GracePeriod           int                           `json:"gracePeriod"`              // Required: seconds to wait before alerting (0-86400)
	HTTPMethod            string                        `json:"httpMethodType,omitempty"` // HEAD, GET, POST, PUT, PATCH, DELETE, OPTIONS
	HTTPUsername          string                        `json:"httpUsername,omitempty"`
	HTTPPassword          string                        `json:"httpPassword,omitempty"`
	HTTPAuthType          string                        `json:"authType,omitempty"` // "NONE", "HTTP_BASIC", "DIGEST"
	PostType              string                        `json:"postValueType,omitempty"`
	PostValue             string                        `json:"postValueData,omitempty"`
	KeywordType           string                        `json:"keywordType,omitempty"`     // "ALERT_EXISTS", "ALERT_NOT_EXISTS"
	KeywordCaseType       *int                          `json:"keywordCaseType,omitempty"` // 0=CaseInsensitive, 1=CaseSensitive
	KeywordValue          string                        `json:"keywordValue,omitempty"`
	Port                  int                           `json:"port,omitempty"`
	AssignedAlertContacts []AssignedAlertContactRequest `json:"assignedAlertContacts,omitempty"`
	// Config object for DNS monitors
	Config *MonitorConfig `json:"config,omitempty"`

	// New v3 API fields
	CustomHTTPHeaders        map[string]string `json:"customHttpHeaders,omitempty"`
	SuccessHTTPResponseCodes []string          `json:"successHttpResponseCodes,omitempty"`
	CheckSSLErrors           *bool             `json:"checkSSLErrors,omitempty"`
	TagNames                 []string          `json:"tagNames,omitempty"`
	MaintenanceWindowsIds    []int             `json:"maintenanceWindowsIds,omitempty"`
	DomainExpirationReminder *bool             `json:"domainExpirationReminder,omitempty"`
	SSLExpirationReminder    *bool             `json:"sslExpirationReminder,omitempty"`
	FollowRedirections       *bool             `json:"followRedirections,omitempty"`
	ResponseTimeThreshold    *int              `json:"responseTimeThreshold,omitempty"`
	RegionalData             string            `json:"regionalData,omitempty"`
	GroupID                  *int              `json:"groupId,omitempty"`
}

// UpdateMonitorRequest represents the v3 API request payload for updating a monitor.
// Note: The v3 API uses camelCase field names. Status is not supported in PATCH/update requests.
type UpdateMonitorRequest struct {
	FriendlyName string `json:"friendlyName,omitempty"`
	URL          string `json:"url,omitempty"`
	Interval     int    `json:"interval,omitempty"`
	Timeout      int    `json:"timeout,omitempty"`
	GracePeriod  int    `json:"gracePeriod,omitempty"` // Seconds to wait before alerting (0-86400)
	// Note: Status field is not supported in v3 API PATCH requests - use pause/resume endpoints instead
	HTTPMethod            string                        `json:"httpMethodType,omitempty"`
	HTTPUsername          string                        `json:"httpUsername,omitempty"`
	HTTPPassword          string                        `json:"httpPassword,omitempty"`
	HTTPAuthType          string                        `json:"authType,omitempty"`
	PostType              string                        `json:"postValueType,omitempty"`
	PostValue             string                        `json:"postValueData,omitempty"`
	KeywordType           string                        `json:"keywordType,omitempty"`
	KeywordCaseType       *int                          `json:"keywordCaseType,omitempty"` // 0=CaseInsensitive, 1=CaseSensitive
	KeywordValue          string                        `json:"keywordValue,omitempty"`
	Port                  int                           `json:"port,omitempty"`
	AssignedAlertContacts []AssignedAlertContactRequest `json:"assignedAlertContacts,omitempty"`
	// Config object for DNS monitors
	Config *MonitorConfig `json:"config,omitempty"`

	// New v3 API fields
	CustomHTTPHeaders        map[string]string `json:"customHttpHeaders,omitempty"`
	SuccessHTTPResponseCodes []string          `json:"successHttpResponseCodes,omitempty"`
	CheckSSLErrors           *bool             `json:"checkSSLErrors,omitempty"`
	TagNames                 []string          `json:"tagNames,omitempty"`
	MaintenanceWindowsIds    []int             `json:"maintenanceWindowsIds,omitempty"`
	DomainExpirationReminder *bool             `json:"domainExpirationReminder,omitempty"`
	SSLExpirationReminder    *bool             `json:"sslExpirationReminder,omitempty"`
	FollowRedirections       *bool             `json:"followRedirections,omitempty"`
	ResponseTimeThreshold    *int              `json:"responseTimeThreshold,omitempty"`
	RegionalData             string            `json:"regionalData,omitempty"`
	GroupID                  *int              `json:"groupId,omitempty"`
}

// AssignedAlertContactRequest represents an alert contact assignment in v3 API.
// Note: The v3 API uses camelCase field names and alertContactId is a string.
type AssignedAlertContactRequest struct {
	AlertContactID string `json:"alertContactId"` // Alert contact ID (string in v3)
	Threshold      int    `json:"threshold"`      // Per-contact threshold in seconds
	Recurrence     int    `json:"recurrence"`     // Minutes between repeat notifications (0 = disabled)
}

// AlertContactRequest is deprecated, use AssignedAlertContactRequest instead.
// Kept for backwards compatibility.
type AlertContactRequest = AssignedAlertContactRequest

// TagResponse represents a tag in v3 API responses.
type TagResponse struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// AssignedAlertContactResp represents an assigned alert contact in v3 API responses.
// Note: API returns alertContactId as number in list responses, but we send as string in requests.
type AssignedAlertContactResp struct {
	AlertContactID interface{} `json:"alertContactId"` // Can be int or string depending on endpoint
	Threshold      int         `json:"threshold"`
	Recurrence     int         `json:"recurrence"`
}

// RegionalDataResponse represents regional monitoring config in v3 API responses.
type RegionalDataResponse struct {
	Region         []string `json:"REGION"`
	ManualSelected bool     `json:"MANUAL_SELECTED"`
	Infrastructure string   `json:"INFRASTRUCTURE"`
}

// MonitorConfigResponse represents the config object in v3 API monitor responses.
type MonitorConfigResponse struct {
	DNSRecords              *DNSRecordsConfig `json:"dnsRecords,omitempty"`
	SSLExpirationPeriodDays []int             `json:"sslExpirationPeriodDays,omitempty"`
}

// MonitorResponse represents a single monitor in v3 API responses.
// Note: The v3 API uses camelCase field names and status as string.
// Expanded to include all fields from monitor-response.json schema for e2e validation.
type MonitorResponse struct {
	ID                       int                        `json:"id"`
	FriendlyName             string                     `json:"friendlyName"`
	URL                      string                     `json:"url"`
	Type                     string                     `json:"type"`
	Status                   string                     `json:"status"` // e.g., "UP", "DOWN", "STARTED", "PAUSED"
	Interval                 int                        `json:"interval"`
	Timeout                  *int                       `json:"timeout,omitempty"`
	GracePeriod              *int                       `json:"gracePeriod,omitempty"`
	HTTPMethodType           string                     `json:"httpMethodType,omitempty"`
	HTTPUsername             string                     `json:"httpUsername,omitempty"`
	AuthType                 string                     `json:"authType,omitempty"`
	KeywordType              string                     `json:"keywordType,omitempty"`
	KeywordCaseType          *int                       `json:"keywordCaseType,omitempty"` // 0=CaseInsensitive, 1=CaseSensitive
	KeywordValue             string                     `json:"keywordValue,omitempty"`
	Port                     *int                       `json:"port,omitempty"`
	PostValueType            string                     `json:"postValueType,omitempty"`
	PostValueData            interface{}                `json:"postValueData,omitempty"` // Can be string or object
	CustomHTTPHeaders        map[string]string          `json:"customHttpHeaders,omitempty"`
	SuccessHTTPResponseCodes []string                   `json:"successHttpResponseCodes,omitempty"`
	CheckSSLErrors           *bool                      `json:"checkSSLErrors,omitempty"`
	SSLExpirationReminder    *bool                      `json:"sslExpirationReminder,omitempty"`
	DomainExpirationReminder *bool                      `json:"domainExpirationReminder,omitempty"`
	FollowRedirections       *bool                      `json:"followRedirections,omitempty"`
	ResponseTimeThreshold    *int                       `json:"responseTimeThreshold,omitempty"`
	Config                   *MonitorConfigResponse     `json:"config,omitempty"`
	Tags                     []TagResponse              `json:"tags,omitempty"`
	AssignedAlertContacts    []AssignedAlertContactResp `json:"assignedAlertContacts,omitempty"`
	MaintenanceWindows       []MaintenanceWindowSummary `json:"maintenanceWindows,omitempty"`
	RegionalData             *RegionalDataResponse      `json:"regionalData,omitempty"`
	GroupID                  *int                       `json:"groupId,omitempty"`
}

// MaintenanceWindowSummary represents a maintenance window in monitor API responses
type MaintenanceWindowSummary struct {
	ID              int    `json:"id"`
	UserID          int    `json:"userId,omitempty"`
	Name            string `json:"name"`
	Interval        string `json:"interval"`
	Date            string `json:"date,omitempty"`
	Time            string `json:"time"`
	Duration        int    `json:"duration"`
	AutoAddMonitors bool   `json:"autoAddMonitors,omitempty"`
	Days            []int  `json:"days,omitempty"`
	Status          string `json:"status,omitempty"`
	Created         string `json:"created,omitempty"`
}

// MonitorsListResponse represents the v3 API response for listing monitors.
// Accepts either "data" or "monitors" as the array key (API docs vary by version).
type MonitorsListResponse struct {
	Monitors []MonitorResponse `json:"-"`
	NextLink *string           `json:"nextLink"`
}

// UnmarshalJSON supports both "data" and "monitors" keys so list works with either API shape.
func (r *MonitorsListResponse) UnmarshalJSON(data []byte) error {
	var raw struct {
		Data     []MonitorResponse `json:"data"`
		Monitors []MonitorResponse `json:"monitors"`
		NextLink *string           `json:"nextLink"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if len(raw.Data) > 0 {
		r.Monitors = raw.Data
	} else {
		r.Monitors = raw.Monitors
	}
	r.NextLink = raw.NextLink
	return nil
}

// MarshalJSON writes Monitors under "data" for compatibility.
func (r *MonitorsListResponse) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Data     []MonitorResponse `json:"data"`
		NextLink *string           `json:"nextLink"`
	}{r.Monitors, r.NextLink})
}

// MonitorCreateResponse is an alias for MonitorResponse since v3 API
// returns the created monitor directly without wrapping.
type MonitorCreateResponse = MonitorResponse

// MonitorUpdateResponse is an alias for MonitorResponse since v3 API
// returns the updated monitor directly without wrapping.
type MonitorUpdateResponse = MonitorResponse

// PaginationInfo represents cursor-based pagination in v3 API responses.
type PaginationInfo struct {
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit"`
	Total  int    `json:"total"`
}

// AlertContactResponse represents an alert contact in v3 API responses.
// Note: The v3 API returns camelCase field names and type/status as strings.
type AlertContactResponse struct {
	ID           int     `json:"id"`
	FriendlyName *string `json:"friendlyName"` // Pointer to handle null values
	Type         string  `json:"type"`
	Status       string  `json:"status"`
	Value        string  `json:"value"`
}

// AlertContactsListResponse represents the v3 API response for listing alert contacts.
type AlertContactsListResponse struct {
	AlertContacts []AlertContactResponse `json:"alert_contacts"`
	Pagination    PaginationInfo         `json:"pagination"`
}

// UserMeResponse represents the v3 API response for /user/me endpoint.
// Note: The v3 API returns user info directly without a wrapper object.
type UserMeResponse struct {
	Email         string `json:"email"`
	FullName      string `json:"fullName"`
	MonitorsCount int    `json:"monitorsCount"`
	MonitorLimit  int    `json:"monitorLimit"`
	SMSCredits    int    `json:"smsCredits"`
}

// APIError represents an error response from the v3 API.
type APIError struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// CreateMaintenanceWindowRequest represents the v3 API request payload for creating a maintenance window.
type CreateMaintenanceWindowRequest struct {
	Name            string `json:"name"`
	AutoAddMonitors bool   `json:"autoAddMonitors,omitempty"`
	Interval        string `json:"interval"`       // "once", "daily", "weekly", "monthly"
	Date            string `json:"date,omitempty"` // YYYY-MM-DD format (required only for "once" interval)
	Time            string `json:"time"`           // HH:mm:ss format
	Duration        int    `json:"duration"`       // Duration in minutes
	Days            []int  `json:"days,omitempty"` // Required for weekly/monthly intervals
	MonitorIDs      []int  `json:"monitorIds,omitempty"`
}

// UpdateMaintenanceWindowRequest represents the v3 API request payload for updating a maintenance window.
type UpdateMaintenanceWindowRequest struct {
	Name            string `json:"name,omitempty"`
	AutoAddMonitors *bool  `json:"autoAddMonitors,omitempty"`
	Interval        string `json:"interval,omitempty"`
	Date            string `json:"date,omitempty"`
	Time            string `json:"time,omitempty"`
	Duration        int    `json:"duration,omitempty"`
	Days            []int  `json:"days,omitempty"`
	MonitorIDs      *[]int `json:"monitorIds,omitempty"`
}

// MaintenanceWindowResponse represents a maintenance window in v3 API responses.
type MaintenanceWindowResponse struct {
	ID              int    `json:"id"`
	UserID          int    `json:"userId"`
	Name            string `json:"name"`
	AutoAddMonitors bool   `json:"autoAddMonitors"`
	Interval        string `json:"interval"`
	Date            string `json:"date,omitempty"` // Only present for "once" interval
	Time            string `json:"time"`
	Duration        int    `json:"duration"`
	Days            []int  `json:"days,omitempty"`
	MonitorIDs      []int  `json:"monitorIds,omitempty"`
	Status          string `json:"status"`  // e.g., "active"
	Created         string `json:"created"` // ISO 8601 timestamp
}

// MaintenanceWindowsListResponse represents the v3 API response for listing maintenance windows.
type MaintenanceWindowsListResponse struct {
	MaintenanceWindows []MaintenanceWindowResponse `json:"data"`
	NextLink           *string                     `json:"nextLink"`
}

// GroupCreationWireFormat structures request payload for spawning collections
type GroupCreationWireFormat struct {
	Name       string `json:"name"`
	MonitorIDs []int  `json:"monitorIds,omitempty"`
	GroupIDs   []int  `json:"groupIds,omitempty"`
}

// GroupUpdateWireFormat structures request payload for mutating collections
type GroupUpdateWireFormat struct {
	Name       string `json:"name,omitempty"`
	MonitorIDs *[]int `json:"monitorIds,omitempty"`
	GroupIDs   []int  `json:"groupIds,omitempty"`
}

// GroupWireFormat represents collection data from backend
type GroupWireFormat struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
	MonitorIDs []int  `json:"monitorIds,omitempty"`
}

// GroupListWireFormat wraps multiple collections from list endpoints
type GroupListWireFormat struct {
	Groups   []GroupWireFormat `json:"data"`
	NextLink *string           `json:"nextLink"`
}
