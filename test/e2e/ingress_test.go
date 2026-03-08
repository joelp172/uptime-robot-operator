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

var _ = Describe("Ingress Controller", Ordered, Label("integration", "ingress"), func() {
	accountName := sharedAccountName
	ingressName := fmt.Sprintf("e2e-ingress-%s", testRunID)
	monitorName := ingressName
	monitorFriendlyName := fmt.Sprintf("E2E Ingress Monitor (%s)", testRunID)
	var apiKey string
	var monitorID string
	makeIngressName := func(suffix string) string {
		return fmt.Sprintf("e2e-ingress-%s-%s", suffix, testRunID)
	}
	waitMonitorReadyAndGetID := func(name string) string {
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "monitor", name, "-n", "default", "-o", "jsonpath={.status.ready}")
			ready, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(ready).To(Equal("true"))
		}, 3*time.Minute, 5*time.Second).Should(Succeed())
		cmd := exec.Command("kubectl", "get", "monitor", name, "-n", "default", "-o", "jsonpath={.status.id}")
		id, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		return strings.TrimSpace(id)
	}
	deleteIngressAndMonitor := func(name string) {
		cmd := exec.Command("kubectl", "delete", "ingress", name, "-n", "default", "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
		cmd = exec.Command("kubectl", "delete", "monitor", name, "-n", "default", "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
	}

	BeforeAll(func() {
		if skipCRDReconciliation {
			Skip("Skipping Ingress e2e tests: UPTIME_ROBOT_API_KEY not set")
		}

		apiKey = os.Getenv("UPTIME_ROBOT_API_KEY")
		Expect(apiKey).NotTo(BeEmpty(), "UPTIME_ROBOT_API_KEY must be set for ingress e2e tests")

		ensureE2EInfra()
		ensureSharedAccountAndContact()
	})

	AfterAll(func() {
		By("cleaning up ingress test resources")
		cmd := exec.Command("kubectl", "delete", "ingress", ingressName, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
		cmd = exec.Command("kubectl", "delete", "monitor", monitorName, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)

		if monitorID != "" {
			WaitForMonitorDeletedFromAPI(apiKey, monitorID)
		}
	})

	It("recreates a monitor when manually deleted while ingress remains", func() {
		By("creating ingress with uptime robot annotations")
		ingressYAML := fmt.Sprintf(`
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: %s
  namespace: default
  annotations:
    uptimerobot.com/enabled: "true"
    uptimerobot.com/monitor.name: %q
    uptimerobot.com/account.name: %s
spec:
  rules:
  - host: example.com
    http:
      paths:
      - path: /healthz
        pathType: Prefix
        backend:
          service:
            name: kubernetes
            port:
              number: 443
`, ingressName, monitorFriendlyName, accountName)
		out, err := applyYAMLWithWebhookRetry("Ingress", ingressYAML)
		Expect(err).NotTo(HaveOccurred(), "Failed to apply ingress: %s", out)

		By("waiting for monitor to be created and ready")
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "monitor", monitorName, "-n", "default", "-o", "jsonpath={.spec.monitor.status}")
			status, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(status).To(Equal("1"))

			cmd = exec.Command("kubectl", "get", "monitor", monitorName, "-n", "default", "-o", "jsonpath={.spec.prune}")
			prune, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(prune).To(Equal("true"))

			cmd = exec.Command("kubectl", "get", "monitor", monitorName, "-n", "default", "-o", "jsonpath={.spec.sourceRef.kind}")
			sourceKind, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(sourceKind).To(Equal("Ingress"))

			cmd = exec.Command("kubectl", "get", "monitor", monitorName, "-n", "default", "-o", "jsonpath={.status.ready}")
			ready, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(ready).To(Equal("true"))
		}, 3*time.Minute, 5*time.Second).Should(Succeed())

		cmd := exec.Command("kubectl", "get", "monitor", monitorName, "-n", "default", "-o", "jsonpath={.status.id}")
		firstID, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.TrimSpace(firstID)).NotTo(BeEmpty())

		By("deleting the monitor directly")
		cmd = exec.Command("kubectl", "delete", "monitor", monitorName, "-n", "default")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("waiting for ingress controller to recreate the monitor")
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "monitor", monitorName, "-n", "default", "-o", "jsonpath={.status.ready}")
			ready, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(ready).To(Equal("true"))
		}, 3*time.Minute, 5*time.Second).Should(Succeed())

		cmd = exec.Command("kubectl", "get", "monitor", monitorName, "-n", "default", "-o", "jsonpath={.status.id}")
		secondID, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		monitorID = strings.TrimSpace(secondID)
		Expect(monitorID).NotTo(BeEmpty())
	})

	It("deletes monitor in Kubernetes and UptimeRobot when ingress is deleted", func() {
		By("capturing current monitor id")
		cmd := exec.Command("kubectl", "get", "monitor", monitorName, "-n", "default", "-o", "jsonpath={.status.id}")
		id, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		monitorID = strings.TrimSpace(id)
		Expect(monitorID).NotTo(BeEmpty())

		By("deleting ingress")
		cmd = exec.Command("kubectl", "delete", "ingress", ingressName, "-n", "default")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("waiting for monitor resource to be deleted")
		Eventually(func() bool {
			cmd := exec.Command("kubectl", "get", "monitor", monitorName, "-n", "default")
			_, err := utils.Run(cmd)
			return err != nil
		}, 3*time.Minute, 5*time.Second).Should(BeTrue())

		By("waiting for monitor to be deleted from UptimeRobot API")
		WaitForMonitorDeletedFromAPI(apiKey, monitorID)
		monitorID = ""
	})

	It("derives monitor URL from ingress rules when monitor.url is not set", func() {
		name := makeIngressName("derive")
		var id string
		defer func() {
			deleteIngressAndMonitor(name)
			if id != "" {
				WaitForMonitorDeletedFromAPI(apiKey, id)
			}
		}()

		ingressYAML := fmt.Sprintf(`
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: %s
  namespace: default
  annotations:
    uptimerobot.com/enabled: "true"
    uptimerobot.com/account.name: %s
spec:
  rules:
  - host: example.com
    http:
      paths:
      - path: /healthz
        pathType: Prefix
        backend:
          service:
            name: kubernetes
            port:
              number: 443
`, name, accountName)
		out, err := applyYAMLWithWebhookRetry("Ingress", ingressYAML)
		Expect(err).NotTo(HaveOccurred(), "Failed to apply ingress: %s", out)

		id = waitMonitorReadyAndGetID(name)
		Eventually(func(g Gomega) {
			monitor, err := getMonitorFromAPI(apiKey, id)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(monitor.URL).To(Equal("http://example.com/healthz"))
		}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
	})

	It("uses explicit monitor.url annotation instead of derived URL", func() {
		name := makeIngressName("url-override")
		var id string
		defer func() {
			deleteIngressAndMonitor(name)
			if id != "" {
				WaitForMonitorDeletedFromAPI(apiKey, id)
			}
		}()
		const explicitURL = "https://custom.example.com/check"

		ingressYAML := fmt.Sprintf(`
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: %s
  namespace: default
  annotations:
    uptimerobot.com/enabled: "true"
    uptimerobot.com/account.name: %s
    uptimerobot.com/monitor.url: %q
spec:
  rules:
  - host: example.com
    http:
      paths:
      - path: /ignored
        pathType: Prefix
        backend:
          service:
            name: kubernetes
            port:
              number: 443
`, name, accountName, explicitURL)
		out, err := applyYAMLWithWebhookRetry("Ingress", ingressYAML)
		Expect(err).NotTo(HaveOccurred(), "Failed to apply ingress: %s", out)

		id = waitMonitorReadyAndGetID(name)
		Eventually(func(g Gomega) {
			monitor, err := getMonitorFromAPI(apiKey, id)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(monitor.URL).To(Equal(explicitURL))
		}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
	})

	It("uses https scheme when ingress has TLS configuration", func() {
		name := makeIngressName("tls")
		var id string
		defer func() {
			deleteIngressAndMonitor(name)
			if id != "" {
				WaitForMonitorDeletedFromAPI(apiKey, id)
			}
		}()

		ingressYAML := fmt.Sprintf(`
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: %s
  namespace: default
  annotations:
    uptimerobot.com/enabled: "true"
    uptimerobot.com/account.name: %s
spec:
  tls:
  - hosts:
    - secure.example.com
    secretName: tls-secret
  rules:
  - host: secure.example.com
    http:
      paths:
      - path: /secure
        pathType: Prefix
        backend:
          service:
            name: kubernetes
            port:
              number: 443
`, name, accountName)
		out, err := applyYAMLWithWebhookRetry("Ingress", ingressYAML)
		Expect(err).NotTo(HaveOccurred(), "Failed to apply ingress: %s", out)

		id = waitMonitorReadyAndGetID(name)
		Eventually(func(g Gomega) {
			monitor, err := getMonitorFromAPI(apiKey, id)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(monitor.URL).To(HavePrefix("https://"))
			g.Expect(monitor.URL).To(Equal("https://secure.example.com/secure"))
		}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
	})

	It("propagates monitor.name annotation updates to UptimeRobot", func() {
		name := makeIngressName("name-update")
		var id string
		defer func() {
			deleteIngressAndMonitor(name)
			if id != "" {
				WaitForMonitorDeletedFromAPI(apiKey, id)
			}
		}()

		initialName := fmt.Sprintf("Ingress Initial (%s)", testRunID)
		updatedName := fmt.Sprintf("Ingress Updated (%s)", testRunID)
		ingressYAML := fmt.Sprintf(`
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: %s
  namespace: default
  annotations:
    uptimerobot.com/enabled: "true"
    uptimerobot.com/account.name: %s
    uptimerobot.com/monitor.name: %q
spec:
  rules:
  - host: name-update.example.com
    http:
      paths:
      - path: /status
        pathType: Prefix
        backend:
          service:
            name: kubernetes
            port:
              number: 443
`, name, accountName, initialName)
		out, err := applyYAMLWithWebhookRetry("Ingress", ingressYAML)
		Expect(err).NotTo(HaveOccurred(), "Failed to apply ingress: %s", out)

		id = waitMonitorReadyAndGetID(name)
		cmd := exec.Command("kubectl", "annotate", "ingress", name, "-n", "default",
			fmt.Sprintf("uptimerobot.com/monitor.name=%s", updatedName), "--overwrite")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) {
			monitor, err := getMonitorFromAPI(apiKey, id)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(monitor.FriendlyName).To(Equal(updatedName))
		}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
	})

	It("deletes monitor when enabled is toggled to false", func() {
		name := makeIngressName("disable")
		defer deleteIngressAndMonitor(name)

		ingressYAML := fmt.Sprintf(`
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: %s
  namespace: default
  annotations:
    uptimerobot.com/enabled: "true"
    uptimerobot.com/account.name: %s
spec:
  rules:
  - host: disable.example.com
    http:
      paths:
      - path: /health
        pathType: Prefix
        backend:
          service:
            name: kubernetes
            port:
              number: 443
`, name, accountName)
		out, err := applyYAMLWithWebhookRetry("Ingress", ingressYAML)
		Expect(err).NotTo(HaveOccurred(), "Failed to apply ingress: %s", out)

		id := waitMonitorReadyAndGetID(name)
		cmd := exec.Command("kubectl", "annotate", "ingress", name, "-n", "default",
			"uptimerobot.com/enabled=false", "--overwrite")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() bool {
			cmd := exec.Command("kubectl", "get", "monitor", name, "-n", "default")
			_, err := utils.Run(cmd)
			return err != nil
		}, 3*time.Minute, 5*time.Second).Should(BeTrue())
		WaitForMonitorDeletedFromAPI(apiKey, id)
	})

	It("uses custom scheme host and path annotations when provided", func() {
		name := makeIngressName("custom-url")
		var id string
		defer func() {
			deleteIngressAndMonitor(name)
			if id != "" {
				WaitForMonitorDeletedFromAPI(apiKey, id)
			}
		}()
		const customURL = "https://custom.example.net/custom/path"

		ingressYAML := fmt.Sprintf(`
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: %s
  namespace: default
  annotations:
    uptimerobot.com/enabled: "true"
    uptimerobot.com/account.name: %s
    uptimerobot.com/monitor.scheme: "https"
    uptimerobot.com/monitor.host: "custom.example.net"
    uptimerobot.com/monitor.path: "/custom/path"
spec:
  rules:
  - host: ignored.example.com
    http:
      paths:
      - path: /ignored
        pathType: Prefix
        backend:
          service:
            name: kubernetes
            port:
              number: 443
`, name, accountName)
		out, err := applyYAMLWithWebhookRetry("Ingress", ingressYAML)
		Expect(err).NotTo(HaveOccurred(), "Failed to apply ingress: %s", out)

		id = waitMonitorReadyAndGetID(name)
		Eventually(func(g Gomega) {
			monitor, err := getMonitorFromAPI(apiKey, id)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(monitor.URL).To(Equal(customURL))
		}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
	})

	It("omits root path from derived URL", func() {
		name := makeIngressName("root-path")
		var id string
		defer func() {
			deleteIngressAndMonitor(name)
			if id != "" {
				WaitForMonitorDeletedFromAPI(apiKey, id)
			}
		}()

		ingressYAML := fmt.Sprintf(`
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: %s
  namespace: default
  annotations:
    uptimerobot.com/enabled: "true"
    uptimerobot.com/account.name: %s
spec:
  rules:
  - host: root.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: kubernetes
            port:
              number: 443
`, name, accountName)
		out, err := applyYAMLWithWebhookRetry("Ingress", ingressYAML)
		Expect(err).NotTo(HaveOccurred(), "Failed to apply ingress: %s", out)

		id = waitMonitorReadyAndGetID(name)
		Eventually(func(g Gomega) {
			monitor, err := getMonitorFromAPI(apiKey, id)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(monitor.URL).To(Equal("http://root.example.com"))
		}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
	})

	It("ignores ingress resources without uptime robot annotations", func() {
		name := makeIngressName("no-anno")
		defer deleteIngressAndMonitor(name)

		ingressYAML := fmt.Sprintf(`
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: %s
  namespace: default
spec:
  rules:
  - host: no-anno.example.com
    http:
      paths:
      - path: /x
        pathType: Prefix
        backend:
          service:
            name: kubernetes
            port:
              number: 443
`, name)
		out, err := applyYAMLWithWebhookRetry("Ingress", ingressYAML)
		Expect(err).NotTo(HaveOccurred(), "Failed to apply ingress: %s", out)

		Consistently(func() bool {
			cmd := exec.Command("kubectl", "get", "monitor", name, "-n", "default")
			_, err := utils.Run(cmd)
			return err != nil
		}, 20*time.Second, 5*time.Second).Should(BeTrue())
	})
})
