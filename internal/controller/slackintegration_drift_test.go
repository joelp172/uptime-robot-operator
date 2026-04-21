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

package controller

import (
	"strings"
	"testing"

	"github.com/joelp172/uptime-robot-operator/internal/uptimerobot"
)

func strPtr(s string) *string { return &s }

func TestSlackIntegrationDriftReason(t *testing.T) {
	t.Parallel()

	slackType := uptimerobot.SlackIntegrationType
	base := uptimerobot.IntegrationResponse{
		ID:                     1,
		Type:                   &slackType,
		FriendlyName:           strPtr("platform-alerts"),
		EnableNotificationsFor: strPtr("Down"),
		SSLExpirationReminder:  true,
		Value:                  "https://hooks.slack.com/services/T000/B000/XXX",
		CustomValue:            "custom",
	}
	desired := uptimerobot.SlackIntegrationData{
		FriendlyName:           "platform-alerts",
		EnableNotificationsFor: "Down",
		SSLExpirationReminder:  true,
		WebhookURL:             "https://hooks.slack.com/services/T000/B000/XXX",
		CustomValue:            "custom",
	}

	t.Run("exact match returns no drift", func(t *testing.T) {
		existing := base
		if got := slackIntegrationDriftReason(&existing, desired); got != "" {
			t.Fatalf("expected no drift, got %q", got)
		}
	})

	t.Run("masked webhook does not trigger drift", func(t *testing.T) {
		existing := base
		existing.Value = "https://hooks.slack.com/services/T000/B000/****"
		if got := slackIntegrationDriftReason(&existing, desired); got != "" {
			t.Fatalf("masked webhook should not be drift, got %q", got)
		}
	})

	t.Run("empty webhook from API does not trigger drift", func(t *testing.T) {
		existing := base
		existing.Value = ""
		if got := slackIntegrationDriftReason(&existing, desired); got != "" {
			t.Fatalf("empty api webhook should not be drift, got %q", got)
		}
	})

	t.Run("unmasked different webhook triggers drift", func(t *testing.T) {
		existing := base
		existing.Value = "https://hooks.slack.com/services/T999/B999/OTHER"
		if got := slackIntegrationDriftReason(&existing, desired); !strings.Contains(got, "webhookURL") {
			t.Fatalf("expected webhookURL drift, got %q", got)
		}
	})

	t.Run("friendly name mismatch reports reason", func(t *testing.T) {
		existing := base
		existing.FriendlyName = strPtr("other-alerts")
		if got := slackIntegrationDriftReason(&existing, desired); !strings.Contains(got, "friendlyName") {
			t.Fatalf("expected friendlyName drift, got %q", got)
		}
	})

	t.Run("customValue mismatch reports reason", func(t *testing.T) {
		existing := base
		existing.CustomValue = "different"
		if got := slackIntegrationDriftReason(&existing, desired); !strings.Contains(got, "customValue") {
			t.Fatalf("expected customValue drift, got %q", got)
		}
	})

	t.Run("wrong type reports reason", func(t *testing.T) {
		existing := base
		other := "MSTeams"
		existing.Type = &other
		if got := slackIntegrationDriftReason(&existing, desired); !strings.Contains(got, "type") {
			t.Fatalf("expected type drift, got %q", got)
		}
	})

	t.Run("nil existing reports reason", func(t *testing.T) {
		if got := slackIntegrationDriftReason(nil, desired); got == "" {
			t.Fatal("expected drift reason for nil existing")
		}
	})
}
