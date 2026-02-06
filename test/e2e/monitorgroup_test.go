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
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/joelp172/uptime-robot-operator/internal/uptimerobot"
	"github.com/joelp172/uptime-robot-operator/test/utils"
)

var _ = Describe("MonitorGroup CRD Reconciliation", Ordered, Label("monitorgroup"), func() {
	var (
		monitorGroupName string
		monitorName      string
	)

	// Skip all tests in this suite if no API key is provided
	BeforeAll(func() {
		if skipCRDReconciliation {
			Skip("Skipping MonitorGroup CRD reconciliation tests: UPTIME_ROBOT_API_KEY not set")
		}

		apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")
		debugLog("Setting up Account for MonitorGroup tests with testRunID: %s", testRunID)

		By("creating Secret with API key for MonitorGroup tests")
		secretYAML := fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: uptime-robot-e2e-mg
  namespace: %s
type: Opaque
stringData:
  apiKey: %s
`, namespace, apiKey)
		debugLog("Applying Secret for MonitorGroup tests (name: uptime-robot-e2e-mg, namespace: %s)", namespace)
		cmd := exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(secretYAML)
		output, err := utils.Run(cmd)
		if err != nil {
			debugLog("Failed to create Secret: %v, output: %s", err, output)
		} else {
			debugLog("Secret created successfully: %s", output)
		}
		Expect(err).NotTo(HaveOccurred())

		By("creating Account resource for MonitorGroup tests")
		accountYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: Account
metadata:
  name: e2e-account-mg-%s
spec:
  isDefault: true
  apiKeySecretRef:
    name: uptime-robot-e2e-mg
    key: apiKey
`, testRunID)
		debugLog("Applying Account YAML:\n%s", accountYAML)
		cmd = exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(accountYAML)
		output, err = utils.Run(cmd)
		if err != nil {
			debugLog("Failed to create Account: %v, output: %s", err, output)
		} else {
			debugLog("Account created successfully: %s", output)
		}
		Expect(err).NotTo(HaveOccurred())

		// Wait for Account to be ready
		debugLog("Waiting for Account e2e-account-mg-%s to become ready", testRunID)
		pollCount := 0
		Eventually(func(g Gomega) {
			pollCount++
			cmd := exec.Command("kubectl", "get", "account", fmt.Sprintf("e2e-account-mg-%s", testRunID),
				"-o", "jsonpath={.status.ready}")
			output, err := utils.Run(cmd)
			if err != nil {
				debugLog("Poll #%d: Failed to get Account: %v", pollCount, err)
			} else {
				debugLog("Poll #%d: Account status ready=%s", pollCount, output)
			}
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("true"))
		}, 2*time.Minute, 5*time.Second).Should(Succeed())
		debugLog("Account e2e-account-mg-%s is ready", testRunID)
	})

	AfterAll(func() {
		if skipCRDReconciliation {
			return
		}

		By("cleaning up Account resource for MonitorGroup tests")
		cmd := exec.Command("kubectl", "delete", "account", fmt.Sprintf("e2e-account-mg-%s", testRunID), "--ignore-not-found=true")
		_, _ = utils.Run(cmd)

		By("cleaning up Secret for MonitorGroup tests")
		cmd = exec.Command("kubectl", "delete", "secret", "uptime-robot-e2e-mg", "-n", namespace, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
	})

	Context("Basic MonitorGroup lifecycle", func() {
		BeforeEach(func() {
			monitorGroupName = fmt.Sprintf("e2e-monitor-group-%s", testRunID)
		})

		AfterEach(func() {
			By("cleaning up MonitorGroup resource")
			cmd := exec.Command("kubectl", "delete", "monitorgroup", monitorGroupName, "-n", namespace, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)
		})

		It("should create and reconcile a MonitorGroup", func() {
			By("creating MonitorGroup resource")
			monitorGroupYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: MonitorGroup
metadata:
  name: %s
  namespace: %s
spec:
  account:
    name: e2e-account-mg-%s
  friendlyName: "E2E Test Group %s"
  syncInterval: 24h
`, monitorGroupName, namespace, testRunID, testRunID)

			debugLog("Applying MonitorGroup YAML:\n%s", monitorGroupYAML)
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(monitorGroupYAML)
			output, err := utils.Run(cmd)
			debugLog("MonitorGroup creation output: %s", output)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for MonitorGroup to become ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "monitorgroup", monitorGroupName, "-n", namespace,
					"-o", "jsonpath={.status.ready}")
				output, err := utils.Run(cmd)
				debugLog("MonitorGroup ready status: %s", output)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("true"))
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying MonitorGroup has an ID")
			cmd = exec.Command("kubectl", "get", "monitorgroup", monitorGroupName, "-n", namespace,
				"-o", "jsonpath={.status.id}")
			output, err = utils.Run(cmd)
			debugLog("MonitorGroup ID: %s", output)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).NotTo(BeEmpty())

			By("verifying MonitorGroup exists in UptimeRobot API")
			apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")
			client := uptimerobot.NewClient(apiKey)
			groups, err := client.EnumerateGroupsFromBackend(context.Background())
			Expect(err).NotTo(HaveOccurred())

			found := false
			for _, group := range groups {
				if group.Name == fmt.Sprintf("E2E Test Group %s", testRunID) {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "MonitorGroup should exist in UptimeRobot API")
		})

		It("should update MonitorGroup name", func() {
			By("creating MonitorGroup resource")
			monitorGroupYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: MonitorGroup
metadata:
  name: %s
  namespace: %s
spec:
  account:
    name: e2e-account-mg-%s
  friendlyName: "E2E Test Group Original %s"
`, monitorGroupName, namespace, testRunID, testRunID)

			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(monitorGroupYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for MonitorGroup to become ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "monitorgroup", monitorGroupName, "-n", namespace,
					"-o", "jsonpath={.status.ready}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("true"))
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("updating MonitorGroup friendly name")
			updatedYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: MonitorGroup
metadata:
  name: %s
  namespace: %s
spec:
  account:
    name: e2e-account-mg-%s
  friendlyName: "E2E Test Group Updated %s"
`, monitorGroupName, namespace, testRunID, testRunID)

			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(updatedYAML)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for update to propagate")
			time.Sleep(10 * time.Second)

			By("verifying updated name in UptimeRobot API")
			apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")
			client := uptimerobot.NewClient(apiKey)
			groups, err := client.EnumerateGroupsFromBackend(context.Background())
			Expect(err).NotTo(HaveOccurred())

			found := false
			for _, group := range groups {
				if group.Name == fmt.Sprintf("E2E Test Group Updated %s", testRunID) {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "Updated MonitorGroup name should exist in UptimeRobot API")
		})

		It("should delete MonitorGroup and clean up in UptimeRobot", func() {
			By("creating MonitorGroup resource")
			monitorGroupYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: MonitorGroup
metadata:
  name: %s
  namespace: %s
spec:
  account:
    name: e2e-account-mg-%s
  friendlyName: "E2E Test Group Delete %s"
  prune: true
`, monitorGroupName, namespace, testRunID, testRunID)

			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(monitorGroupYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for MonitorGroup to become ready")
			var groupID string
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "monitorgroup", monitorGroupName, "-n", namespace,
					"-o", "jsonpath={.status.ready}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("true"))

				// Get the ID
				cmd = exec.Command("kubectl", "get", "monitorgroup", monitorGroupName, "-n", namespace,
					"-o", "jsonpath={.status.id}")
				groupID, err = utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(groupID).NotTo(BeEmpty())
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("deleting MonitorGroup resource")
			cmd = exec.Command("kubectl", "delete", "monitorgroup", monitorGroupName, "-n", namespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for MonitorGroup to be removed")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "monitorgroup", monitorGroupName, "-n", namespace)
				_, err := utils.Run(cmd)
				g.Expect(err).To(HaveOccurred())
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying MonitorGroup is deleted from UptimeRobot API")
			time.Sleep(10 * time.Second)
			apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")
			client := uptimerobot.NewClient(apiKey)
			groups, err := client.EnumerateGroupsFromBackend(context.Background())
			Expect(err).NotTo(HaveOccurred())

			found := false
			for _, group := range groups {
				if group.Name == fmt.Sprintf("E2E Test Group Delete %s", testRunID) {
					found = true
					break
				}
			}
			Expect(found).To(BeFalse(), "MonitorGroup should be deleted from UptimeRobot API")
		})
	})

	Context("MonitorGroup with Monitor references", func() {
		BeforeEach(func() {
			monitorGroupName = fmt.Sprintf("e2e-monitor-group-ref-%s", testRunID)
			monitorName = fmt.Sprintf("e2e-monitor-for-group-%s", testRunID)
		})

		AfterEach(func() {
			By("cleaning up Monitor resource")
			cmd := exec.Command("kubectl", "delete", "monitor", monitorName, "-n", namespace, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)

			By("cleaning up MonitorGroup resource")
			cmd = exec.Command("kubectl", "delete", "monitorgroup", monitorGroupName, "-n", namespace, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)
		})

		It("should create MonitorGroup with Monitor references", func() {
			By("creating a Monitor resource first")
			monitorYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: %s
  namespace: %s
spec:
  account:
    name: e2e-account-mg-%s
  monitor:
    name: "E2E Test Monitor For Group %s"
    url: "https://example-group-test-%s.com"
    type: HTTPS
`, monitorName, namespace, testRunID, testRunID, testRunID)

			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(monitorYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Monitor to become ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "monitor", monitorName, "-n", namespace,
					"-o", "jsonpath={.status.ready}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("true"))
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("creating MonitorGroup with Monitor reference")
			monitorGroupYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: MonitorGroup
metadata:
  name: %s
  namespace: %s
spec:
  account:
    name: e2e-account-mg-%s
  friendlyName: "E2E Test Group With Monitors %s"
  monitors:
    - name: %s
`, monitorGroupName, namespace, testRunID, testRunID, monitorName)

			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(monitorGroupYAML)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for MonitorGroup to become ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "monitorgroup", monitorGroupName, "-n", namespace,
					"-o", "jsonpath={.status.ready}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("true"))
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying MonitorGroup has the correct monitor count")
			cmd = exec.Command("kubectl", "get", "monitorgroup", monitorGroupName, "-n", namespace,
				"-o", "jsonpath={.status.monitorCount}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("1"))
		})
	})
})
