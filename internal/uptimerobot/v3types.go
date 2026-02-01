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

// CreateMonitorRequest represents the v3 API request payload for creating a monitor.
type CreateMonitorRequest struct {
	FriendlyName    string                `json:"friendly_name"`
	URL             string                `json:"url"`
	Type            string                `json:"type"` // "HTTP", "Keyword", "Ping", "Port", "Heartbeat", "DNS"
	Interval        int                   `json:"interval"`
	Timeout         int                   `json:"timeout,omitempty"`
	HTTPMethod      string                `json:"http_method,omitempty"`
	HTTPUsername    string                `json:"http_username,omitempty"`
	HTTPPassword    string                `json:"http_password,omitempty"`
	HTTPAuthType    string                `json:"http_auth_type,omitempty"` // "basic", "digest"
	PostType        string                `json:"post_type,omitempty"`
	PostContentType string                `json:"post_content_type,omitempty"`
	PostValue       string                `json:"post_value,omitempty"`
	KeywordType     string                `json:"keyword_type,omitempty"`      // "exists", "not_exists"
	KeywordCaseType string                `json:"keyword_case_type,omitempty"` // "case_sensitive", "case_insensitive"
	KeywordValue    string                `json:"keyword_value,omitempty"`
	SubType         string                `json:"sub_type,omitempty"` // For port monitors
	Port            int                   `json:"port,omitempty"`
	AlertContacts   []AlertContactRequest `json:"alert_contacts,omitempty"`
	// DNS monitor specific fields
	DNSRecordType string `json:"dns_record_type,omitempty"` // "A", "AAAA", "MX", etc.
	DNSValue      string `json:"dns_value,omitempty"`
}

// UpdateMonitorRequest represents the v3 API request payload for updating a monitor.
type UpdateMonitorRequest struct {
	FriendlyName    string                `json:"friendly_name,omitempty"`
	URL             string                `json:"url,omitempty"`
	Interval        int                   `json:"interval,omitempty"`
	Timeout         int                   `json:"timeout,omitempty"`
	Status          int                   `json:"status,omitempty"` // 0 = paused, 1 = running
	HTTPMethod      string                `json:"http_method,omitempty"`
	HTTPUsername    string                `json:"http_username,omitempty"`
	HTTPPassword    string                `json:"http_password,omitempty"`
	HTTPAuthType    string                `json:"http_auth_type,omitempty"`
	PostType        string                `json:"post_type,omitempty"`
	PostContentType string                `json:"post_content_type,omitempty"`
	PostValue       string                `json:"post_value,omitempty"`
	KeywordType     string                `json:"keyword_type,omitempty"`
	KeywordCaseType string                `json:"keyword_case_type,omitempty"`
	KeywordValue    string                `json:"keyword_value,omitempty"`
	SubType         string                `json:"sub_type,omitempty"`
	Port            int                   `json:"port,omitempty"`
	AlertContacts   []AlertContactRequest `json:"alert_contacts,omitempty"`
	DNSRecordType   string                `json:"dns_record_type,omitempty"`
	DNSValue        string                `json:"dns_value,omitempty"`
}

// AlertContactRequest represents an alert contact assignment in v3 API.
type AlertContactRequest struct {
	ID         string `json:"id"`
	Threshold  int    `json:"threshold"`  // Minutes to wait before alerting
	Recurrence int    `json:"recurrence"` // Minutes between repeat notifications (0 = disabled)
}

// MonitorResponse represents a single monitor in v3 API responses.
type MonitorResponse struct {
	ID           int    `json:"id"`
	FriendlyName string `json:"friendly_name"`
	URL          string `json:"url"`
	Type         string `json:"type"`
	Status       int    `json:"status"`
	Interval     int    `json:"interval"`
}

// MonitorsListResponse represents the v3 API response for listing monitors.
type MonitorsListResponse struct {
	Monitors   []MonitorResponse `json:"monitors"`
	Pagination PaginationInfo    `json:"pagination"`
}

// MonitorCreateResponse represents the v3 API response for creating a monitor.
type MonitorCreateResponse struct {
	Monitor MonitorResponse `json:"monitor"`
}

// MonitorUpdateResponse represents the v3 API response for updating a monitor.
type MonitorUpdateResponse struct {
	Monitor MonitorResponse `json:"monitor"`
}

// PaginationInfo represents cursor-based pagination in v3 API responses.
type PaginationInfo struct {
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit"`
	Total  int    `json:"total"`
}

// AlertContactResponse represents an alert contact in v3 API responses.
// Note: The v3 API returns type and status as strings, not integers.
type AlertContactResponse struct {
	ID           int    `json:"id"`
	FriendlyName string `json:"friendly_name"`
	Type         string `json:"type"`
	Status       string `json:"status"`
	Value        string `json:"value"`
}

// AlertContactsListResponse represents the v3 API response for listing alert contacts.
type AlertContactsListResponse struct {
	AlertContacts []AlertContactResponse `json:"alert_contacts"`
	Pagination    PaginationInfo         `json:"pagination"`
}

// UserMeResponse represents the v3 API response for /user/me endpoint.
type UserMeResponse struct {
	User UserInfo `json:"user"`
}

// UserInfo represents user account information in v3 API.
type UserInfo struct {
	Email        string `json:"email"`
	MonitorLimit int    `json:"monitor_limit"`
	MonitorUsage int    `json:"monitor_usage"`
	SMSLimit     int    `json:"sms_limit"`
	SMSUsage     int    `json:"sms_usage"`
}

// APIError represents an error response from the v3 API.
type APIError struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}
