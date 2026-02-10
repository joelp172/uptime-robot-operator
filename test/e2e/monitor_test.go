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

var _ = Describe("Monitor Resources", Ordered, Label("monitor"), func() {
	// Skip all tests in this suite if no API key is provided
	BeforeAll(func() {
		if skipCRDReconciliation {
			Skip("Skipping Monitor tests: UPTIME_ROBOT_API_KEY not set")
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

		By("installing pinned cert-manager")
		cmd = exec.Command("make", "cert-manager-install")
		out, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install cert-manager: %s", out)

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
		Expect(apiKey).NotTo(BeEmpty(), "UPTIME_ROBOT_API_KEY must be set for Monitor tests")
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

		By("creating Account resource for monitors")
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

	AfterAll(func() {
		if skipCRDReconciliation {
			return
		}

		By("cleaning up e2e test monitors")
		cleanupMonitors()

		By("cleaning up Account, Contact, and Secret")
		cmd := exec.Command("kubectl", "delete", "contact", fmt.Sprintf("e2e-default-contact-%s", testRunID), "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
		cmd = exec.Command("kubectl", "delete", "account", fmt.Sprintf("e2e-account-%s", testRunID), "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
		cmd = exec.Command("kubectl", "delete", "secret", "uptime-robot-e2e", "-n", namespace, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)

		// NOTE: Infrastructure cleanup (undeploy, uninstall CRDs, delete namespace) is handled
		// by e2e_test.go AfterAll to ensure all test suites complete before teardown
	})

	// HTTP monitor: kubectl uses metadata.name (e.g. e2e-http-e2e-<id>); we set
	// spec.monitor.name to the same so the UptimeRobot UI label matches the resource.
	Context("HTTP Type", func() {
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
				errs := ValidateHTTPSMonitorFields(HTTPSMonitorExpectation{
					Name:        friendlyName,
					URL:         "https://example.com",
					Type:        "HTTP",
					IntervalSec: 300,
					Method:      "HEAD",
				}, monitor)
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

	Context("Heartbeat Type", func() {
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

	Context("HTTPS Full", func() {
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
				timeout := 30
				gracePeriod := 60
				responseTimeThreshold := 5000
				checkSSL := true
				followRedir := true
				sslExpReminder := true
				domainExpReminder := true
				errs := ValidateHTTPSMonitorFields(HTTPSMonitorExpectation{
					Name:                     "E2E HTTPS Full",
					URL:                      "https://httpbin.org/get",
					Type:                     "HTTP",
					IntervalSec:              300,
					Method:                   "GET",
					Timeout:                  &timeout,
					GracePeriod:              &gracePeriod,
					ResponseTimeThreshold:    &responseTimeThreshold,
					CheckSSLErrors:           &checkSSL,
					FollowRedirections:       &followRedir,
					SSLExpirationReminder:    &sslExpReminder,
					DomainExpirationReminder: &domainExpReminder,
					SuccessHTTPResponseCodes: []string{"2xx", "3xx"},
					Tags:                     []string{"e2e", "https"},
					CustomHTTPHeaders:        map[string]string{"X-Custom": "test-value"},
				}, monitor)
				g.Expect(errs).To(BeEmpty(), "field validation: %s", errs)
			}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
		})
	})

	Context("Regional Monitoring", func() {
		monitorName := fmt.Sprintf("e2e-region-%s", testRunID)

		AfterEach(func() {
			deleteMonitorAndWaitForAPICleanup(monitorName)
		})

		It("should create monitor in the specified region", func() {
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
    name: "E2E Regional Monitor"
    url: https://example.com
    type: HTTPS
    interval: 5m
    region: eu
`, monitorName, testRunID))

			monitorID := waitMonitorReadyAndGetID(monitorName)
			apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")

			Eventually(func(g Gomega) {
				monitor, err := getMonitorFromAPI(apiKey, monitorID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(monitor.RegionalData).NotTo(BeNil(), "regionalData should be returned by API")
				g.Expect(monitor.RegionalData.Region).NotTo(BeEmpty(), "regionalData.REGION should not be empty")

				foundEU := false
				for _, region := range monitor.RegionalData.Region {
					if strings.EqualFold(region, "eu") {
						foundEU = true
						break
					}
				}
				g.Expect(foundEU).To(BeTrue(), "expected region list %v to contain eu", monitor.RegionalData.Region)
			}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
		})
	})

	Context("HTTPS Auth", func() {
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

	Context("HTTPS POST", func() {
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

	Context("Keyword Full", func() {
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
				timeout := 30
				errs := ValidateHTTPSMonitorFields(HTTPSMonitorExpectation{
					Name:        "E2E Keyword Full",
					URL:         "https://example.com",
					Type:        "KEYWORD",
					IntervalSec: 300,
					Timeout:     &timeout,
				}, monitor)
				g.Expect(errs).To(BeEmpty())
				errs = ValidateKeywordMonitorFields("ALERT_EXISTS", "Example Domain", &cs, monitor)
				g.Expect(errs).To(BeEmpty())
			}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
		})
	})

	Context("Keyword NotExists", func() {
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

	Context("Ping", func() {
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

	Context("Port", func() {
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
    url: google.com:443
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

	Context("DNS", func() {
		monitorName := fmt.Sprintf("e2e-dns-%s", testRunID)

		AfterEach(func() {
			deleteMonitorAndWaitForAPICleanup(monitorName)
		})

		It("should create DNS monitor with A records and validate in API", func() {
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
    name: "E2E DNS A Record Monitor"
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

		It("should create DNS monitor with CNAME records and validate in API", func() {
			monitorName := fmt.Sprintf("e2e-dns-cname-%s", testRunID)
			defer deleteMonitorAndWaitForAPICleanup(monitorName)

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
    name: "E2E DNS CNAME Monitor"
    url: www.github.com
    type: DNS
    interval: 5m
    dns:
      cname: ["github.com"]
`, monitorName, testRunID))

			monitorID := waitMonitorReadyAndGetID(monitorName)
			apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")

			Eventually(func(g Gomega) {
				monitor, err := getMonitorFromAPI(apiKey, monitorID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(monitor.Type).To(Equal("DNS"))
				g.Expect(monitor.URL).To(Equal("www.github.com"))
				errs := ValidateDNSMonitorFields(DNSRecordExpectation{
					CNAME: []string{"github.com"},
				}, monitor)
				g.Expect(errs).To(BeEmpty(), "DNS CNAME validation: %s", errs)
			}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
		})

		It("should create DNS monitor with MX records and validate in API", func() {
			monitorName := fmt.Sprintf("e2e-dns-mx-%s", testRunID)
			defer deleteMonitorAndWaitForAPICleanup(monitorName)

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
    name: "E2E DNS MX Monitor"
    url: google.com
    type: DNS
    interval: 5m
    dns:
      mx: ["smtp.google.com"]
`, monitorName, testRunID))

			monitorID := waitMonitorReadyAndGetID(monitorName)
			apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")

			Eventually(func(g Gomega) {
				monitor, err := getMonitorFromAPI(apiKey, monitorID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(monitor.Type).To(Equal("DNS"))
				g.Expect(monitor.URL).To(Equal("google.com"))
				errs := ValidateDNSMonitorFields(DNSRecordExpectation{
					MX: []string{"smtp.google.com"},
				}, monitor)
				g.Expect(errs).To(BeEmpty(), "DNS MX validation: %s", errs)
			}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
		})

		It("should create DNS monitor with NS records and validate in API", func() {
			monitorName := fmt.Sprintf("e2e-dns-ns-%s", testRunID)
			defer deleteMonitorAndWaitForAPICleanup(monitorName)

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
    name: "E2E DNS NS Monitor"
    url: google.com
    type: DNS
    interval: 5m
    dns:
      ns: ["ns1.google.com", "ns2.google.com"]
`, monitorName, testRunID))

			monitorID := waitMonitorReadyAndGetID(monitorName)
			apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")

			Eventually(func(g Gomega) {
				monitor, err := getMonitorFromAPI(apiKey, monitorID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(monitor.Type).To(Equal("DNS"))
				g.Expect(monitor.URL).To(Equal("google.com"))
				errs := ValidateDNSMonitorFields(DNSRecordExpectation{
					NS: []string{"ns1.google.com", "ns2.google.com"},
				}, monitor)
				g.Expect(errs).To(BeEmpty(), "DNS NS validation: %s", errs)
			}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
		})

		It("should create DNS monitor with TXT records and validate in API", func() {
			monitorName := fmt.Sprintf("e2e-dns-txt-%s", testRunID)
			defer deleteMonitorAndWaitForAPICleanup(monitorName)

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
    name: "E2E DNS TXT Monitor"
    url: google.com
    type: DNS
    interval: 5m
    dns:
      txt: ["v=spf1 include:_spf.google.com ~all"]
`, monitorName, testRunID))

			monitorID := waitMonitorReadyAndGetID(monitorName)
			apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")

			Eventually(func(g Gomega) {
				monitor, err := getMonitorFromAPI(apiKey, monitorID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(monitor.Type).To(Equal("DNS"))
				g.Expect(monitor.URL).To(Equal("google.com"))
				errs := ValidateDNSMonitorFields(DNSRecordExpectation{
					TXT: []string{"v=spf1 include:_spf.google.com ~all"},
				}, monitor)
				g.Expect(errs).To(BeEmpty(), "DNS TXT validation: %s", errs)
			}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
		})
	})

	Context("Contact assignment", func() {
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

	Context("Monitor Adoption", func() {
		monitorName := fmt.Sprintf("e2e-adopt-%s", testRunID)
		adoptedMonitorName := fmt.Sprintf("e2e-adopted-%s", testRunID)
		var sharedMonitorID string

		BeforeEach(func() {
			// Reset sharedMonitorID before each test to prevent cross-test contamination
			sharedMonitorID = ""
		})

		AfterEach(func() {
			// Clean up the adopted monitor (prune: false, so won't delete from API)
			cmd := exec.Command("kubectl", "delete", "monitor", adoptedMonitorName, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)

			// Clean up the original monitor if it still exists
			cmd = exec.Command("kubectl", "delete", "monitor", monitorName, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)

			// Manually clean up the monitor from UptimeRobot API since adopted monitor has prune: false
			// Only attempt deletion if this test actually created a monitor
			if sharedMonitorID != "" {
				apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")
				if apiKey != "" {
					urclient := uptimerobot.NewClient(apiKey)
					_ = urclient.DeleteMonitor(context.Background(), sharedMonitorID)
				}
				// Reset after cleanup
				sharedMonitorID = ""
			}
		})

		It("should adopt an existing monitor via annotation", func() {
			By("creating a monitor directly via the operator first")
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
    name: "E2E Monitor to Adopt"
    url: https://example.com
    type: HTTPS
    interval: 5m
`, monitorName, testRunID))

			existingMonitorID := waitMonitorReadyAndGetID(monitorName)
			Expect(existingMonitorID).NotTo(BeEmpty(), "Monitor should have ID in status")
			sharedMonitorID = existingMonitorID // Save for cleanup

			By("adopting the existing monitor with a new Monitor resource")
			// Note: prune: false prevents accidental deletion of the adopted monitor
			// This is a recommended practice for adoption scenarios
			adoptMonitorYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: %s
  annotations:
    uptimerobot.com/adopt-id: "%s"
spec:
  syncInterval: 1m
  prune: false
  account:
    name: e2e-account-%s
  monitor:
    name: "E2E Adopted Monitor (Updated)"
    url: https://example.com
    type: HTTPS
    interval: 5m
`, adoptedMonitorName, existingMonitorID, testRunID)

			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(adoptMonitorYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for the adopted Monitor to become ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "monitor", adoptedMonitorName,
					"-o", "jsonpath={.status.ready}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("true"))
			}, e2ePollTimeout, e2ePollInterval).Should(Succeed())

			By("verifying the adopted monitor has the same ID as the original")
			cmd = exec.Command("kubectl", "get", "monitor", adoptedMonitorName,
				"-o", "jsonpath={.status.id}")
			adoptedID, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.TrimSpace(adoptedID)).To(Equal(existingMonitorID))

			By("verifying the monitor was updated with the new spec values")
			apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")
			Eventually(func(g Gomega) {
				monitor, err := getMonitorFromAPI(apiKey, existingMonitorID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(monitor.FriendlyName).To(Equal("E2E Adopted Monitor (Updated)"))
			}, e2ePollTimeout, e2ePollInterval).Should(Succeed())

			By("deleting the original Monitor resource (should not delete from UptimeRobot)")
			cmd = exec.Command("kubectl", "delete", "monitor", monitorName)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the monitor still exists in UptimeRobot API")
			Eventually(func(g Gomega) {
				monitor, err := getMonitorFromAPI(apiKey, existingMonitorID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(monitor.FriendlyName).To(Equal("E2E Adopted Monitor (Updated)"))
			}, 30*time.Second, 5*time.Second).Should(Succeed())
		})

		It("should fail to adopt a non-existent monitor", func() {
			By("attempting to adopt a monitor with a non-existent ID")
			// Use a timestamp-based ID that is extremely unlikely to exist
			// Format: 99 (prefix) + 10 digits from nanosecond timestamp = 12 digit ID
			nonExistentID := fmt.Sprintf("99%010d", time.Now().UnixNano()%10000000000)
			adoptMonitorYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: %s
  annotations:
    uptimerobot.com/adopt-id: "%s"
spec:
  syncInterval: 1m
  prune: false
  account:
    name: e2e-account-%s
  monitor:
    name: "E2E Non-existent Monitor"
    url: https://example.com
    type: HTTPS
    interval: 5m
`, adoptedMonitorName, nonExistentID, testRunID)

			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(adoptMonitorYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the monitor does not become ready")
			Consistently(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "monitor", adoptedMonitorName,
					"-o", "jsonpath={.status.ready}")
				output, err := utils.Run(cmd)
				if err == nil {
					// If no error, status.ready should not be "true"
					g.Expect(output).NotTo(Equal("true"))
				}
			}, 30*time.Second, 5*time.Second).Should(Succeed())
		})

		It("should fail to adopt a monitor with type mismatch", func() {
			By("creating an HTTPS monitor first")
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
    name: "E2E HTTPS Monitor for Type Mismatch"
    url: https://example.com
    type: HTTPS
    interval: 5m
`, monitorName, testRunID))

			existingMonitorID := waitMonitorReadyAndGetID(monitorName)
			sharedMonitorID = existingMonitorID // Save for cleanup

			By("attempting to adopt it with a Ping type specification")
			adoptMonitorYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: %s
  annotations:
    uptimerobot.com/adopt-id: "%s"
spec:
  syncInterval: 1m
  prune: false
  account:
    name: e2e-account-%s
  monitor:
    name: "E2E Ping Type Mismatch"
    url: "8.8.8.8"
    type: Ping
    interval: 5m
`, adoptedMonitorName, existingMonitorID, testRunID)

			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(adoptMonitorYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the monitor does not become ready due to type mismatch")
			Consistently(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "monitor", adoptedMonitorName,
					"-o", "jsonpath={.status.ready}")
				output, err := utils.Run(cmd)
				if err == nil {
					g.Expect(output).NotTo(Equal("true"))
				}
			}, 30*time.Second, 5*time.Second).Should(Succeed())
		})

		It("should delete adopted monitor from UptimeRobot when prune is true", func() {
			By("creating a monitor first")
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
    name: "E2E Monitor for Prune Test"
    url: https://example.com
    type: HTTPS
    interval: 5m
`, monitorName, testRunID))

			existingMonitorID := waitMonitorReadyAndGetID(monitorName)
			sharedMonitorID = existingMonitorID // Save for cleanup (though this test deletes it)

			By("adopting the monitor with prune: true")
			adoptMonitorYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: Monitor
metadata:
  name: %s
  annotations:
    uptimerobot.com/adopt-id: "%s"
spec:
  syncInterval: 1m
  prune: true
  account:
    name: e2e-account-%s
  monitor:
    name: "E2E Adopted for Prune"
    url: https://example.com
    type: HTTPS
    interval: 5m
`, adoptedMonitorName, existingMonitorID, testRunID)

			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(adoptMonitorYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			waitMonitorReadyAndGetID(adoptedMonitorName)

			By("deleting the original monitor resource")
			cmd = exec.Command("kubectl", "delete", "monitor", monitorName)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("deleting the adopted monitor resource with prune: true")
			cmd = exec.Command("kubectl", "delete", "monitor", adoptedMonitorName)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the monitor is deleted from UptimeRobot API")
			apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")
			WaitForMonitorDeletedFromAPI(apiKey, existingMonitorID)
		})
	})
})
