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

// ValidateHTTPSMonitorFields checks common HTTPS monitor fields against the API response.
// Returns a list of error messages for any mismatches; empty means valid.
func ValidateHTTPSMonitorFields(
	expectedName, expectedURL, expectedType string,
	expectedIntervalSec int,
	expectedMethod string,
	actual *uptimerobot.MonitorResponse,
) MonitorFieldErrors {
	debugLog("Validating HTTPS monitor fields: want(name=%q, url=%q, type=%q, interval=%d, method=%q) got(name=%q, url=%q, type=%q, interval=%d, method=%q)",
		expectedName, expectedURL, expectedType, expectedIntervalSec, expectedMethod,
		actual.FriendlyName, actual.URL, actual.Type, actual.Interval, actual.HTTPMethodType)

	var errs MonitorFieldErrors
	if actual.FriendlyName != expectedName {
		errs = append(errs, fmt.Sprintf("friendlyName: got %q want %q", actual.FriendlyName, expectedName))
	}
	if actual.URL != expectedURL {
		errs = append(errs, fmt.Sprintf("url: got %q want %q", actual.URL, expectedURL))
	}
	if actual.Type != expectedType {
		errs = append(errs, fmt.Sprintf("type: got %q want %q", actual.Type, expectedType))
	}
	if actual.Interval != expectedIntervalSec {
		errs = append(errs, fmt.Sprintf("interval: got %d want %d", actual.Interval, expectedIntervalSec))
	}
	if expectedMethod != "" && actual.HTTPMethodType != expectedMethod {
		errs = append(errs, fmt.Sprintf("httpMethodType: got %q want %q", actual.HTTPMethodType, expectedMethod))
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

// ValidateDNSMonitorFields checks DNS config (e.g. A records) in API response.
func ValidateDNSMonitorFields(expectedARecords []string, actual *uptimerobot.MonitorResponse) MonitorFieldErrors {
	debugLog("Validating DNS records: want %d A records", len(expectedARecords))

	var errs MonitorFieldErrors
	if actual.Config == nil || actual.Config.DNSRecords == nil {
		debugLog("DNS config or records missing in response")
		if len(expectedARecords) > 0 {
			errs = append(errs, "config.dnsRecords: missing in response")
		}
		return errs
	}
	actualA := actual.Config.DNSRecords.A
	debugLog("Found %d A records in response", len(actualA))

	if len(actualA) != len(expectedARecords) {
		errs = append(errs, fmt.Sprintf("config.dnsRecords.A length: got %d want %d", len(actualA), len(expectedARecords)))
		return errs
	}
	for i, want := range expectedARecords {
		if i >= len(actualA) || actualA[i] != want {
			errs = append(errs, fmt.Sprintf("config.dnsRecords.A[%d]: got %v want %q", i, actualA, want))
			break
		}
	}

	if len(errs) > 0 {
		debugLog("DNS validation failed")
	} else {
		debugLog("DNS validation passed")
	}

	return errs
}
