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
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/joelp172/uptime-robot-operator/test/utils"
)

type accountList struct {
	Items []struct {
		Spec struct {
			IsDefault bool `json:"isDefault"`
		} `json:"spec"`
	} `json:"items"`
}

var _ = Describe("Webhook Account Reference Validation", Ordered, Label("webhook", "admission"), func() {
	secretName := fmt.Sprintf("e2e-webhook-accountref-secret-%s", testRunID)
	slackSecretName := fmt.Sprintf("e2e-webhook-slack-secret-%s", testRunID)
	explicitAccountName := fmt.Sprintf("e2e-webhook-accountref-explicit-%s", testRunID)
	defaultAccountName := fmt.Sprintf("e2e-webhook-accountref-default-%s", testRunID)
	backingMonitorName := fmt.Sprintf("e2e-webhook-backing-monitor-%s", testRunID)
	defaultAccountCreated := false

	BeforeAll(func() {
		ensureE2EInfra()

		By("creating a secret for Account resources used by webhook validation tests")
		secretYAML := fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
type: Opaque
stringData:
  apiKey: dummy
`, secretName, namespace)
		cmd := exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(secretYAML)
		out, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create test secret: %s", out)

		By("creating a non-default Account used for explicit account reference checks")
		accountYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: Account
metadata:
  name: %s
spec:
  isDefault: false
  apiKeySecretRef:
    name: %s
    key: apiKey
`, explicitAccountName, secretName)
		out, err = applyYAMLWithWebhookRetry("Account", accountYAML)
		Expect(err).NotTo(HaveOccurred(), "Failed to create explicit account: %s", out)

		By("creating a monitor used by monitorRef webhook checks")
		monitorYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: %s
  namespace: %s
spec:
  account:
    name: %s
  monitor:
    name: "Webhook backing monitor"
    type: HTTPS
    url: https://example.com
`, backingMonitorName, namespace, explicitAccountName)
		out, err = applyYAMLWithWebhookRetry("Monitor", monitorYAML)
		Expect(err).NotTo(HaveOccurred(), "Failed to create backing monitor: %s", out)

		By("creating a secret used by SlackIntegration webhook checks")
		slackSecretYAML := fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
type: Opaque
stringData:
  webhookURL: https://example-slack-webhook.internal/test
`, slackSecretName, namespace)
		cmd = exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(slackSecretYAML)
		out, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create Slack secret: %s", out)

		if !defaultAccountExists() {
			By("creating a default Account so fallback account resolution is admissible")
			defaultYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: Account
metadata:
  name: %s
spec:
  isDefault: true
  apiKeySecretRef:
    name: %s
    key: apiKey
`, defaultAccountName, secretName)
			out, err = applyYAMLWithWebhookRetry("Account", defaultYAML)
			Expect(err).NotTo(HaveOccurred(), "Failed to create fallback default account: %s", out)
			defaultAccountCreated = true
		}
	})

	AfterAll(func() {
		cmd := exec.Command("kubectl", "delete", "monitor", backingMonitorName, "-n", namespace, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)

		cmd = exec.Command("kubectl", "delete", "account", explicitAccountName, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)

		if defaultAccountCreated {
			cmd = exec.Command("kubectl", "delete", "account", defaultAccountName, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)
		}

		cmd = exec.Command("kubectl", "delete", "secret", secretName, "-n", namespace, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
		cmd = exec.Command("kubectl", "delete", "secret", slackSecretName, "-n", namespace, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
	})

	It("should reject Monitor dry-run when account name does not exist", func() {
		monitorName := fmt.Sprintf("e2e-webhook-accountref-missing-%s", testRunID)
		monitorYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: %s
spec:
  account:
    name: does-not-exist
  monitor:
    name: "Webhook missing account check"
    type: HTTPS
    url: https://example.com
`, monitorName)

		out, err := dryRunApplyYAML(monitorYAML)
		Expect(err).To(HaveOccurred(), "Expected unknown account to be rejected by webhook")
		Expect(out).To(ContainSubstring("Account \"does-not-exist\" not found"))
	})

	It("should allow Monitor dry-run when account name exists", func() {
		monitorName := fmt.Sprintf("e2e-webhook-accountref-existing-%s", testRunID)
		monitorYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: %s
spec:
  account:
    name: %s
  monitor:
    name: "Webhook named account check"
    type: HTTPS
    url: https://example.com
`, monitorName, explicitAccountName)

		out, err := dryRunApplyYAML(monitorYAML)
		Expect(err).NotTo(HaveOccurred(), "Expected existing named account to pass webhook validation: %s", out)
	})

	It("should allow Monitor dry-run when account name is omitted and a default account exists", func() {
		monitorName := fmt.Sprintf("e2e-webhook-accountref-default-%s", testRunID)
		monitorYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: %s
spec:
  monitor:
    name: "Webhook default account fallback check"
    type: HTTPS
    url: https://example.com
`, monitorName)

		out, err := dryRunApplyYAML(monitorYAML)
		Expect(err).NotTo(HaveOccurred(), "Expected default account fallback to pass webhook validation: %s", out)
	})

	It("should reject MaintenanceWindow dry-run when monitorRef does not exist", func() {
		name := fmt.Sprintf("e2e-webhook-mw-missing-ref-%s", testRunID)
		yaml := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: MaintenanceWindow
metadata:
  name: %s
  namespace: %s
spec:
  account:
    name: %s
  name: "Webhook MW missing ref"
  interval: daily
  startTime: "02:00:00"
  duration: 30m
  monitorRefs:
  - name: does-not-exist
`, name, namespace, explicitAccountName)
		out, err := dryRunApplyYAML(yaml)
		Expect(err).To(HaveOccurred(), "Expected missing monitorRef to be rejected by webhook")
		Expect(out).To(ContainSubstring(`Monitor "does-not-exist" not found`))
	})

	It("should allow MaintenanceWindow dry-run when monitorRef exists", func() {
		name := fmt.Sprintf("e2e-webhook-mw-existing-ref-%s", testRunID)
		yaml := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: MaintenanceWindow
metadata:
  name: %s
  namespace: %s
spec:
  account:
    name: %s
  name: "Webhook MW existing ref"
  interval: daily
  startTime: "02:00:00"
  duration: 30m
  monitorRefs:
  - name: %s
`, name, namespace, explicitAccountName, backingMonitorName)
		out, err := dryRunApplyYAML(yaml)
		Expect(err).NotTo(HaveOccurred(), "Expected existing monitorRef to pass webhook validation: %s", out)
	})

	It("should reject MaintenanceWindow dry-run with past startDate for once interval", func() {
		name := fmt.Sprintf("e2e-webhook-mw-past-date-%s", testRunID)
		pastDate := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
		yaml := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: MaintenanceWindow
metadata:
  name: %s
  namespace: %s
spec:
  account:
    name: %s
  name: "Webhook MW past date"
  interval: once
  startDate: %s
  startTime: "02:00:00"
  duration: 30m
`, name, namespace, explicitAccountName, pastDate)
		out, err := dryRunApplyYAML(yaml)
		Expect(err).To(HaveOccurred(), "Expected past startDate to be rejected by webhook")
		Expect(out).To(ContainSubstring("startDate must be today or in the future"))
	})

	It("should reject MonitorGroup dry-run when monitorRef does not exist", func() {
		name := fmt.Sprintf("e2e-webhook-mg-missing-ref-%s", testRunID)
		yaml := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: MonitorGroup
metadata:
  name: %s
  namespace: %s
spec:
  account:
    name: %s
  friendlyName: "Webhook MG missing ref"
  monitors:
  - name: does-not-exist
`, name, namespace, explicitAccountName)
		out, err := dryRunApplyYAML(yaml)
		Expect(err).To(HaveOccurred(), "Expected missing monitorRef to be rejected by webhook")
		Expect(out).To(ContainSubstring(`Monitor "does-not-exist" not found`))
	})

	It("should allow MonitorGroup dry-run when monitorRef exists", func() {
		name := fmt.Sprintf("e2e-webhook-mg-existing-ref-%s", testRunID)
		yaml := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: MonitorGroup
metadata:
  name: %s
  namespace: %s
spec:
  account:
    name: %s
  friendlyName: "Webhook MG existing ref"
  monitors:
  - name: %s
`, name, namespace, explicitAccountName, backingMonitorName)
		out, err := dryRunApplyYAML(yaml)
		Expect(err).NotTo(HaveOccurred(), "Expected existing monitorRef to pass webhook validation: %s", out)
	})

	It("should reject SlackIntegration dry-run when webhookURL is invalid", func() {
		name := fmt.Sprintf("e2e-webhook-si-invalid-url-%s", testRunID)
		yaml := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: SlackIntegration
metadata:
  name: %s
  namespace: %s
spec:
  account:
    name: %s
  integration:
    friendlyName: "Webhook SI invalid URL"
    webhookURL: not-a-url
`, name, namespace, explicitAccountName)
		out, err := dryRunApplyYAML(yaml)
		Expect(err).To(HaveOccurred(), "Expected invalid webhookURL to be rejected by webhook")
		Expect(out).To(ContainSubstring("invalid webhook URL"))
	})

	It("should reject SlackIntegration dry-run when webhookURL is not https", func() {
		name := fmt.Sprintf("e2e-webhook-si-http-url-%s", testRunID)
		yaml := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: SlackIntegration
metadata:
  name: %s
  namespace: %s
spec:
  account:
    name: %s
  integration:
    friendlyName: "Webhook SI http URL"
    webhookURL: http://example-slack-webhook.internal/test
`, name, namespace, explicitAccountName)
		out, err := dryRunApplyYAML(yaml)
		Expect(err).To(HaveOccurred(), "Expected non-https webhookURL to be rejected by webhook")
		Expect(out).To(ContainSubstring("webhook URL must use HTTPS"))
	})

	It("should reject SlackIntegration dry-run when secretName does not exist", func() {
		name := fmt.Sprintf("e2e-webhook-si-missing-secret-%s", testRunID)
		yaml := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: SlackIntegration
metadata:
  name: %s
  namespace: %s
spec:
  account:
    name: %s
  integration:
    friendlyName: "Webhook SI missing secret"
    secretName: does-not-exist
`, name, namespace, explicitAccountName)
		out, err := dryRunApplyYAML(yaml)
		Expect(err).To(HaveOccurred(), "Expected missing secretName to be rejected by webhook")
		Expect(out).To(ContainSubstring(`Secret "does-not-exist" not found`))
	})

	It("should allow SlackIntegration dry-run when secretName exists", func() {
		name := fmt.Sprintf("e2e-webhook-si-existing-secret-%s", testRunID)
		yaml := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: SlackIntegration
metadata:
  name: %s
  namespace: %s
spec:
  account:
    name: %s
  integration:
    friendlyName: "Webhook SI existing secret"
    secretName: %s
    webhookURLKey: webhookURL
`, name, namespace, explicitAccountName, slackSecretName)
		out, err := dryRunApplyYAML(yaml)
		Expect(err).NotTo(HaveOccurred(), "Expected existing secretName to pass webhook validation: %s", out)
	})
})

func dryRunApplyYAML(yaml string) (string, error) {
	cmd := exec.Command("kubectl", "apply", "--dry-run=server", "-f", "-")
	cmd.Stdin = strings.NewReader(yaml)
	return utils.Run(cmd)
}

func defaultAccountExists() bool {
	cmd := exec.Command("kubectl", "get", "accounts", "-o", "json")
	out, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to list accounts: %s", out)

	var list accountList
	err = json.Unmarshal([]byte(out), &list)
	Expect(err).NotTo(HaveOccurred(), "Failed to parse account list JSON")

	for i := range list.Items {
		if list.Items[i].Spec.IsDefault {
			return true
		}
	}
	return false
}
