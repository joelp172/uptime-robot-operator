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

package e2e

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/joelp172/uptime-robot-operator/internal/uptimerobot"
)

// debugEnabled returns true if E2E_DEBUG environment variable is set to a truthy value.
func debugEnabled() bool {
	val := strings.ToLower(os.Getenv("E2E_DEBUG"))
	return val == "1" || val == "true" || val == "yes"
}

// debugLog prints a debug message if E2E_DEBUG is enabled.
func debugLog(format string, args ...interface{}) {
	if debugEnabled() {
		fmt.Fprintf(GinkgoWriter, "[DEBUG] "+format+"\n", args...)
	}
}

// getMonitorFromAPI fetches a monitor directly from the UptimeRobot API by ID.
// Used in e2e tests to verify that the operator has correctly reconciled the CR to the API.
func getMonitorFromAPI(apiKey, monitorID string) (*uptimerobot.MonitorResponse, error) {
	debugLog("Calling GetMonitor for ID=%s", monitorID)

	// Set API URL before creating client (NewClient reads UPTIME_ROBOT_API env var).
	// Default to production API v3 unless already overridden in environment.
	apiURL := os.Getenv("UPTIME_ROBOT_API")
	if apiURL == "" {
		apiURL = "https://api.uptimerobot.com/v3"
		if err := os.Setenv("UPTIME_ROBOT_API", apiURL); err != nil {
			return nil, fmt.Errorf("failed to set UPTIME_ROBOT_API env var: %w", err)
		}
	}
	debugLog("Using API endpoint: %s", apiURL)

	client := uptimerobot.NewClient(apiKey)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	monitor, err := client.GetMonitor(ctx, monitorID)
	if err != nil {
		debugLog("GetMonitor failed: %v", err)
		return nil, err
	}

	debugLog("GetMonitor succeeded: Name=%s, URL=%s, Type=%s, Interval=%d, Method=%s",
		monitor.FriendlyName, monitor.URL, monitor.Type, monitor.Interval, monitor.HTTPMethodType)
	if len(monitor.AssignedAlertContacts) > 0 {
		debugLog("Contacts=%d, First contact alertContactId=%v (type=%T)",
			len(monitor.AssignedAlertContacts),
			monitor.AssignedAlertContacts[0].AlertContactID,
			monitor.AssignedAlertContacts[0].AlertContactID)
	}

	return monitor, nil
}

// WaitForMonitorDeletedFromAPI polls the UptimeRobot API until the monitor is gone (404).
// Use after deleting a Monitor CR so the next test does not hit account monitor limits.
func WaitForMonitorDeletedFromAPI(apiKey, monitorID string) {
	if apiKey == "" || monitorID == "" {
		debugLog("Skipping API deletion wait: apiKey or monitorID is empty")
		return
	}
	By("waiting for monitor to be removed from UptimeRobot API")
	debugLog("Polling for monitor deletion from API: ID=%s", monitorID)
	Eventually(func(g Gomega) {
		_, err := getMonitorFromAPI(apiKey, monitorID)
		g.Expect(err).To(HaveOccurred())
		g.Expect(errors.Is(err, uptimerobot.ErrMonitorNotFound)).To(BeTrue())
	}, 90*time.Second, 5*time.Second).Should(Succeed())
	debugLog("Monitor successfully deleted from API: ID=%s", monitorID)
}

// MonitorFieldErrors collects validation errors for API response vs expected spec.
type MonitorFieldErrors []string

func (e MonitorFieldErrors) Error() string {
	if len(e) == 0 {
		return ""
	}
	return fmt.Sprintf("%d field error(s): %v", len(e), []string(e))
}

// HTTPSMonitorExpectation contains expected fields for HTTPS/HTTP monitor validation.
type HTTPSMonitorExpectation struct {
	Name                     string
	URL                      string
	Type                     string
	IntervalSec              int
	Method                   string
	Timeout                  *int // in seconds, nil means don't check
	GracePeriod              *int // in seconds, nil means don't check
	ResponseTimeThreshold    *int // in ms, nil means don't check
	CheckSSLErrors           *bool
	FollowRedirections       *bool
	SSLExpirationReminder    *bool
	DomainExpirationReminder *bool
	SuccessHTTPResponseCodes []string // e.g. ["2xx", "3xx"]
	Tags                     []string
	CustomHTTPHeaders        map[string]string
}

