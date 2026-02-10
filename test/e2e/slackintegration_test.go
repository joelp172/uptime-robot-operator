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

		By("ensuring manager namespace exists")
		cmd := exec.Command("kubectl", "get", "ns", namespace)
		_, err := utils.Run(cmd)
		if err != nil {
			cmd = exec.Command("kubectl", "create", "ns", namespace)
			out, runErr := utils.Run(cmd)
			Expect(runErr).NotTo(HaveOccurred(), "Failed to create namespace: %s", out)
		}

		By("labeling the namespace to enforce the restricted security policy")
		cmd = exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
			"pod-security.kubernetes.io/enforce=restricted")
		out, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace: %s", out)

		By("installing CRDs")
		cmd = exec.Command("make", "install")
		out, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs: %s", out)

		By("deploying the controller-manager")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
		out, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager: %s", out)

		By("creating account API key secret")
		apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")
		cmd = exec.Command("kubectl", "delete", "secret", "uptime-robot-e2e", "-n", namespace, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
		secretYAML := fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: uptime-robot-e2e
  namespace: %s
type: Opaque
stringData:
  apiKey: %s
`, namespace, apiKey)
		cmd = exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(secretYAML)
		out, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create API key secret: %s", out)

		By("creating Account resource")
		accountYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: Account
metadata:
  name: e2e-account-%s
spec:
  isDefault: true
  apiKeySecretRef:
    name: uptime-robot-e2e
    key: apiKey
`, testRunID)
		cmd = exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(accountYAML)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("waiting for Account to become ready")
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "account",
				fmt.Sprintf("e2e-account-%s", testRunID),
				"-o", "jsonpath={.status.ready}")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("true"))
		}, 2*time.Minute, 5*time.Second).Should(Succeed())
	})

	AfterAll(func() {
		if skipCRDReconciliation {
			return
		}
		cmd := exec.Command("kubectl", "delete", "slackintegration", fmt.Sprintf("e2e-slackintegration-%s", testRunID), "-n", namespace, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
		cmd = exec.Command("kubectl", "delete", "secret", fmt.Sprintf("e2e-slack-webhook-%s", testRunID), "-n", namespace, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
		cmd = exec.Command("kubectl", "delete", "account", fmt.Sprintf("e2e-account-%s", testRunID), "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
		cmd = exec.Command("kubectl", "delete", "secret", "uptime-robot-e2e", "-n", namespace, "--ignore-not-found=true")
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
})
