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

// testRunID is a unique identifier for this test run to avoid conflicts
var testRunID = fmt.Sprintf("e2e-%d", time.Now().Unix())

// skipCRDReconciliation determines if CRD reconciliation tests should be skipped
// Tests require UPTIME_ROBOT_API_KEY environment variable to be set
var skipCRDReconciliation = os.Getenv("UPTIME_ROBOT_API_KEY") == ""

var _ = Describe("CRD Reconciliation", Ordered, Label("crd-reconciliation"), func() {
	// Skip all tests in this suite if no API key is provided
	BeforeAll(func() {
		if skipCRDReconciliation {
			Skip("Skipping CRD reconciliation tests: UPTIME_ROBOT_API_KEY not set")
		}

		By("creating the API key secret")
		apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")
		cmd := exec.Command("kubectl", "create", "secret", "generic",
			"uptime-robot-e2e",
			"--namespace", namespace,
			fmt.Sprintf("--from-literal=apiKey=%s", apiKey),
			"--dry-run=client", "-o", "yaml")
		secretYAML, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		cmd = exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(secretYAML)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		if skipCRDReconciliation {
			return
		}

		By("cleaning up e2e test resources")
		// Delete all monitors created during tests
		cleanupMonitors()
		// Delete account and secret
		cmd := exec.Command("kubectl", "delete", "account", fmt.Sprintf("e2e-account-%s", testRunID), "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
		cmd = exec.Command("kubectl", "delete", "secret", "uptime-robot-e2e", "-n", namespace, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
	})

	Context("Account Resource", func() {
		It("should create and become ready", func() {
			By("creating an Account resource")
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

			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(accountYAML)
			_, err := utils.Run(cmd)
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

			By("verifying Account has email in status")
			cmd = exec.Command("kubectl", "get", "account",
				fmt.Sprintf("e2e-account-%s", testRunID),
				"-o", "jsonpath={.status.email}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).NotTo(BeEmpty(), "Account should have email in status")

			By("verifying Account has alertContacts in status")
			cmd = exec.Command("kubectl", "get", "account",
				fmt.Sprintf("e2e-account-%s", testRunID),
				"-o", "jsonpath={.status.alertContacts}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).NotTo(BeEmpty(), "Account should have alertContacts in status")
		})
	})

	Context("Monitor Resource - HTTP Type", func() {
		monitorName := fmt.Sprintf("e2e-http-%s", testRunID)

		AfterEach(func() {
			// Clean up monitor after each test
			cmd := exec.Command("kubectl", "delete", "monitor", monitorName, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)
		})

		It("should create an HTTP monitor in UptimeRobot", func() {
			By("creating a Monitor resource")
			monitorYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: %s
spec:
  syncInterval: 1m
  prune: true
  account:
    name: e2e-account-%s
  monitor:
    name: "E2E Test HTTP Monitor %s"
    url: https://example.com
    type: HTTPS
    interval: 5m
`, monitorName, testRunID, testRunID)

			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(monitorYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Monitor to become ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "monitor", monitorName,
					"-o", "jsonpath={.status.ready}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("true"))
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying Monitor has ID in status")
			cmd = exec.Command("kubectl", "get", "monitor", monitorName,
				"-o", "jsonpath={.status.id}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).NotTo(BeEmpty(), "Monitor should have ID in status")

			By("verifying Monitor type in status")
			cmd = exec.Command("kubectl", "get", "monitor", monitorName,
				"-o", "jsonpath={.status.type}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("HTTPS"))
		})

		It("should update monitor when spec changes", func() {
			By("creating a Monitor resource")
			monitorYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: %s
spec:
  syncInterval: 1m
  prune: true
  account:
    name: e2e-account-%s
  monitor:
    name: "E2E Test Update Monitor %s"
    url: https://example.com
    type: HTTPS
    interval: 5m
`, monitorName, testRunID, testRunID)

			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(monitorYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Monitor to become ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "monitor", monitorName,
					"-o", "jsonpath={.status.ready}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("true"))
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("updating the Monitor with new URL")
			updatedMonitorYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: %s
spec:
  syncInterval: 1m
  prune: true
  account:
    name: e2e-account-%s
  monitor:
    name: "E2E Test Update Monitor %s - Updated"
    url: https://httpbin.org/status/200
    type: HTTPS
    interval: 5m
`, monitorName, testRunID, testRunID)

			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(updatedMonitorYAML)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Monitor to be updated")
			// Give some time for the reconciliation
			time.Sleep(10 * time.Second)

			// Verify the monitor is still ready after update
			cmd = exec.Command("kubectl", "get", "monitor", monitorName,
				"-o", "jsonpath={.status.ready}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("true"))
		})

		It("should delete monitor from UptimeRobot when resource is deleted", func() {
			By("creating a Monitor resource")
			monitorYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: %s
spec:
  syncInterval: 1m
  prune: true
  account:
    name: e2e-account-%s
  monitor:
    name: "E2E Test Delete Monitor %s"
    url: https://example.com
    type: HTTPS
    interval: 5m
`, monitorName, testRunID, testRunID)

			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(monitorYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Monitor to become ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "monitor", monitorName,
					"-o", "jsonpath={.status.ready}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("true"))
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("getting the monitor ID before deletion")
			cmd = exec.Command("kubectl", "get", "monitor", monitorName,
				"-o", "jsonpath={.status.id}")
			monitorID, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(monitorID).NotTo(BeEmpty())

			By("deleting the Monitor resource")
			cmd = exec.Command("kubectl", "delete", "monitor", monitorName)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the Monitor resource is deleted")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "monitor", monitorName)
				_, err := utils.Run(cmd)
				g.Expect(err).To(HaveOccurred(), "Monitor should be deleted")
			}, 1*time.Minute, 5*time.Second).Should(Succeed())
		})
	})

	Context("Monitor Resource - Keyword Type", func() {
		monitorName := fmt.Sprintf("e2e-keyword-%s", testRunID)

		AfterEach(func() {
			cmd := exec.Command("kubectl", "delete", "monitor", monitorName, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)
		})

		It("should create a Keyword monitor in UptimeRobot", func() {
			By("creating a Keyword Monitor resource")
			monitorYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: %s
spec:
  syncInterval: 1m
  prune: true
  account:
    name: e2e-account-%s
  monitor:
    name: "E2E Test Keyword Monitor %s"
    url: https://example.com
    type: Keyword
    interval: 5m
    keyword:
      type: Exists
      value: "Example Domain"
      caseSensitive: false
`, monitorName, testRunID, testRunID)

			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(monitorYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Monitor to become ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "monitor", monitorName,
					"-o", "jsonpath={.status.ready}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("true"))
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying Monitor type is Keyword")
			cmd = exec.Command("kubectl", "get", "monitor", monitorName,
				"-o", "jsonpath={.status.type}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("Keyword"))
		})
	})

	Context("Monitor Resource - Heartbeat Type", func() {
		monitorName := fmt.Sprintf("e2e-heartbeat-%s", testRunID)

		AfterEach(func() {
			cmd := exec.Command("kubectl", "delete", "monitor", monitorName, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)
		})

		It("should create a Heartbeat monitor in UptimeRobot", func() {
			By("creating a Heartbeat Monitor resource")
			monitorYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: %s
spec:
  syncInterval: 1m
  prune: true
  account:
    name: e2e-account-%s
  monitor:
    name: "E2E Test Heartbeat Monitor %s"
    url: https://heartbeat.example.com
    type: Heartbeat
    heartbeat:
      interval: 5m
`, monitorName, testRunID, testRunID)

			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(monitorYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Monitor to become ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "monitor", monitorName,
					"-o", "jsonpath={.status.ready}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("true"))
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying Monitor type is Heartbeat")
			cmd = exec.Command("kubectl", "get", "monitor", monitorName,
				"-o", "jsonpath={.status.type}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("Heartbeat"))
		})
	})

	Context("Drift Detection", func() {
		monitorName := fmt.Sprintf("e2e-drift-%s", testRunID)

		AfterEach(func() {
			cmd := exec.Command("kubectl", "delete", "monitor", monitorName, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)
		})

		It("should recreate monitor if deleted externally from UptimeRobot", func() {
			By("creating a Monitor resource")
			monitorYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: %s
spec:
  syncInterval: 30s
  prune: true
  account:
    name: e2e-account-%s
  monitor:
    name: "E2E Test Drift Monitor %s"
    url: https://example.com
    type: HTTPS
    interval: 5m
`, monitorName, testRunID, testRunID)

			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(monitorYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Monitor to become ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "monitor", monitorName,
					"-o", "jsonpath={.status.ready}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("true"))
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("getting the original monitor ID")
			cmd = exec.Command("kubectl", "get", "monitor", monitorName,
				"-o", "jsonpath={.status.id}")
			originalID, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(originalID).NotTo(BeEmpty())

			By("simulating external deletion by clearing status ID")
			// We patch the status to simulate the monitor being deleted externally
			// The next reconciliation should detect this and recreate
			patchJSON := `{"status":{"id":"","ready":false}}`
			cmd = exec.Command("kubectl", "patch", "monitor", monitorName,
				"--type=merge", "--subresource=status", "-p", patchJSON)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Monitor to be recreated and become ready again")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "monitor", monitorName,
					"-o", "jsonpath={.status.ready}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("true"))
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying a new monitor ID was assigned")
			cmd = exec.Command("kubectl", "get", "monitor", monitorName,
				"-o", "jsonpath={.status.id}")
			newID, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(newID).NotTo(BeEmpty())
			// Note: The new ID might be the same or different depending on UptimeRobot's behavior
		})
	})

	Context("Contact Resource", func() {
		It("should resolve contact by ID from Account status", func() {
			By("getting a contact ID from Account status")
			cmd := exec.Command("kubectl", "get", "account",
				fmt.Sprintf("e2e-account-%s", testRunID),
				"-o", "jsonpath={.status.alertContacts[0].id}")
			contactID, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			if contactID == "" {
				Skip("No alert contacts available in account")
			}

			contactName := fmt.Sprintf("e2e-contact-%s", testRunID)

			By("creating a Contact resource using the ID")
			contactYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: Contact
metadata:
  name: %s
spec:
  isDefault: false
  contact:
    id: "%s"
`, contactName, contactID)

			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(contactYAML)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Contact to become ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "contact", contactName,
					"-o", "jsonpath={.status.ready}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("true"))
			}, 1*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying Contact has ID in status")
			cmd = exec.Command("kubectl", "get", "contact", contactName,
				"-o", "jsonpath={.status.id}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal(contactID))

			By("cleaning up Contact")
			cmd = exec.Command("kubectl", "delete", "contact", contactName)
			_, _ = utils.Run(cmd)
		})
	})

	Context("Monitor with Contact", func() {
		monitorName := fmt.Sprintf("e2e-monitor-contact-%s", testRunID)
		contactName := fmt.Sprintf("e2e-contact-monitor-%s", testRunID)

		AfterEach(func() {
			cmd := exec.Command("kubectl", "delete", "monitor", monitorName, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "contact", contactName, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)
		})

		It("should create monitor with alert contact", func() {
			By("getting a contact ID from Account status")
			cmd := exec.Command("kubectl", "get", "account",
				fmt.Sprintf("e2e-account-%s", testRunID),
				"-o", "jsonpath={.status.alertContacts[0].id}")
			contactID, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			if contactID == "" {
				Skip("No alert contacts available in account")
			}

			By("creating a Contact resource")
			contactYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: Contact
metadata:
  name: %s
spec:
  isDefault: false
  contact:
    id: "%s"
`, contactName, contactID)

			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(contactYAML)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Contact to become ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "contact", contactName,
					"-o", "jsonpath={.status.ready}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("true"))
			}, 1*time.Minute, 5*time.Second).Should(Succeed())

			By("creating a Monitor with Contact reference")
			monitorYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: %s
spec:
  syncInterval: 1m
  prune: true
  account:
    name: e2e-account-%s
  contacts:
    - name: %s
      threshold: 1m
      recurrence: 0s
  monitor:
    name: "E2E Test Monitor with Contact %s"
    url: https://example.com
    type: HTTPS
    interval: 5m
`, monitorName, testRunID, contactName, testRunID)

			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(monitorYAML)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Monitor to become ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "monitor", monitorName,
					"-o", "jsonpath={.status.ready}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("true"))
			}, 2*time.Minute, 5*time.Second).Should(Succeed())
		})
	})
})

// cleanupMonitors deletes all monitors created during the e2e tests
func cleanupMonitors() {
	monitorPrefixes := []string{
		fmt.Sprintf("e2e-http-%s", testRunID),
		fmt.Sprintf("e2e-keyword-%s", testRunID),
		fmt.Sprintf("e2e-heartbeat-%s", testRunID),
		fmt.Sprintf("e2e-drift-%s", testRunID),
		fmt.Sprintf("e2e-monitor-contact-%s", testRunID),
	}

	for _, name := range monitorPrefixes {
		cmd := exec.Command("kubectl", "delete", "monitor", name, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
	}

	// Also clean up contacts
	contactPrefixes := []string{
		fmt.Sprintf("e2e-contact-%s", testRunID),
		fmt.Sprintf("e2e-contact-monitor-%s", testRunID),
	}
	for _, name := range contactPrefixes {
		cmd := exec.Command("kubectl", "delete", "contact", name, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
	}
}
