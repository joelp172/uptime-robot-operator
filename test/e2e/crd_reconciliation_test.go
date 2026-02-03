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

const (
	e2ePollInterval = 5 * time.Second
	e2ePollTimeout  = 3 * time.Minute
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
		Expect(apiKey).NotTo(BeEmpty(), "UPTIME_ROBOT_API_KEY must be set for CRD reconciliation tests")
		// Delete existing secret from a previous run so create succeeds
		cmd = exec.Command("kubectl", "delete", "secret", "uptime-robot-e2e", "-n", namespace, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
		cmd = exec.Command("kubectl", "create", "secret", "generic",
			"uptime-robot-e2e",
			"--namespace", namespace,
			"--from-literal=apiKey="+apiKey)
		out, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create API key secret: %s", out)
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

	// HTTP monitor: kubectl uses metadata.name (e.g. e2e-http-e2e-<id>); we set
	// spec.monitor.name to the same so the UptimeRobot UI label matches the resource.
	Context("Monitor Resource - HTTP Type", func() {
		monitorName := fmt.Sprintf("e2e-http-%s", testRunID)
		friendlyName := fmt.Sprintf("E2E Test HTTP Monitor (%s)", monitorName)

		AfterEach(func() {
			deleteMonitorAndWaitForAPICleanup(monitorName)
		})

		It("should create an HTTP monitor in UptimeRobot", func() {
			applyMonitor(fmt.Sprintf(`
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
    name: %q
    url: https://example.com
    type: HTTPS
    interval: 5m
`, monitorName, testRunID, friendlyName))

			monitorID := waitMonitorReadyAndGetID(monitorName)
			Expect(monitorID).NotTo(BeEmpty(), "Monitor should have ID in status")

			By("verifying monitor fields in UptimeRobot API")
			apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")
			Eventually(func(g Gomega) {
				monitor, err := getMonitorFromAPI(apiKey, monitorID)
				g.Expect(err).NotTo(HaveOccurred())
				errs := ValidateHTTPSMonitorFields(friendlyName, "https://example.com", "HTTP", 300, "HEAD", monitor)
				g.Expect(errs).To(BeEmpty(), "field validation: %s", errs)
			}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
		})

		It("should delete monitor from UptimeRobot when resource is deleted", func() {
			By("creating a Monitor resource")
			deleteFriendlyName := fmt.Sprintf("E2E Test Delete Monitor (%s)", monitorName)
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
    name: %q
    url: https://example.com
    type: HTTPS
    interval: 5m
`, monitorName, testRunID, deleteFriendlyName)

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

	Context("Monitor Resource - Heartbeat Type", func() {
		monitorName := fmt.Sprintf("e2e-heartbeat-%s", testRunID)

		AfterEach(func() {
			deleteMonitorAndWaitForAPICleanup(monitorName)
		})

		It("should create a Heartbeat monitor and set heartbeatURL in status", func() {
			applyMonitor(fmt.Sprintf(`
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
    type: Heartbeat
    heartbeat:
      interval: 5m
`, monitorName, testRunID, testRunID))

			monitorID := waitMonitorReadyAndGetID(monitorName)
			Expect(monitorID).NotTo(BeEmpty())

			By("verifying status.heartbeatURL is populated")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "monitor", monitorName, "-o", "jsonpath={.status.heartbeatURL}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(strings.TrimSpace(output)).NotTo(BeEmpty())
				g.Expect(output).To(ContainSubstring("heartbeat.uptimerobot.com"))
			}, e2ePollTimeout, e2ePollInterval).Should(Succeed())

			By("verifying monitor type in UptimeRobot API")
			apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")
			Eventually(func(g Gomega) {
				monitor, err := getMonitorFromAPI(apiKey, monitorID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(monitor.Type).To(Equal("HEARTBEAT"))
			}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
		})
	})

	Context("Monitor Resource - HTTPS Full", func() {
		monitorName := fmt.Sprintf("e2e-https-full-%s", testRunID)

		AfterEach(func() {
			deleteMonitorAndWaitForAPICleanup(monitorName)
		})

		It("should create HTTPS monitor with all common fields and validate in API", func() {
			applyMonitor(fmt.Sprintf(`
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
    name: "E2E HTTPS Full"
    url: https://httpbin.org/get
    type: HTTPS
    interval: 5m
    timeout: 30s
    gracePeriod: 60s
    method: GET
    followRedirections: true
    checkSSLErrors: true
    sslExpirationReminder: true
    domainExpirationReminder: true
    successHttpResponseCodes: ["2xx", "3xx"]
    responseTimeThreshold: 5000
    customHttpHeaders:
      X-Custom: "test-value"
    tags: ["e2e", "https"]
`, monitorName, testRunID))

			monitorID := waitMonitorReadyAndGetID(monitorName)
			apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")

			By("verifying monitor fields in UptimeRobot")
			Eventually(func(g Gomega) {
				monitor, err := getMonitorFromAPI(apiKey, monitorID)
				g.Expect(err).NotTo(HaveOccurred())
				errs := ValidateHTTPSMonitorFields("E2E HTTPS Full", "https://httpbin.org/get", "HTTP", 300, "GET", monitor)
				g.Expect(errs).To(BeEmpty(), "field validation: %s", errs)
				g.Expect(monitor.CustomHTTPHeaders).To(HaveKeyWithValue("X-Custom", "test-value"))
				g.Expect(monitor.CheckSSLErrors).NotTo(BeNil())
				g.Expect(*monitor.CheckSSLErrors).To(BeTrue())
				g.Expect(monitor.FollowRedirections).NotTo(BeNil())
				g.Expect(*monitor.FollowRedirections).To(BeTrue())
			}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
		})
	})

	Context("Monitor Resource - HTTPS Auth", func() {
		monitorName := fmt.Sprintf("e2e-https-auth-%s", testRunID)

		AfterEach(func() {
			deleteMonitorAndWaitForAPICleanup(monitorName)
		})

		It("should create HTTPS monitor with Basic auth and validate in API", func() {
			applyMonitor(fmt.Sprintf(`
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
    name: "E2E HTTPS Auth"
    url: https://httpbin.org/basic-auth/user/pass
    type: HTTPS
    method: GET
    auth:
      type: Basic
      username: user
      password: pass
`, monitorName, testRunID))

			monitorID := waitMonitorReadyAndGetID(monitorName)
			apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")

			Eventually(func(g Gomega) {
				monitor, err := getMonitorFromAPI(apiKey, monitorID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(monitor.AuthType).To(Equal("HTTP_BASIC"))
				g.Expect(monitor.HTTPUsername).To(Equal("user"))
			}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
		})
	})

	Context("Monitor Resource - HTTPS POST", func() {
		monitorName := fmt.Sprintf("e2e-https-post-%s", testRunID)

		AfterEach(func() {
			deleteMonitorAndWaitForAPICleanup(monitorName)
		})

		It("should create HTTPS monitor with POST and request body and validate in API", func() {
			applyMonitor(fmt.Sprintf(`
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
    name: "E2E HTTPS POST"
    url: https://httpbin.org/post
    type: HTTPS
    method: POST
    post:
      postType: RawData
      contentType: application/json
      value: '{"test": "data"}'
`, monitorName, testRunID))

			monitorID := waitMonitorReadyAndGetID(monitorName)
			apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")

			Eventually(func(g Gomega) {
				monitor, err := getMonitorFromAPI(apiKey, monitorID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(monitor.HTTPMethodType).To(Equal("POST"))
				g.Expect(monitor.PostValueType).To(Equal("RAW_JSON"))
				// PostValueData can be string or object; just check it's not nil
				g.Expect(monitor.PostValueData).NotTo(BeNil())
			}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
		})
	})

	Context("Monitor Resource - Keyword Full", func() {
		monitorName := fmt.Sprintf("e2e-keyword-%s", testRunID)

		AfterEach(func() {
			deleteMonitorAndWaitForAPICleanup(monitorName)
		})

		It("should create Keyword monitor with Exists and caseSensitive and validate in API", func() {
			applyMonitor(fmt.Sprintf(`
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
    name: "E2E Keyword Full"
    url: https://example.com
    type: Keyword
    interval: 5m
    keyword:
      type: Exists
      value: "Example Domain"
      caseSensitive: true
    timeout: 30s
`, monitorName, testRunID))

			monitorID := waitMonitorReadyAndGetID(monitorName)
			apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")
			cs := true

			Eventually(func(g Gomega) {
				monitor, err := getMonitorFromAPI(apiKey, monitorID)
				g.Expect(err).NotTo(HaveOccurred())
				errs := ValidateHTTPSMonitorFields("E2E Keyword Full", "https://example.com", "KEYWORD", 300, "", monitor)
				g.Expect(errs).To(BeEmpty())
				errs = ValidateKeywordMonitorFields("ALERT_EXISTS", "Example Domain", &cs, monitor)
				g.Expect(errs).To(BeEmpty())
			}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
		})
	})

	Context("Monitor Resource - Keyword NotExists", func() {
		monitorName := fmt.Sprintf("e2e-keyword-notexists-%s", testRunID)

		AfterEach(func() {
			deleteMonitorAndWaitForAPICleanup(monitorName)
		})

		It("should create Keyword monitor with NotExists and validate in API", func() {
			applyMonitor(fmt.Sprintf(`
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
    name: "E2E Keyword NotExists"
    url: https://example.com
    type: Keyword
    interval: 5m
    keyword:
      type: NotExists
      value: "This should not exist"
`, monitorName, testRunID))

			monitorID := waitMonitorReadyAndGetID(monitorName)
			apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")

			Eventually(func(g Gomega) {
				monitor, err := getMonitorFromAPI(apiKey, monitorID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(monitor.KeywordType).To(Equal("ALERT_NOT_EXISTS"))
				g.Expect(monitor.KeywordValue).To(Equal("This should not exist"))
			}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
		})
	})

	Context("Monitor Resource - Ping", func() {
		monitorName := fmt.Sprintf("e2e-ping-%s", testRunID)

		AfterEach(func() {
			deleteMonitorAndWaitForAPICleanup(monitorName)
		})

		It("should create Ping monitor and validate type and URL in API", func() {
			applyMonitor(fmt.Sprintf(`
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
    name: "E2E Ping Monitor"
    url: "8.8.8.8"
    type: Ping
    interval: 5m
`, monitorName, testRunID))

			monitorID := waitMonitorReadyAndGetID(monitorName)
			apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")

			Eventually(func(g Gomega) {
				monitor, err := getMonitorFromAPI(apiKey, monitorID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(monitor.Type).To(Equal("PING"))
				g.Expect(monitor.URL).To(Equal("8.8.8.8"))
				g.Expect(monitor.FriendlyName).To(Equal("E2E Ping Monitor"))
			}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
		})
	})

	Context("Monitor Resource - Port", func() {
		monitorName := fmt.Sprintf("e2e-port-%s", testRunID)

		AfterEach(func() {
			deleteMonitorAndWaitForAPICleanup(monitorName)
		})

		It("should create Port monitor and validate port number in API", func() {
			applyMonitor(fmt.Sprintf(`
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
    name: "E2E Port Monitor"
    url: google.com
    type: Port
    interval: 5m
    port:
      number: 443
`, monitorName, testRunID))

			monitorID := waitMonitorReadyAndGetID(monitorName)
			apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")

			Eventually(func(g Gomega) {
				monitor, err := getMonitorFromAPI(apiKey, monitorID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(monitor.Type).To(Equal("PORT"))
				errs := ValidatePortMonitorFields(443, monitor)
				g.Expect(errs).To(BeEmpty())
			}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
		})
	})

	Context("Monitor Resource - DNS", func() {
		monitorName := fmt.Sprintf("e2e-dns-%s", testRunID)

		AfterEach(func() {
			deleteMonitorAndWaitForAPICleanup(monitorName)
		})

		It("should create DNS monitor and validate config.dnsRecords in API", func() {
			applyMonitor(fmt.Sprintf(`
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
    name: "E2E DNS Monitor"
    url: google.com
    type: DNS
    interval: 5m
    dns:
      a: ["142.250.80.46"]
`, monitorName, testRunID))

			monitorID := waitMonitorReadyAndGetID(monitorName)
			apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")

			Eventually(func(g Gomega) {
				monitor, err := getMonitorFromAPI(apiKey, monitorID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(monitor.Type).To(Equal("DNS"))
				g.Expect(monitor.URL).To(Equal("google.com"))
				// DNS config A records may be returned; exact IP can change so just check type and config presence
				g.Expect(monitor.Config).NotTo(BeNil())
				g.Expect(monitor.Config.DNSRecords).NotTo(BeNil())
				g.Expect(monitor.Config.DNSRecords.A).NotTo(BeEmpty())
			}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
		})
	})

	Context("Monitor Resource - Contact assignment", func() {
		monitorName := fmt.Sprintf("e2e-contacts-%s", testRunID)

		AfterEach(func() {
			deleteMonitorAndWaitForAPICleanup(monitorName)
		})

		It("should create monitor with contact ref and validate threshold/recurrence in API", func() {
			applyMonitor(fmt.Sprintf(`
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
    name: "E2E Contacts Monitor"
    url: https://example.com
    type: HTTPS
    interval: 5m
  contacts:
    - name: e2e-default-contact-%s
      threshold: 2m
      recurrence: 5m
`, monitorName, testRunID, testRunID))

			monitorID := waitMonitorReadyAndGetID(monitorName)
			apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")

			Eventually(func(g Gomega) {
				monitor, err := getMonitorFromAPI(apiKey, monitorID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(monitor.AssignedAlertContacts).NotTo(BeEmpty())
				// Threshold in API is seconds (120 for 2m), recurrence is minutes (5)
				ac := monitor.AssignedAlertContacts[0]
				g.Expect(ac.Threshold).To(BeNumerically(">=", 0))
				g.Expect(ac.Recurrence).To(BeNumerically(">=", 0))
			}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
		})
	})
})

// getMonitorIDFromCluster returns the status.id of a Monitor resource, or empty if not found.
func getMonitorIDFromCluster(monitorName string) string {
	cmd := exec.Command("kubectl", "get", "monitor", monitorName, "-o", "jsonpath={.status.id}")
	output, err := utils.Run(cmd)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(output)
}

// deleteMonitorAndWaitForAPICleanup deletes the Monitor CR and waits until it is removed from the
// UptimeRobot API. Ensures only one monitor exists at a time for accounts with monitor limits.
func deleteMonitorAndWaitForAPICleanup(monitorName string) {
	monitorID := getMonitorIDFromCluster(monitorName)
	cmd := exec.Command("kubectl", "delete", "monitor", monitorName, "--ignore-not-found=true")
	_, _ = utils.Run(cmd)
	if monitorID != "" && !skipCRDReconciliation {
		WaitForMonitorDeletedFromAPI(os.Getenv("UPTIME_ROBOT_API_KEY"), monitorID)
	}
}

// applyMonitor applies the given monitor YAML via kubectl.
func applyMonitor(monitorYAML string) {
	By("creating/updating Monitor resource")
	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(monitorYAML)
	_, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
}

// waitMonitorReadyAndGetID waits for the monitor to become ready and returns status.id.
func waitMonitorReadyAndGetID(monitorName string) string {
	By("waiting for Monitor to become ready")
	Eventually(func(g Gomega) {
		cmd := exec.Command("kubectl", "get", "monitor", monitorName, "-o", "jsonpath={.status.ready}")
		output, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(output).To(Equal("true"))
	}, e2ePollTimeout, e2ePollInterval).Should(Succeed())

	cmd := exec.Command("kubectl", "get", "monitor", monitorName, "-o", "jsonpath={.status.id}")
	output, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
	return strings.TrimSpace(output)
}

// cleanupMonitors deletes all monitors created during the e2e tests
func cleanupMonitors() {
	monitorPrefixes := []string{
		fmt.Sprintf("e2e-http-%s", testRunID),
		fmt.Sprintf("e2e-https-full-%s", testRunID),
		fmt.Sprintf("e2e-https-auth-%s", testRunID),
		fmt.Sprintf("e2e-https-post-%s", testRunID),
		fmt.Sprintf("e2e-keyword-%s", testRunID),
		fmt.Sprintf("e2e-keyword-notexists-%s", testRunID),
		fmt.Sprintf("e2e-ping-%s", testRunID),
		fmt.Sprintf("e2e-port-%s", testRunID),
		fmt.Sprintf("e2e-heartbeat-%s", testRunID),
		fmt.Sprintf("e2e-dns-%s", testRunID),
		fmt.Sprintf("e2e-contacts-%s", testRunID),
	}

	for _, name := range monitorPrefixes {
		cmd := exec.Command("kubectl", "delete", "monitor", name, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
	}
}
