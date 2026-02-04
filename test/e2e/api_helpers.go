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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/joelp172/uptime-robot-operator/internal/uptimerobot"
)

// getMonitorFromAPI fetches a monitor directly from the UptimeRobot API by ID.
// Used in e2e tests to verify that the operator has correctly reconciled the CR to the API.
func getMonitorFromAPI(apiKey, monitorID string) (*uptimerobot.MonitorResponse, error) {
	// Set API URL before creating client (NewClient reads UPTIME_ROBOT_API env var).
	// Default to production API v3 unless already overridden in environment.
	if os.Getenv("UPTIME_ROBOT_API") == "" {
		if err := os.Setenv("UPTIME_ROBOT_API", "https://api.uptimerobot.com/v3"); err != nil {
			return nil, fmt.Errorf("failed to set UPTIME_ROBOT_API env var: %w", err)
		}
	}

	client := uptimerobot.NewClient(apiKey)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return client.GetMonitor(ctx, monitorID)
}

// WaitForMonitorDeletedFromAPI polls the UptimeRobot API until the monitor is gone (404).
// Use after deleting a Monitor CR so the next test does not hit account monitor limits.
func WaitForMonitorDeletedFromAPI(apiKey, monitorID string) {
	if apiKey == "" || monitorID == "" {
		return
	}
	By("waiting for monitor to be removed from UptimeRobot API")
	Eventually(func(g Gomega) {
		_, err := getMonitorFromAPI(apiKey, monitorID)
		g.Expect(err).To(HaveOccurred())
		g.Expect(errors.Is(err, uptimerobot.ErrMonitorNotFound)).To(BeTrue())
	}, 90*time.Second, 5*time.Second).Should(Succeed())
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
	return errs
}

// ValidateKeywordMonitorFields checks keyword-specific fields.
func ValidateKeywordMonitorFields(
	expectedKeywordType, expectedKeywordValue string,
	expectedCaseSensitive *bool,
	actual *uptimerobot.MonitorResponse,
) MonitorFieldErrors {
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
	return errs
}

// ValidatePortMonitorFields checks port number in API response.
func ValidatePortMonitorFields(expectedPort int, actual *uptimerobot.MonitorResponse) MonitorFieldErrors {
	var errs MonitorFieldErrors
	if actual.Port == nil {
		errs = append(errs, "port: missing in response")
	} else if *actual.Port != expectedPort {
		errs = append(errs, fmt.Sprintf("port: got %d want %d", *actual.Port, expectedPort))
	}
	return errs
}

// ValidateDNSMonitorFields checks DNS config (e.g. A records) in API response.
func ValidateDNSMonitorFields(expectedARecords []string, actual *uptimerobot.MonitorResponse) MonitorFieldErrors {
	var errs MonitorFieldErrors
	if actual.Config == nil || actual.Config.DNSRecords == nil {
		if len(expectedARecords) > 0 {
			errs = append(errs, "config.dnsRecords: missing in response")
		}
		return errs
	}
	actualA := actual.Config.DNSRecords.A
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
	return errs
}
