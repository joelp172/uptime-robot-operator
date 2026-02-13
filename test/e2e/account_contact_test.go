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

var _ = Describe("Account and Contact Resources", Ordered, Label("account", "contact"), func() {
	// Skip all tests in this suite if no API key is provided
	BeforeAll(func() {
		if skipCRDReconciliation {
			Skip("Skipping Account/Contact tests: UPTIME_ROBOT_API_KEY not set")
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

		By("creating the API key secret")
		apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")
		Expect(apiKey).NotTo(BeEmpty(), "UPTIME_ROBOT_API_KEY must be set for Account/Contact tests")
		// Delete existing secret from a previous run so create succeeds
		cmd = exec.Command("kubectl", "delete", "secret", "uptime-robot-e2e", "-n", namespace, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
		// Use kubectl apply with stdin to avoid exposing API key in command line logs
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
	})

	AfterAll(func() {
		if skipCRDReconciliation {
			return
		}

		By("cleaning up Account and Contact resources")
		cmd := exec.Command("kubectl", "delete", "contact", fmt.Sprintf("e2e-default-contact-%s", testRunID), "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
		cmd = exec.Command("kubectl", "delete", "account", fmt.Sprintf("e2e-account-%s", testRunID), "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
		cmd = exec.Command("kubectl", "delete", "secret", "uptime-robot-e2e", "-n", namespace, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)

		// NOTE: Infrastructure cleanup (undeploy, uninstall CRDs, delete namespace) is handled
		// by e2e_test.go AfterAll to ensure all test suites complete before teardown
	})

	Context("Account Setup", func() {
		It("should create Account and retrieve status", func() {
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

			By("verifying Account has alert contacts in status")
			cmd = exec.Command("kubectl", "get", "account",
				fmt.Sprintf("e2e-account-%s", testRunID),
				"-o", "jsonpath={.status.alertContacts[0].id}")
			contactID, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(contactID).NotTo(BeEmpty(), "Account should have at least one alert contact")

			By("verifying Account status conditions and observedGeneration")
			cmd = exec.Command("kubectl", "get", "account",
				fmt.Sprintf("e2e-account-%s", testRunID),
				"-o", "jsonpath={.status.observedGeneration}")
			observedGeneration, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.TrimSpace(observedGeneration)).NotTo(BeEmpty())

			cmd = exec.Command("kubectl", "get", "account",
				fmt.Sprintf("e2e-account-%s", testRunID),
				"-o", "jsonpath={.status.conditions[?(@.type==\"Ready\")].status}")
			readyStatus, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(readyStatus).To(Equal("True"))

			cmd = exec.Command("kubectl", "get", "account",
				fmt.Sprintf("e2e-account-%s", testRunID),
				"-o", "jsonpath={.status.conditions[?(@.type==\"Synced\")].status}")
			syncedStatus, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(syncedStatus).To(Equal("True"))

			cmd = exec.Command("kubectl", "get", "account",
				fmt.Sprintf("e2e-account-%s", testRunID),
				"-o", "jsonpath={.status.conditions[?(@.type==\"Error\")].status}")
			errorStatus, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(errorStatus).To(Equal("False"))
		})

		It("should reject creating a second default Account", func() {
			By("creating a baseline default Account")
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

			By("attempting to create a second default Account")
			secondDefaultYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: Account
metadata:
  name: e2e-account-second-default-%s
spec:
  isDefault: true
  apiKeySecretRef:
    name: uptime-robot-e2e
    key: apiKey
`, testRunID)

			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(secondDefaultYAML)
			out, err := utils.Run(cmd)
			Expect(err).To(HaveOccurred(), "Expected second default Account apply to fail")
			Expect(out).To(ContainSubstring("spec.isDefault: Forbidden"))
			Expect(out).To(SatisfyAny(
				ContainSubstring("exactly one Account can have spec.isDefault=true"),
				ContainSubstring("at most one Account can have spec.isDefault=true"),
			))
		})
	})

	Context("Contact Setup", func() {
		It("should create default Contact", func() {
			By("creating a baseline default Account")
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

			By("getting the first contact ID from Account status")
			var contactID string
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "account",
					fmt.Sprintf("e2e-account-%s", testRunID),
					"-o", "jsonpath={.status.alertContacts[0].id}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).NotTo(BeEmpty(), "Account should have at least one alert contact")
				contactID = output
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

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

			By("verifying Contact status conditions and observedGeneration")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "contact",
					fmt.Sprintf("e2e-default-contact-%s", testRunID),
					"-o", "jsonpath={.status.observedGeneration}")
				observedGeneration, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(strings.TrimSpace(observedGeneration)).NotTo(BeEmpty())
			}, 1*time.Minute, 5*time.Second).Should(Succeed())

			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "contact",
					fmt.Sprintf("e2e-default-contact-%s", testRunID),
					"-o", "jsonpath={.status.conditions[?(@.type==\"Ready\")].status}")
				readyStatus, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(readyStatus).To(Equal("True"))
			}, 1*time.Minute, 5*time.Second).Should(Succeed())

			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "contact",
					fmt.Sprintf("e2e-default-contact-%s", testRunID),
					"-o", "jsonpath={.status.conditions[?(@.type==\"Synced\")].status}")
				syncedStatus, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(syncedStatus).To(Equal("Unknown"))
			}, 1*time.Minute, 5*time.Second).Should(Succeed())

			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "contact",
					fmt.Sprintf("e2e-default-contact-%s", testRunID),
					"-o", "jsonpath={.status.conditions[?(@.type==\"Synced\")].reason}")
				syncedReason, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(syncedReason).To(Equal("SyncSkipped"))
			}, 1*time.Minute, 5*time.Second).Should(Succeed())

			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "contact",
					fmt.Sprintf("e2e-default-contact-%s", testRunID),
					"-o", "jsonpath={.status.conditions[?(@.type==\"Error\")].status}")
				errorStatus, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(errorStatus).To(Equal("False"))
			}, 1*time.Minute, 5*time.Second).Should(Succeed())
		})

		It("should reject creating a second default Contact", func() {
			By("creating a baseline default Account")
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

			By("getting a contact ID from Account status")
			cmd = exec.Command("kubectl", "get", "account",
				fmt.Sprintf("e2e-account-%s", testRunID),
				"-o", "jsonpath={.status.alertContacts[0].id}")
			contactID, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(contactID).NotTo(BeEmpty(), "Account should have at least one alert contact")

			By("creating a baseline default Contact")
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

			By("attempting to create a second default Contact")
			secondDefaultContactYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: Contact
metadata:
  name: e2e-contact-second-default-%s
spec:
  isDefault: true
  contact:
    id: "%s"
`, testRunID, contactID)

			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(secondDefaultContactYAML)
			out, err := utils.Run(cmd)
			Expect(err).To(HaveOccurred(), "Expected second default Contact apply to fail")
			Expect(out).To(ContainSubstring("spec.isDefault: Forbidden"))
			Expect(out).To(SatisfyAny(
				ContainSubstring("exactly one Contact can have spec.isDefault=true"),
				ContainSubstring("at most one Contact can have spec.isDefault=true"),
			))
		})
	})
})