// ValidateHTTPSMonitorFields checks common HTTPS monitor fields against the API response.
// Returns a list of error messages for any mismatches; empty means valid.
func ValidateHTTPSMonitorFields(
	expected HTTPSMonitorExpectation,
	actual *uptimerobot.MonitorResponse,
) MonitorFieldErrors {
	debugLog("Validating HTTPS monitor fields: want(name=%q, url=%q, type=%q, interval=%d, method=%q)",
		expected.Name, expected.URL, expected.Type, expected.IntervalSec, expected.Method)

	var errs MonitorFieldErrors
	if actual.FriendlyName != expected.Name {
		errs = append(errs, fmt.Sprintf("friendlyName: got %q want %q", actual.FriendlyName, expected.Name))
	}
	if actual.URL != expected.URL {
		errs = append(errs, fmt.Sprintf("url: got %q want %q", actual.URL, expected.URL))
	}
	if actual.Type != expected.Type {
		errs = append(errs, fmt.Sprintf("type: got %q want %q", actual.Type, expected.Type))
	}
	if actual.Interval != expected.IntervalSec {
		errs = append(errs, fmt.Sprintf("interval: got %d want %d", actual.Interval, expected.IntervalSec))
	}
	if expected.Method != "" && actual.HTTPMethodType != expected.Method {
		errs = append(errs, fmt.Sprintf("httpMethodType: got %q want %q", actual.HTTPMethodType, expected.Method))
	}

	// Check timeout
	if expected.Timeout != nil {
		if actual.Timeout == nil {
			errs = append(errs, fmt.Sprintf("timeout: got nil want %d", *expected.Timeout))
		} else if *actual.Timeout != *expected.Timeout {
			errs = append(errs, fmt.Sprintf("timeout: got %d want %d", *actual.Timeout, *expected.Timeout))
		}
	}

	// Check gracePeriod
	if expected.GracePeriod != nil {
		if actual.GracePeriod == nil {
			errs = append(errs, fmt.Sprintf("gracePeriod: got nil want %d", *expected.GracePeriod))
		} else if *actual.GracePeriod != *expected.GracePeriod {
			errs = append(errs, fmt.Sprintf("gracePeriod: got %d want %d", *actual.GracePeriod, *expected.GracePeriod))
		}
	}

	// Check responseTimeThreshold
	if expected.ResponseTimeThreshold != nil {
		if actual.ResponseTimeThreshold == nil {
			errs = append(errs, fmt.Sprintf("responseTimeThreshold: got nil want %d", *expected.ResponseTimeThreshold))
		} else if *actual.ResponseTimeThreshold != *expected.ResponseTimeThreshold {
			errs = append(errs, fmt.Sprintf("responseTimeThreshold: got %d want %d", *actual.ResponseTimeThreshold, *expected.ResponseTimeThreshold))
		}
	}

	// Check SSL/Domain options
	if expected.CheckSSLErrors != nil {
		if actual.CheckSSLErrors == nil {
			errs = append(errs, fmt.Sprintf("checkSSLErrors: got nil want %v", *expected.CheckSSLErrors))
		} else if *actual.CheckSSLErrors != *expected.CheckSSLErrors {
			errs = append(errs, fmt.Sprintf("checkSSLErrors: got %v want %v", *actual.CheckSSLErrors, *expected.CheckSSLErrors))
		}
	}

	if expected.FollowRedirections != nil {
		if actual.FollowRedirections == nil {
			errs = append(errs, fmt.Sprintf("followRedirections: got nil want %v", *expected.FollowRedirections))
		} else if *actual.FollowRedirections != *expected.FollowRedirections {
			errs = append(errs, fmt.Sprintf("followRedirections: got %v want %v", *actual.FollowRedirections, *expected.FollowRedirections))
		}
	}

	if expected.SSLExpirationReminder != nil {
		if actual.SSLExpirationReminder == nil {
			errs = append(errs, fmt.Sprintf("sslExpirationReminder: got nil want %v", *expected.SSLExpirationReminder))
		} else if *actual.SSLExpirationReminder != *expected.SSLExpirationReminder {
			errs = append(errs, fmt.Sprintf("sslExpirationReminder: got %v want %v", *actual.SSLExpirationReminder, *expected.SSLExpirationReminder))
		}
	}

	if expected.DomainExpirationReminder != nil {
		if actual.DomainExpirationReminder == nil {
			errs = append(errs, fmt.Sprintf("domainExpirationReminder: got nil want %v", *expected.DomainExpirationReminder))
		} else if *actual.DomainExpirationReminder != *expected.DomainExpirationReminder {
			errs = append(errs, fmt.Sprintf("domainExpirationReminder: got %v want %v", *actual.DomainExpirationReminder, *expected.DomainExpirationReminder))
		}
	}

	// Check successHttpResponseCodes
	if len(expected.SuccessHTTPResponseCodes) > 0 {
		if len(actual.SuccessHTTPResponseCodes) != len(expected.SuccessHTTPResponseCodes) {
			errs = append(errs, fmt.Sprintf("successHttpResponseCodes length: got %d want %d", len(actual.SuccessHTTPResponseCodes), len(expected.SuccessHTTPResponseCodes)))
		} else {
			for i, want := range expected.SuccessHTTPResponseCodes {
				if actual.SuccessHTTPResponseCodes[i] != want {
					errs = append(errs, fmt.Sprintf("successHttpResponseCodes[%d]: got %q want %q", i, actual.SuccessHTTPResponseCodes[i], want))
				}
			}
		}
	}

	// Check tags
	if len(expected.Tags) > 0 {
		if len(actual.Tags) != len(expected.Tags) {
			errs = append(errs, fmt.Sprintf("tags length: got %d want %d", len(actual.Tags), len(expected.Tags)))
		} else {
			// Tags may be returned in different order, so check if all expected tags are present
			actualTagsMap := make(map[string]bool)
			for _, tag := range actual.Tags {
				actualTagsMap[tag.Name] = true
			}
			for _, want := range expected.Tags {
				if !actualTagsMap[want] {
					errs = append(errs, fmt.Sprintf("tags: missing %q", want))
				}
			}
		}
	}

	// Check custom HTTP headers
	if len(expected.CustomHTTPHeaders) > 0 {
		for key, wantVal := range expected.CustomHTTPHeaders {
			if actualVal, ok := actual.CustomHTTPHeaders[key]; !ok {
				errs = append(errs, fmt.Sprintf("customHttpHeaders[%q]: missing", key))
			} else if actualVal != wantVal {
				errs = append(errs, fmt.Sprintf("customHttpHeaders[%q]: got %q want %q", key, actualVal, wantVal))
			}
		}
	}

	if len(errs) > 0 {
		debugLog("Validation failed with %d error(s)", len(errs))
	} else {
		debugLog("Validation passed")
	}

	return errs
}

