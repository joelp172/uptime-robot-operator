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

// BridgedIntegrationTypes lists the UptimeRobot /integrations types that the
// operator bridges into Contact/Monitor alert routing: they are surfaced in
// Account.status.alertContacts and resolvable via Contact friendly-name
// lookup (FindContactID).
//
// To extend the bridge to a new integration kind (e.g. MSTeams, Webhook,
// Discord, PagerDuty), append its API-side type string here. No other
// changes are required for the lookup/status-population paths — they are
// driven entirely by this list.
//
// Note: this is intentionally separate from SlackIntegrationType, which
// gates Slack-specific behaviour in the SlackIntegration controller
// (adoption on 409, drift detection, etc.) and is not a bridge concern.
var BridgedIntegrationTypes = []string{
	SlackIntegrationType,
	// "MSTeams",  // future
	// "Webhook",  // future
	// "Discord",  // future
}

// IsBridgedIntegrationType reports whether the given UptimeRobot integration
// type is surfaced through the Contact/Monitor bridge.
func IsBridgedIntegrationType(t string) bool {
	for _, bridged := range BridgedIntegrationTypes {
		if bridged == t {
			return true
		}
	}
	return false
}
