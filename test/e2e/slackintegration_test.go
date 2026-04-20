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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/joelp172/uptime-robot-operator/test/utils"
)

var _ = Describe("SlackIntegration CRD Reconciliation", Ordered, Label("slackintegration"), func() {
	BeforeAll(func() {
		if skipCRDReconciliation {
			Skip("Skipping SlackIntegration tests: UPTIME_ROBOT_API_KEY not set")
		}
		if os.Getenv("UPTIME_ROBOT_SLACK_WEBHOOK_URL") == "" {
			Skip("Skipping SlackIntegration tests: UPTIME_ROBOT_SLACK_WEBHOOK_URL not set")
		}

		ensureE2EInfra()
		ensureSharedAccountAndContact()
	})

	AfterAll(func() {
		if skipCRDReconciliation {
			return
		}
		cmd := exec.Command("kubectl", "delete", "slackintegration", fmt.Sprintf("e2e-slackintegration-%s", testRunID), "-n", namespace, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
		cmd = exec.Command("kubectl", "delete", "secret", fmt.Sprintf("e2e-slack-webhook-%s", testRunID), "-n", namespace, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
	})

	It("should create and prune a Slack integration from the SlackIntegration CR", func() {
		integrationName := fmt.Sprintf("e2e-slackintegration-%s", testRunID)
		webhookSecretName := fmt.Sprintf("e2e-slack-webhook-%s", testRunID)
		friendlyName := fmt.Sprintf("E2E Slack Integration (%s)", testRunID)

		By("creating webhook secret in test namespace")
		webhookSecretYAML := fmt.Sprintf(`
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
		cmd.Stdin = strings.NewReader(webhookSecretYAML)
		out, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create webhook secret: %s", out)

		By("creating SlackIntegration resource")
		resourceYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: SlackIntegration
metadata:
  name: %s
  namespace: %s
spec:
  syncInterval: 1m
  prune: true
  account:
    name: e2e-account-%s
  integration:
    friendlyName: %q
    enableNotificationsFor: Down
    sslExpirationReminder: false
    secretName: %s
    webhookURLKey: webhookURL
    customValue: "created by e2e"
`, integrationName, namespace, testRunID, friendlyName, webhookSecretName)
		cmd = exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(resourceYAML)
		out, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create SlackIntegration: %s", out)

		By("waiting for SlackIntegration to become ready")
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "slackintegration", integrationName, "-n", namespace, "-o", "jsonpath={.status.ready}")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("true"))
		}, e2ePollTimeout, e2ePollInterval).Should(Succeed())

		By("getting status id")
		cmd = exec.Command("kubectl", "get", "slackintegration", integrationName, "-n", namespace, "-o", "jsonpath={.status.id}")
		integrationID, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		integrationID = strings.TrimSpace(integrationID)
		Expect(integrationID).NotTo(BeEmpty())

		By("verifying SlackIntegration status conditions and observedGeneration")
		waitForObservedGeneration("slackintegration", integrationName, namespace)

		cmd = exec.Command("kubectl", "get", "slackintegration", integrationName, "-n", namespace, "-o", "jsonpath={.status.conditions[?(@.type==\"Ready\")].status}")
		readyStatus, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(readyStatus).To(Equal("True"))

		cmd = exec.Command("kubectl", "get", "slackintegration", integrationName, "-n", namespace, "-o", "jsonpath={.status.conditions[?(@.type==\"Synced\")].status}")
		syncedStatus, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(syncedStatus).To(Equal("True"))

		cmd = exec.Command("kubectl", "get", "slackintegration", integrationName, "-n", namespace, "-o", "jsonpath={.status.conditions[?(@.type==\"Error\")].status}")
		errorStatus, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(errorStatus).To(Equal("False"))

		By("verifying integration exists in UptimeRobot API")
		apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")
		Eventually(func(g Gomega) {
			integration, err := getIntegrationFromAPI(apiKey, integrationID)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(integration.Type).NotTo(BeNil())
			g.Expect(*integration.Type).To(Equal("Slack"))
			if integration.FriendlyName != nil {
				g.Expect(*integration.FriendlyName).To(Equal(friendlyName))
			}
		}, e2ePollTimeout, e2ePollInterval).Should(Succeed())

		By("deleting SlackIntegration resource")
		cmd = exec.Command("kubectl", "delete", "slackintegration", integrationName, "-n", namespace)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("verifying integration is deleted from UptimeRobot API")
		WaitForIntegrationDeletedFromAPI(apiKey, integrationID)
	})

	Context("Duplicate SlackIntegration", func() {
		baseName := fmt.Sprintf("e2e-slack-dup-base-%s", testRunID)
		duplicateName := fmt.Sprintf("e2e-slack-dup-attempt-%s", testRunID)
		mismatchName := fmt.Sprintf("e2e-slack-dup-mismatch-%s", testRunID)
		webhookSecretName := fmt.Sprintf("e2e-slack-dup-webhook-%s", testRunID)
		friendlyName := fmt.Sprintf("E2E Slack Duplicate %s", testRunID)
		var sharedIntegrationID string

		BeforeAll(func() {
			if skipCRDReconciliation {
				Skip("Skipping SlackIntegration duplicate tests: UPTIME_ROBOT_API_KEY not set")
			}
			if os.Getenv("UPTIME_ROBOT_SLACK_WEBHOOK_URL") == "" {
				Skip("Skipping SlackIntegration duplicate tests: UPTIME_ROBOT_SLACK_WEBHOOK_URL not set")
			}

			By("creating shared webhook secret for duplicate tests")
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
			Expect(err).NotTo(HaveOccurred(), "Failed to create duplicate webhook secret: %s", out)

			By("creating the base SlackIntegration that subsequent specs adopt against")
			baseYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: SlackIntegration
metadata:
  name: %s
  namespace: %s
spec:
  syncInterval: 1m
  prune: true
  account:
    name: e2e-account-%s
  integration:
    friendlyName: %q
    enableNotificationsFor: Down
    sslExpirationReminder: false
    secretName: %s
    webhookURLKey: webhookURL
`, baseName, namespace, testRunID, friendlyName, webhookSecretName)
			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(baseYAML)
			out, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create base SlackIntegration: %s", out)

			By("waiting for base SlackIntegration to become ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "slackintegration", baseName, "-n", namespace, "-o", "jsonpath={.status.ready}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(strings.TrimSpace(output)).To(Equal("true"))
			}, e2ePollTimeout, e2ePollInterval).Should(Succeed())

			cmd = exec.Command("kubectl", "get", "slackintegration", baseName, "-n", namespace, "-o", "jsonpath={.status.id}")
			baseID, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			baseID = strings.TrimSpace(baseID)
			Expect(baseID).NotTo(BeEmpty())
			sharedIntegrationID = baseID
		})

		AfterAll(func() {
			if skipCRDReconciliation {
				return
			}
			// Delete mismatch and duplicate first (prune: false) so they don't try to clean up the shared integration.
			cmd := exec.Command("kubectl", "delete", "slackintegration", mismatchName, "-n", namespace, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "slackintegration", duplicateName, "-n", namespace, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)
			// Delete the base CR last (prune: true) — this will remove the integration from the API.
			cmd = exec.Command("kubectl", "delete", "slackintegration", baseName, "-n", namespace, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "secret", webhookSecretName, "-n", namespace, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)

			if sharedIntegrationID != "" {
				WaitForIntegrationDeletedFromAPI(os.Getenv("UPTIME_ROBOT_API_KEY"), sharedIntegrationID)
			}
		})

		It("should adopt an existing Slack integration on 409 duplicate and share the ID", func() {
			baseID := sharedIntegrationID
			Expect(baseID).NotTo(BeEmpty(), "BeforeAll must have created the base integration")

			By("creating a duplicate SlackIntegration with identical friendlyName and webhookURL (prune: false)")
			dupYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: SlackIntegration
metadata:
  name: %s
  namespace: %s
spec:
  syncInterval: 1m
  prune: false
  account:
    name: e2e-account-%s
  integration:
    friendlyName: %q
    enableNotificationsFor: Down
    sslExpirationReminder: false
    secretName: %s
    webhookURLKey: webhookURL
`, duplicateName, namespace, testRunID, friendlyName, webhookSecretName)
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(dupYAML)
			out, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create duplicate SlackIntegration: %s", out)

			By("verifying duplicate SlackIntegration adopts the existing integration ID")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "slackintegration", duplicateName, "-n", namespace, "-o", "jsonpath={.status.ready}")
				ready, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(strings.TrimSpace(ready)).To(Equal("true"))

				cmd = exec.Command("kubectl", "get", "slackintegration", duplicateName, "-n", namespace, "-o", "jsonpath={.status.id}")
				dupID, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(strings.TrimSpace(dupID)).To(Equal(baseID))
			}, 2*time.Minute, e2ePollInterval).Should(Succeed())
		})

		It("should not adopt when webhook matches but friendlyName differs", func() {
			Expect(sharedIntegrationID).NotTo(BeEmpty(), "BeforeAll must have created the base integration")

			By("creating a SlackIntegration with same webhook but a different friendlyName")
			mismatchYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: SlackIntegration
metadata:
  name: %s
  namespace: %s
spec:
  syncInterval: 1m
  prune: false
  account:
    name: e2e-account-%s
  integration:
    friendlyName: "E2E Slack Mismatch %s"
    enableNotificationsFor: Down
    sslExpirationReminder: false
    secretName: %s
    webhookURLKey: webhookURL
`, mismatchName, namespace, testRunID, testRunID, webhookSecretName)
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(mismatchYAML)
			out, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create mismatch SlackIntegration: %s", out)

			By("verifying the CR surfaces the 409 error and does not become ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "slackintegration", mismatchName, "-n", namespace,
					"-o", "jsonpath={.status.conditions[?(@.type==\"Error\")].status}")
				errStatus, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(strings.TrimSpace(errStatus)).To(Equal("True"))

				cmd = exec.Command("kubectl", "get", "slackintegration", mismatchName, "-n", namespace, "-o", "jsonpath={.status.ready}")
				ready, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(strings.TrimSpace(ready)).NotTo(Equal("true"))
			}, 2*time.Minute, e2ePollInterval).Should(Succeed())

			By("confirming the mismatch CR did not set status.id (no silent adoption)")
			cmd = exec.Command("kubectl", "get", "slackintegration", mismatchName, "-n", namespace, "-o", "jsonpath={.status.id}")
			mismatchID, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.TrimSpace(mismatchID)).To(BeEmpty())
		})
	})
})