// ValidateKeywordMonitorFields checks keyword-specific fields.
func ValidateKeywordMonitorFields(
	expectedKeywordType, expectedKeywordValue string,
	expectedCaseSensitive *bool,
	actual *uptimerobot.MonitorResponse,
) MonitorFieldErrors {
	debugLog("Validating keyword fields: want(type=%q, value=%q) got(type=%q, value=%q)",
		expectedKeywordType, expectedKeywordValue, actual.KeywordType, actual.KeywordValue)

	var errs MonitorFieldErrors
	if actual.KeywordType != expectedKeywordType {
		errs = append(errs, fmt.Sprintf("keywordType: got %q want %q", actual.KeywordType, expectedKeywordType))
	}
	if actual.KeywordValue != expectedKeywordValue {
		errs = append(errs, fmt.Sprintf("keywordValue: got %q want %q", actual.KeywordValue, expectedKeywordValue))
	}
	if expectedCaseSensitive != nil && actual.KeywordCaseType != nil {
		want := 0
		if *expectedCaseSensitive {
			want = 1
		}
		if *actual.KeywordCaseType != want {
			errs = append(errs, fmt.Sprintf("keywordCaseType: got %d want %d", *actual.KeywordCaseType, want))
		}
	}

	if len(errs) > 0 {
		debugLog("Keyword validation failed with %d error(s)", len(errs))
	} else {
		debugLog("Keyword validation passed")
	}

	return errs
}

