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
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/joelp172/uptime-robot-operator/test/utils"
)

// alertContactIDToString renders the interface{}-typed AlertContactID from
// the UptimeRobot monitor response (number or string depending on endpoint)
// as a plain decimal string. A generic fmt.Sprintf("%v", ...) on a float64
// collapses large IDs to scientific notation (e.g. "8.34704e+06"), which
// never matches the integration ID format the operator uses.
func alertContactIDToString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatInt(int64(t), 10)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// Covers the bridge between SlackIntegration (/integrations) and Contact/Monitor
// alert routing (/user/alert-contacts). See issue #185 for the scenario these
// tests exercise.
var _ = Describe("SlackIntegration Contact bridge", Ordered, Label("slackcontact"), func() {
	var (
		webhookSecretName = fmt.Sprintf("e2e-slackbridge-webhook-%s", testRunID)
		integrationName   = fmt.Sprintf("e2e-slackbridge-integration-%s", testRunID)
		contactName       = fmt.Sprintf("e2e-slackbridge-contact-%s", testRunID)
		monitorName       = fmt.Sprintf("e2e-slackbridge-monitor-%s", testRunID)
		friendlyName      = fmt.Sprintf("E2E Slack Bridge %s", testRunID)
		integrationID     string
	)

	BeforeAll(func() {
		if skipCRDReconciliation {
			Skip("Skipping SlackIntegration Contact bridge tests: UPTIME_ROBOT_API_KEY not set")
		}
		if os.Getenv("UPTIME_ROBOT_SLACK_WEBHOOK_URL") == "" {
			Skip("Skipping SlackIntegration Contact bridge tests: UPTIME_ROBOT_SLACK_WEBHOOK_URL not set")
		}

		ensureE2EInfra()
		ensureSharedAccountAndContact()

		By("creating the webhook secret for the Slack integration")
		secretYAML := fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
type: Opaque
stringData:
  webhookURL: %s
`, webhookSecretName, namespace, os.Getenv("UPTIME_ROBOT_SLACK_WEBHOOK_URL"))
		cmd := exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(secretYAML)
		out, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create webhook secret: %s", out)

		By("creating a SlackIntegration with a unique friendlyName")
		integrationYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: SlackIntegration
metadata:
  name: %s
  namespace: %s
spec:
  syncInterval: 1m
  prune: true
  account:
    name: %s
  integration:
    friendlyName: %q
    enableNotificationsFor: Down
    sslExpirationReminder: false
    secretName: %s
    webhookURLKey: webhookURL
`, integrationName, namespace, legacyAccountName, friendlyName, webhookSecretName)
		cmd = exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(integrationYAML)
		out, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create SlackIntegration: %s", out)

		By("waiting for SlackIntegration to become ready and capturing its ID")
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "slackintegration", integrationName, "-n", namespace,
				"-o", "jsonpath={.status.ready}")
			ready, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(strings.TrimSpace(ready)).To(Equal("true"))
		}, e2ePollTimeout, e2ePollInterval).Should(Succeed())

		cmd = exec.Command("kubectl", "get", "slackintegration", integrationName, "-n", namespace,
			"-o", "jsonpath={.status.id}")
		id, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		integrationID = strings.TrimSpace(id)
		Expect(integrationID).NotTo(BeEmpty(), "SlackIntegration should expose status.id")
	})

	AfterAll(func() {
		if skipCRDReconciliation {
			return
		}
		cmd := exec.Command("kubectl", "delete", "monitor", monitorName, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
		cmd = exec.Command("kubectl", "delete", "contact", contactName, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
		cmd = exec.Command("kubectl", "delete", "slackintegration", integrationName, "-n", namespace, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
		cmd = exec.Command("kubectl", "delete", "secret", webhookSecretName, "-n", namespace, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)

		if integrationID != "" {
			WaitForIntegrationDeletedFromAPI(os.Getenv("UPTIME_ROBOT_API_KEY"), integrationID)
		}
	})

	// Note: surfacing new SlackIntegrations in Account.status.alertContacts is
	// covered by unit tests. The Account controller only requeues on its own
	// spec changes or Secret updates (GenerationChangedPredicate + Secret
	// Watches), so the discoverability surface only refreshes on the next
	// Account reconcile. Forcing one here would be brittle and doesn't add
	// signal beyond what the unit tests already verify.

	It("should resolve a Contact by friendly name to the SlackIntegration ID", func() {
		contactYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: Contact
metadata:
  name: %s
spec:
  isDefault: false
  account:
    name: %s
  contact:
    name: %q
`, contactName, legacyAccountName, friendlyName)
		out, err := applyYAMLWithWebhookRetry("Contact", contactYAML)
		Expect(err).NotTo(HaveOccurred(), "Failed to create Contact: %s", out)

		By("waiting for Contact to become ready")
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "contact", contactName,
				"-o", "jsonpath={.status.ready}")
			ready, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(strings.TrimSpace(ready)).To(Equal("true"))
		}, 2*time.Minute, e2ePollInterval).Should(Succeed())

		By("verifying Contact.status.id matches the SlackIntegration's current status.id")
		// Re-read the SlackIntegration's status.id here instead of using the
		// value captured in BeforeAll: the Slack reconciler can legitimately
		// replace the underlying integration (e.g. on drift-detected recreate),
		// in which case the BeforeAll snapshot is stale but the bridge is
		// still functioning correctly.
		cmd := exec.Command("kubectl", "get", "slackintegration", integrationName, "-n", namespace,
			"-o", "jsonpath={.status.id}")
		currentIntegrationID, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		currentIntegrationID = strings.TrimSpace(currentIntegrationID)
		Expect(currentIntegrationID).NotTo(BeEmpty())
		integrationID = currentIntegrationID

		cmd = exec.Command("kubectl", "get", "contact", contactName, "-o", "jsonpath={.status.id}")
		resolvedID, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.TrimSpace(resolvedID)).To(Equal(currentIntegrationID))

		cmd = exec.Command("kubectl", "get", "contact", contactName,
			"-o", "jsonpath={.status.conditions[?(@.type==\"Synced\")].status}")
		synced, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.TrimSpace(synced)).To(Equal("True"))
	})

	It("should route a Monitor's alert contact to the Slack integration ID", func() {
		Expect(integrationID).NotTo(BeEmpty(), "BeforeAll must have created the Slack integration")

		// Re-read the current SlackIntegration ID (see note in the Contact
		// spec above — the underlying ID can change across reconciles).
		cmd := exec.Command("kubectl", "get", "slackintegration", integrationName, "-n", namespace,
			"-o", "jsonpath={.status.id}")
		currentIntegrationID, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		currentIntegrationID = strings.TrimSpace(currentIntegrationID)
		Expect(currentIntegrationID).NotTo(BeEmpty())
		integrationID = currentIntegrationID

		applyMonitor(fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: %s
spec:
  syncInterval: 1m
  prune: true
  account:
    name: %s
  monitor:
    name: "E2E Slack Bridge Monitor %s"
    url: https://example.com
    type: HTTPS
    interval: 5m
  contacts:
    - name: %s
      threshold: 0s
      recurrence: 0m
`, monitorName, legacyAccountName, testRunID, contactName))

		monitorID := waitMonitorReadyAndGetID(monitorName)
		Expect(monitorID).NotTo(BeEmpty())

		By("verifying the Monitor's assignedAlertContacts includes the Slack integration ID")
		apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")
		Eventually(func(g Gomega) {
			monitor, err := getMonitorFromAPI(apiKey, monitorID)
			g.Expect(err).NotTo(HaveOccurred())
			ids := make([]string, 0, len(monitor.AssignedAlertContacts))
			for _, ac := range monitor.AssignedAlertContacts {
				ids = append(ids, alertContactIDToString(ac.AlertContactID))
			}
			g.Expect(ids).To(ContainElement(currentIntegrationID),
				"Monitor.AssignedAlertContacts should reference the Slack integration ID (got %v)", ids)
		}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
	})
})
