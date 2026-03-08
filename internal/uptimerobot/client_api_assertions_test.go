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
	"testing"

	uptimerobotv1 "github.com/joelp172/uptime-robot-operator/api/v1alpha1"
	"github.com/joelp172/uptime-robot-operator/internal/uptimerobot/urtypes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildAPIAssertionsConfig(t *testing.T) {
	t.Run("returns nil when assertions is nil", func(t *testing.T) {
		result := buildAPIAssertionsConfig(nil)
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("returns nil when checks is empty", func(t *testing.T) {
		assertions := &uptimerobotv1.MonitorAPIAssertions{
			Logic:  urtypes.LogicAND,
			Checks: []uptimerobotv1.MonitorAPIAssertion{},
		}
		result := buildAPIAssertionsConfig(assertions)
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("builds config with single assertion", func(t *testing.T) {
		assertions := &uptimerobotv1.MonitorAPIAssertions{
			Logic: urtypes.LogicAND,
			Checks: []uptimerobotv1.MonitorAPIAssertion{
				{
					Property: "$.status",
					Operator: urtypes.AssertionEquals,
					Value:    "healthy",
				},
			},
		}
		result := buildAPIAssertionsConfig(assertions)

		if result == nil {
			t.Fatal("expected non-nil result")
			return
		}
		if result.Logic != "AND" {
			t.Errorf("expected logic to be AND, got %s", result.Logic)
		}
		if len(result.Checks) != 1 {
			t.Fatalf("expected 1 check, got %d", len(result.Checks))
		}
		if result.Checks[0].Property != "$.status" {
			t.Errorf("expected property $.status, got %s", result.Checks[0].Property)
		}
		if result.Checks[0].Comparison != "equals" {
			t.Errorf("expected comparison equals, got %s", result.Checks[0].Comparison)
		}
		if result.Checks[0].Target != "healthy" {
			t.Errorf("expected target healthy, got %v", result.Checks[0].Target)
		}
	})

	t.Run("builds config with multiple assertions and OR logic", func(t *testing.T) {
		assertions := &uptimerobotv1.MonitorAPIAssertions{
			Logic: urtypes.LogicOR,
			Checks: []uptimerobotv1.MonitorAPIAssertion{
				{
					Property: "$.status",
					Operator: urtypes.AssertionEquals,
					Value:    "healthy",
				},
				{
					Property: "$.version",
					Operator: urtypes.AssertionIsNotNull,
					Value:    "",
				},
				{
					Property: "$.latency",
					Operator: urtypes.AssertionLessThan,
					Value:    "1000",
				},
			},
		}
		result := buildAPIAssertionsConfig(assertions)

		if result == nil {
			t.Fatal("expected non-nil result")
			return
		}
		if result.Logic != "OR" {
			t.Errorf("expected logic to be OR, got %s", result.Logic)
		}
		if len(result.Checks) != 3 {
			t.Fatalf("expected 3 checks, got %d", len(result.Checks))
		}

		// Check first assertion
		if result.Checks[0].Property != "$.status" {
			t.Errorf("expected property $.status, got %s", result.Checks[0].Property)
		}
		if result.Checks[0].Comparison != "equals" {
			t.Errorf("expected comparison equals, got %s", result.Checks[0].Comparison)
		}

		// Check second assertion (is_not_null should not have target)
		if result.Checks[1].Property != "$.version" {
			t.Errorf("expected property $.version, got %s", result.Checks[1].Property)
		}
		if result.Checks[1].Comparison != "is_not_null" {
			t.Errorf("expected comparison is_not_null, got %s", result.Checks[1].Comparison)
		}

		// Check third assertion
		if result.Checks[2].Property != "$.latency" {
			t.Errorf("expected property $.latency, got %s", result.Checks[2].Property)
		}
		if result.Checks[2].Comparison != "less_than" {
			t.Errorf("expected comparison less_than, got %s", result.Checks[2].Comparison)
		}
		if target, ok := result.Checks[2].Target.(int); !ok || target != 1000 {
			t.Errorf("expected numeric target 1000, got %#v", result.Checks[2].Target)
		}
	})

	t.Run("handles all assertion operators", func(t *testing.T) {
		operators := []struct {
			op       urtypes.AssertionOperator
			expected string
		}{
			{urtypes.AssertionEquals, "equals"},
			{urtypes.AssertionNotEquals, "not_equals"},
			{urtypes.AssertionContains, "contains"},
			{urtypes.AssertionNotContains, "not_contains"},
			{urtypes.AssertionGreaterThan, "greater_than"},
			{urtypes.AssertionLessThan, "less_than"},
			{urtypes.AssertionIsNull, "is_null"},
			{urtypes.AssertionIsNotNull, "is_not_null"},
		}

		for _, tc := range operators {
			assertions := &uptimerobotv1.MonitorAPIAssertions{
				Logic: urtypes.LogicAND,
				Checks: []uptimerobotv1.MonitorAPIAssertion{
					{
						Property: "$.test",
						Operator: tc.op,
						Value:    "test-value",
					},
				},
			}
			result := buildAPIAssertionsConfig(assertions)

			if result == nil {
				t.Fatalf("expected non-nil result for operator %v", tc.op)
				return
			}
			if result.Checks[0].Comparison != tc.expected {
				t.Errorf("expected comparison %s, got %s", tc.expected, result.Checks[0].Comparison)
			}
		}
	})

	t.Run("keeps equals target values as strings", func(t *testing.T) {
		assertions := &uptimerobotv1.MonitorAPIAssertions{
			Logic: urtypes.LogicAND,
			Checks: []uptimerobotv1.MonitorAPIAssertion{
				{
					Property: "$.enabled",
					Operator: urtypes.AssertionEquals,
					Value:    "true",
				},
			},
		}
		result := buildAPIAssertionsConfig(assertions)
		if result == nil {
			t.Fatal("expected non-nil result")
			return
		}
		if target, ok := result.Checks[0].Target.(string); !ok || target != "true" {
			t.Fatalf("expected string target \"true\", got %#v", result.Checks[0].Target)
		}
	})
}

func TestBuildCreateMonitorRequest_APIAssertions(t *testing.T) {
	client := NewClient("test-api-key")

	interval := metav1.Duration{Duration: 300000000000}   // 5m
	timeout := metav1.Duration{Duration: 30000000000}     // 30s
	gracePeriod := metav1.Duration{Duration: 60000000000} // 60s

	t.Run("includes API assertions in config when provided", func(t *testing.T) {
		monitor := uptimerobotv1.MonitorValues{
			Name:        "Test Monitor",
			URL:         "https://api.example.com/health",
			Interval:    &interval,
			Timeout:     &timeout,
			GracePeriod: &gracePeriod,
			APIAssertions: &uptimerobotv1.MonitorAPIAssertions{
				Logic: urtypes.LogicAND,
				Checks: []uptimerobotv1.MonitorAPIAssertion{
					{
						Property: "$.status",
						Operator: urtypes.AssertionEquals,
						Value:    "healthy",
					},
				},
			},
		}

		req := client.buildCreateMonitorRequest(monitor, nil)

		if req.Type != "API" {
			t.Fatalf("expected monitor type API when apiAssertions are configured, got %s", req.Type)
		}
		if req.Config == nil {
			t.Fatal("expected Config to be non-nil")
		}
		if req.Config.APIAssertions == nil {
			t.Fatal("expected APIAssertions to be non-nil")
		}
		if req.Config.APIAssertions.Logic != "AND" {
			t.Errorf("expected logic AND, got %s", req.Config.APIAssertions.Logic)
		}
		if len(req.Config.APIAssertions.Checks) != 1 {
			t.Errorf("expected 1 check, got %d", len(req.Config.APIAssertions.Checks))
		}
	})

	t.Run("omits API assertions when not provided", func(t *testing.T) {
		monitor := uptimerobotv1.MonitorValues{
			Name:        "Test Monitor",
			URL:         "https://example.com",
			Type:        urtypes.TypeHTTPS,
			Interval:    &interval,
			Timeout:     &timeout,
			GracePeriod: &gracePeriod,
		}

		req := client.buildCreateMonitorRequest(monitor, nil)
		if req.Type != "HTTP" {
			t.Fatalf("expected monitor type HTTP when apiAssertions are omitted, got %s", req.Type)
		}

		// Config may be nil or empty depending on monitor type
		if req.Config != nil && req.Config.APIAssertions != nil {
			t.Errorf("expected APIAssertions to be nil")
		}
	})

	t.Run("prefers API assertions config over DNS config when assertions are present", func(t *testing.T) {
		monitor := uptimerobotv1.MonitorValues{
			Name:        "Test Monitor",
			URL:         "8.8.8.8",
			Type:        urtypes.TypeDNS,
			Interval:    &interval,
			Timeout:     &timeout,
			GracePeriod: &gracePeriod,
			DNS: &uptimerobotv1.MonitorDNS{
				A: []string{"8.8.8.8"},
			},
			APIAssertions: &uptimerobotv1.MonitorAPIAssertions{
				Logic: urtypes.LogicAND,
				Checks: []uptimerobotv1.MonitorAPIAssertion{
					{
						Property: "$.status",
						Operator: urtypes.AssertionEquals,
						Value:    "ok",
					},
				},
			},
		}

		req := client.buildCreateMonitorRequest(monitor, nil)
		if req.Type != "API" {
			t.Fatalf("expected monitor type API when apiAssertions are configured, got %s", req.Type)
		}

		if req.Config == nil {
			t.Fatal("expected Config to be non-nil")
		}
		if req.Config.DNSRecords != nil {
			t.Error("expected DNSRecords to be nil when apiAssertions force API monitor type")
		}
		if req.Config.APIAssertions == nil {
			t.Error("expected APIAssertions to be non-nil")
		}
	})
}

func TestBuildUpdateMonitorRequest_APIAssertions(t *testing.T) {
	client := NewClient("test-api-key")

	interval := metav1.Duration{Duration: 300000000000}   // 5m
	timeout := metav1.Duration{Duration: 30000000000}     // 30s
	gracePeriod := metav1.Duration{Duration: 60000000000} // 60s

	t.Run("includes API assertions in config when provided", func(t *testing.T) {
		monitor := uptimerobotv1.MonitorValues{
			Name:        "Test Monitor",
			URL:         "https://api.example.com/health",
			Interval:    &interval,
			Timeout:     &timeout,
			GracePeriod: &gracePeriod,
			APIAssertions: &uptimerobotv1.MonitorAPIAssertions{
				Logic: urtypes.LogicOR,
				Checks: []uptimerobotv1.MonitorAPIAssertion{
					{
						Property: "$.status",
						Operator: urtypes.AssertionEquals,
						Value:    "healthy",
					},
					{
						Property: "$.status",
						Operator: urtypes.AssertionEquals,
						Value:    "degraded",
					},
				},
			},
		}

		req := client.buildUpdateMonitorRequest(monitor, nil)

		if req.Config == nil {
			t.Fatal("expected Config to be non-nil")
		}
		if req.Config.APIAssertions == nil {
			t.Fatal("expected APIAssertions to be non-nil")
		}
		if req.Config.APIAssertions.Logic != "OR" {
			t.Errorf("expected logic OR, got %s", req.Config.APIAssertions.Logic)
		}
		if len(req.Config.APIAssertions.Checks) != 2 {
			t.Errorf("expected 2 checks, got %d", len(req.Config.APIAssertions.Checks))
		}
	})

	t.Run("omits API assertions when not provided", func(t *testing.T) {
		monitor := uptimerobotv1.MonitorValues{
			Name:        "Test Monitor",
			URL:         "https://example.com",
			Interval:    &interval,
			Timeout:     &timeout,
			GracePeriod: &gracePeriod,
		}

		req := client.buildUpdateMonitorRequest(monitor, nil)

		// Config may be nil or empty depending on monitor type
		if req.Config != nil && req.Config.APIAssertions != nil {
			t.Errorf("expected APIAssertions to be nil")
		}
	})

	t.Run("includes URL for DNS spec when apiAssertions force API type", func(t *testing.T) {
		monitor := uptimerobotv1.MonitorValues{
			Name:        "Test Monitor",
			URL:         "https://api.example.com/health",
			Type:        urtypes.TypeDNS,
			Interval:    &interval,
			Timeout:     &timeout,
			GracePeriod: &gracePeriod,
			APIAssertions: &uptimerobotv1.MonitorAPIAssertions{
				Logic: urtypes.LogicAND,
				Checks: []uptimerobotv1.MonitorAPIAssertion{
					{
						Property: "$.status",
						Operator: urtypes.AssertionEquals,
						Value:    "ok",
					},
				},
			},
		}

		req := client.buildUpdateMonitorRequest(monitor, nil)
		if req.URL != "https://api.example.com/health" {
			t.Fatalf("expected URL to be included for API monitor update, got %q", req.URL)
		}
	})
}