// ValidatePortMonitorFields checks port number in API response.
func ValidatePortMonitorFields(expectedPort int, actual *uptimerobot.MonitorResponse) MonitorFieldErrors {
	actualPort := "nil"
	if actual.Port != nil {
		actualPort = fmt.Sprintf("%d", *actual.Port)
	}
	debugLog("Validating port: want(%d) got(%s)", expectedPort, actualPort)

	var errs MonitorFieldErrors
	if actual.Port == nil {
		errs = append(errs, "port: missing in response")
	} else if *actual.Port != expectedPort {
		errs = append(errs, fmt.Sprintf("port: got %d want %d", *actual.Port, expectedPort))
	}

	if len(errs) > 0 {
		debugLog("Port validation failed")
	} else {
		debugLog("Port validation passed")
	}

	return errs
}

// DNSRecordExpectation contains expected DNS records for validation.
type DNSRecordExpectation struct {
	A     []string
	AAAA  []string
	CNAME []string
	MX    []string
	NS    []string
	TXT   []string
}

// ValidateDNSMonitorFields checks DNS config records in API response.
func ValidateDNSMonitorFields(expected DNSRecordExpectation, actual *uptimerobot.MonitorResponse) MonitorFieldErrors {
	debugLog("Validating DNS records: A=%d, AAAA=%d, CNAME=%d, MX=%d, NS=%d, TXT=%d",
		len(expected.A), len(expected.AAAA), len(expected.CNAME), len(expected.MX), len(expected.NS), len(expected.TXT))

	var errs MonitorFieldErrors
	if actual.Config == nil || actual.Config.DNSRecords == nil {
		debugLog("DNS config or records missing in response")
		if len(expected.A) > 0 || len(expected.AAAA) > 0 || len(expected.CNAME) > 0 ||
			len(expected.MX) > 0 || len(expected.NS) > 0 || len(expected.TXT) > 0 {
			errs = append(errs, "config.dnsRecords: missing in response")
		}
		return errs
	}

	// Validate A records
	if len(expected.A) > 0 {
		actualA := actual.Config.DNSRecords.A
		debugLog("Found %d A records in response", len(actualA))
		if len(actualA) != len(expected.A) {
			errs = append(errs, fmt.Sprintf("config.dnsRecords.A length: got %d want %d", len(actualA), len(expected.A)))
		} else {
			for i, want := range expected.A {
				if i >= len(actualA) || actualA[i] != want {
					errs = append(errs, fmt.Sprintf("config.dnsRecords.A[%d]: got %q want %q", i, actualA[i], want))
				}
			}
		}
	}

	// Validate AAAA records
	if len(expected.AAAA) > 0 {
		actualAAAA := actual.Config.DNSRecords.AAAA
		debugLog("Found %d AAAA records in response", len(actualAAAA))
		if len(actualAAAA) != len(expected.AAAA) {
			errs = append(errs, fmt.Sprintf("config.dnsRecords.AAAA length: got %d want %d", len(actualAAAA), len(expected.AAAA)))
		} else {
			for i, want := range expected.AAAA {
				if i >= len(actualAAAA) || actualAAAA[i] != want {
					errs = append(errs, fmt.Sprintf("config.dnsRecords.AAAA[%d]: got %q want %q", i, actualAAAA[i], want))
				}
			}
		}
	}

	// Validate CNAME records
	if len(expected.CNAME) > 0 {
		actualCNAME := actual.Config.DNSRecords.CNAME
		debugLog("Found %d CNAME records in response", len(actualCNAME))
		if len(actualCNAME) != len(expected.CNAME) {
			errs = append(errs, fmt.Sprintf("config.dnsRecords.CNAME length: got %d want %d", len(actualCNAME), len(expected.CNAME)))
		} else {
			for i, want := range expected.CNAME {
				if i >= len(actualCNAME) || actualCNAME[i] != want {
					errs = append(errs, fmt.Sprintf("config.dnsRecords.CNAME[%d]: got %q want %q", i, actualCNAME[i], want))
				}
			}
		}
	}

	// Validate MX records
	if len(expected.MX) > 0 {
		actualMX := actual.Config.DNSRecords.MX
		debugLog("Found %d MX records in response", len(actualMX))
		if len(actualMX) != len(expected.MX) {
			errs = append(errs, fmt.Sprintf("config.dnsRecords.MX length: got %d want %d", len(actualMX), len(expected.MX)))
		} else {
			for i, want := range expected.MX {
				if i >= len(actualMX) || actualMX[i] != want {
					errs = append(errs, fmt.Sprintf("config.dnsRecords.MX[%d]: got %q want %q", i, actualMX[i], want))
				}
			}
		}
	}

	// Validate NS records
	if len(expected.NS) > 0 {
		actualNS := actual.Config.DNSRecords.NS
		debugLog("Found %d NS records in response", len(actualNS))
		if len(actualNS) != len(expected.NS) {
			errs = append(errs, fmt.Sprintf("config.dnsRecords.NS length: got %d want %d", len(actualNS), len(expected.NS)))
		} else {
			for i, want := range expected.NS {
				if i >= len(actualNS) || actualNS[i] != want {
					errs = append(errs, fmt.Sprintf("config.dnsRecords.NS[%d]: got %q want %q", i, actualNS[i], want))
				}
			}
		}
	}

	// Validate TXT records
	if len(expected.TXT) > 0 {
		actualTXT := actual.Config.DNSRecords.TXT
		debugLog("Found %d TXT records in response", len(actualTXT))
		if len(actualTXT) != len(expected.TXT) {
			errs = append(errs, fmt.Sprintf("config.dnsRecords.TXT length: got %d want %d", len(actualTXT), len(expected.TXT)))
		} else {
			for i, want := range expected.TXT {
				if i >= len(actualTXT) || actualTXT[i] != want {
					errs = append(errs, fmt.Sprintf("config.dnsRecords.TXT[%d]: got %q want %q", i, actualTXT[i], want))
				}
			}
		}
	}

	if len(errs) > 0 {
		debugLog("DNS validation failed")
	} else {
		debugLog("DNS validation passed")
	}

	return errs
}

