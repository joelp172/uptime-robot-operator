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

		By("ensuring manager namespace exists")
		cmd := exec.Command("kubectl", "get", "ns", namespace)
		_, err := utils.Run(cmd)
		if err != nil {
			cmd = exec.Command("kubectl", "create", "ns", namespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")
		}

		By("labeling the namespace to enforce the restricted security policy")
		cmd = exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")

		By("installing CRDs")
		cmd = exec.Command("make", "install")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

		By("deploying the controller-manager")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")

		By("creating the API key secret")
		apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")
		cmd = exec.Command("kubectl", "create", "secret", "generic",
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
		cleanupMonitors()
		// Delete default contact, account and secret
		cmd := exec.Command("kubectl", "delete", "contact", fmt.Sprintf("e2e-default-contact-%s", testRunID), "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
		cmd = exec.Command("kubectl", "delete", "account", fmt.Sprintf("e2e-account-%s", testRunID), "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
		cmd = exec.Command("kubectl", "delete", "secret", "uptime-robot-e2e", "-n", namespace, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)

		By("undeploying the controller-manager")
		cmd = exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)

		By("removing manager namespace")
		cmd = exec.Command("kubectl", "delete", "ns", namespace, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
	})

	Context("Account and Default Contact Setup", func() {
		It("should create Account and default Contact", func() {
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

			By("getting the first contact ID from Account status")
			cmd = exec.Command("kubectl", "get", "account",
				fmt.Sprintf("e2e-account-%s", testRunID),
				"-o", "jsonpath={.status.alertContacts[0].id}")
			contactID, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(contactID).NotTo(BeEmpty(), "Account should have at least one alert contact")

			By("creating a default Contact resource")
			contactYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: Contact
metadata:
  name: e2e-default-contact-%s
spec:
  isDefault: true
  contact:
    id: "%s"
`, testRunID, contactID)

			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(contactYAML)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Contact to become ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "contact",
					fmt.Sprintf("e2e-default-contact-%s", testRunID),
					"-o", "jsonpath={.status.ready}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("true"))
			}, 1*time.Minute, 5*time.Second).Should(Succeed())
		})
	})

	Context("Monitor Resource - HTTP Type", func() {
		monitorName := fmt.Sprintf("e2e-http-%s", testRunID)

		AfterEach(func() {
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
		})
	})
})

// cleanupMonitors deletes all monitors created during the e2e tests
func cleanupMonitors() {
	monitorPrefixes := []string{
		fmt.Sprintf("e2e-http-%s", testRunID),
		fmt.Sprintf("e2e-keyword-%s", testRunID),
		fmt.Sprintf("e2e-heartbeat-%s", testRunID),
	}

	for _, name := range monitorPrefixes {
		cmd := exec.Command("kubectl", "delete", "monitor", name, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
	}
}
