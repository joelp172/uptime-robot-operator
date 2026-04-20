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
	"testing"

	"github.com/joelp172/uptime-robot-operator/internal/uptimerobot"
)

func TestNormalizeSlackIntegrationData_TrimsStringFields(t *testing.T) {
	t.Parallel()

	got := normalizeSlackIntegrationData(uptimerobot.SlackIntegrationData{
		FriendlyName:           "  My Slack \n",
		EnableNotificationsFor: " Down ",
		WebhookURL:             "https://hooks.slack.com/services/AAA/BBB/CCC\n",
		CustomValue:            "\tcustom\n",
	})

	if got.FriendlyName != "My Slack" {
		t.Errorf("FriendlyName: got %q want %q", got.FriendlyName, "My Slack")
	}
	if got.EnableNotificationsFor != "Down" {
		t.Errorf("EnableNotificationsFor: got %q want %q", got.EnableNotificationsFor, "Down")
	}
	if got.WebhookURL != "https://hooks.slack.com/services/AAA/BBB/CCC" {
		t.Errorf("WebhookURL: got %q want %q", got.WebhookURL, "https://hooks.slack.com/services/AAA/BBB/CCC")
	}
	if got.CustomValue != "custom" {
		t.Errorf("CustomValue: got %q want %q", got.CustomValue, "custom")
	}
}

func TestNormalizeSlackIntegrationData_DefaultsEnableNotificationsFor(t *testing.T) {
	t.Parallel()

	got := normalizeSlackIntegrationData(uptimerobot.SlackIntegrationData{
		EnableNotificationsFor: "   ",
	})
	if got.EnableNotificationsFor != "UpAndDown" {
		t.Errorf("EnableNotificationsFor default: got %q want %q", got.EnableNotificationsFor, "UpAndDown")
	}
}