// getMaintenanceWindowFromAPI fetches a maintenance window directly from the UptimeRobot API by ID.
// Used in e2e tests to verify that the operator has correctly reconciled the CR to the API.
func getMaintenanceWindowFromAPI(apiKey, mwID string) (*uptimerobot.MaintenanceWindowResponse, error) {
	debugLog("Calling GetMaintenanceWindow for ID=%s", mwID)

	// Set API URL before creating client (NewClient reads UPTIME_ROBOT_API env var).
	apiURL := os.Getenv("UPTIME_ROBOT_API")
	if apiURL == "" {
		apiURL = "https://api.uptimerobot.com/v3"
		if err := os.Setenv("UPTIME_ROBOT_API", apiURL); err != nil {
			return nil, fmt.Errorf("failed to set UPTIME_ROBOT_API env var: %w", err)
		}
	}
	debugLog("Using API endpoint: %s", apiURL)

	client := uptimerobot.NewClient(apiKey)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mw, err := client.GetMaintenanceWindow(ctx, mwID)
	if err != nil {
		debugLog("GetMaintenanceWindow failed: %v", err)
		return nil, err
	}

	debugLog("GetMaintenanceWindow succeeded: Name=%s, Interval=%s, Duration=%d, MonitorCount=%d",
		mw.Name, mw.Interval, mw.Duration, len(mw.MonitorIDs))

	return &mw, nil
}

// WaitForMaintenanceWindowDeletedFromAPI polls the UptimeRobot API until the maintenance window is gone (404).
// Use after deleting a MaintenanceWindow CR to verify API cleanup.
func WaitForMaintenanceWindowDeletedFromAPI(apiKey, mwID string) {
	if apiKey == "" || mwID == "" {
		debugLog("Skipping MW API deletion wait: apiKey or mwID is empty")
		return
	}
	By("waiting for maintenance window to be removed from UptimeRobot API")
	debugLog("Polling for maintenance window deletion from API: ID=%s", mwID)
	Eventually(func(g Gomega) {
		_, err := getMaintenanceWindowFromAPI(apiKey, mwID)
		g.Expect(err).To(HaveOccurred())
		g.Expect(uptimerobot.IsNotFound(err)).To(BeTrue())
	}, 90*time.Second, 5*time.Second).Should(Succeed())
	debugLog("Maintenance window successfully deleted from API: ID=%s", mwID)
}

