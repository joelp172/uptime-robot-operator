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

// MonitorConfig represents the config object for certain monitor types (DNS, Heartbeat, etc.)
// The v3 API uses a config object for type-specific settings.
type MonitorConfig struct {
	// Empty struct - the v3 API requires a config object but specific DNS/Heartbeat
	// fields are not yet documented. The API accepts an empty config object.
}

// CreateMonitorRequest represents the v3 API request payload for creating a monitor.
// Note: The v3 API uses camelCase field names.
type CreateMonitorRequest struct {
	FriendlyName          string                        `json:"friendlyName"`
	URL                   string                        `json:"url"`
	Type                  string                        `json:"type"` // "HTTP", "KEYWORD", "PING", "PORT", "HEARTBEAT", "DNS"
	Interval              int                           `json:"interval"`
	Timeout               int                           `json:"timeout,omitempty"`
	GracePeriod           int                           `json:"gracePeriod"`              // Required: seconds to wait before alerting (0-86400)
	HTTPMethod            string                        `json:"httpMethodType,omitempty"` // HEAD, GET, POST, PUT, PATCH, DELETE, OPTIONS
	HTTPUsername          string                        `json:"httpUsername,omitempty"`
	HTTPPassword          string                        `json:"httpPassword,omitempty"`
	HTTPAuthType          string                        `json:"httpAuthType,omitempty"` // "basic", "digest"
	PostType              string                        `json:"postType,omitempty"`
	PostContentType       string                        `json:"postContentType,omitempty"`
	PostValue             string                        `json:"postValue,omitempty"`
	KeywordType           string                        `json:"keywordType,omitempty"`     // "exists", "not_exists"
	KeywordCaseType       string                        `json:"keywordCaseType,omitempty"` // "case_sensitive", "case_insensitive"
	KeywordValue          string                        `json:"keywordValue,omitempty"`
	SubType               string                        `json:"subType,omitempty"` // For port monitors
	Port                  int                           `json:"port,omitempty"`
	AssignedAlertContacts []AssignedAlertContactRequest `json:"assignedAlertContacts,omitempty"`
	// Config object required for DNS, Heartbeat monitors
	Config *MonitorConfig `json:"config,omitempty"`
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
	HTTPAuthType          string                        `json:"httpAuthType,omitempty"`
	PostType              string                        `json:"postType,omitempty"`
	PostContentType       string                        `json:"postContentType,omitempty"`
	PostValue             string                        `json:"postValue,omitempty"`
	KeywordType           string                        `json:"keywordType,omitempty"`
	KeywordCaseType       string                        `json:"keywordCaseType,omitempty"`
	KeywordValue          string                        `json:"keywordValue,omitempty"`
	SubType               string                        `json:"subType,omitempty"`
	Port                  int                           `json:"port,omitempty"`
	AssignedAlertContacts []AssignedAlertContactRequest `json:"assignedAlertContacts,omitempty"`
	// Config object required for DNS, Heartbeat monitors
	Config *MonitorConfig `json:"config,omitempty"`
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

// MonitorResponse represents a single monitor in v3 API responses.
// Note: The v3 API uses camelCase field names and status as string.
type MonitorResponse struct {
	ID           int    `json:"id"`
	FriendlyName string `json:"friendlyName"`
	URL          string `json:"url"`
	Type         string `json:"type"`
	Status       string `json:"status"` // e.g., "UP", "DOWN", "STARTED", "PAUSED"
	Interval     int    `json:"interval"`
}

// MonitorsListResponse represents the v3 API response for listing monitors.
// Note: v3 API returns monitors in the "data" field, not "monitors"
type MonitorsListResponse struct {
	Monitors []MonitorResponse `json:"data"`
	NextLink *string           `json:"nextLink"`
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