// MaintenanceWindowFieldErrors collects validation errors for API response vs expected spec.
type MaintenanceWindowFieldErrors []string

func (e MaintenanceWindowFieldErrors) Error() string {
	if len(e) == 0 {
		return ""
	}
	return fmt.Sprintf("%d field error(s): %v", len(e), []string(e))
}

// ValidateMaintenanceWindowFields checks maintenance window fields against the API response.
// Returns a list of error messages for any mismatches; empty means valid.
func ValidateMaintenanceWindowFields(
	expectedName, expectedInterval, expectedTime string,
	expectedDurationMin int,
	expectedDays []int,
	expectedStartDate string, // Only for "once" interval
	actual *uptimerobot.MaintenanceWindowResponse,
) MaintenanceWindowFieldErrors {
	debugLog("Validating MW fields: want(name=%q, interval=%q, time=%q, duration=%d, days=%v, startDate=%q)",
		expectedName, expectedInterval, expectedTime, expectedDurationMin, expectedDays, expectedStartDate)
	debugLog("  got(name=%q, interval=%q, time=%q, duration=%d, days=%v, date=%q)",
		actual.Name, actual.Interval, actual.Time, actual.Duration, actual.Days, actual.Date)

	var errs MaintenanceWindowFieldErrors
	if actual.Name != expectedName {
		errs = append(errs, fmt.Sprintf("name: got %q want %q", actual.Name, expectedName))
	}
	if actual.Interval != expectedInterval {
		errs = append(errs, fmt.Sprintf("interval: got %q want %q", actual.Interval, expectedInterval))
	}
	if actual.Time != expectedTime {
		errs = append(errs, fmt.Sprintf("time: got %q want %q", actual.Time, expectedTime))
	}
	if actual.Duration != expectedDurationMin {
		errs = append(errs, fmt.Sprintf("duration: got %d want %d", actual.Duration, expectedDurationMin))
	}

	// Validate days for weekly/monthly intervals
	// Compare as sets since API may return days in different order
	if len(expectedDays) > 0 {
		if len(actual.Days) != len(expectedDays) {
			errs = append(errs, fmt.Sprintf("days length: got %d want %d", len(actual.Days), len(expectedDays)))
		} else {
			// Create maps to compare as sets
			expectedSet := make(map[int]bool)
			for _, day := range expectedDays {
				expectedSet[day] = true
			}
			actualSet := make(map[int]bool)
			for _, day := range actual.Days {
				actualSet[day] = true
			}

			// Check if all expected days are present
			for day := range expectedSet {
				if !actualSet[day] {
					errs = append(errs, fmt.Sprintf("days: missing expected day %d (got %v, want %v)", day, actual.Days, expectedDays))
					break
				}
			}
			// Check if there are any unexpected days
			for day := range actualSet {
				if !expectedSet[day] {
					errs = append(errs, fmt.Sprintf("days: unexpected day %d (got %v, want %v)", day, actual.Days, expectedDays))
					break
				}
			}
		}
	}

	// Validate startDate for "once" interval
	if expectedInterval == "once" && expectedStartDate != "" {
		if actual.Date != expectedStartDate {
			errs = append(errs, fmt.Sprintf("date: got %q want %q", actual.Date, expectedStartDate))
		}
	}

	if len(errs) > 0 {
		debugLog("MW validation failed with %d error(s)", len(errs))
	} else {
		debugLog("MW validation passed")
	}

	return errs
}

// monitorHasMaintenanceWindow checks if a monitor's maintenanceWindows list
// contains a maintenance window with the given ID (as string).
func monitorHasMaintenanceWindow(mws []uptimerobot.MaintenanceWindowSummary, mwID string) bool {
	for _, mw := range mws {
		if fmt.Sprintf("%d", mw.ID) == mwID {
			return true
		}
	}
	return false
}

// maintenanceWindowIDs extracts the IDs from a list of maintenance window summaries.
// Used for debug logging.
func maintenanceWindowIDs(mws []uptimerobot.MaintenanceWindowSummary) []int {
	ids := make([]int, len(mws))
	for i, mw := range mws {
		ids[i] = mw.ID
	}
	return ids
}
