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
	"errors"
	"fmt"
	"io"
	"net/http"
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
	defaultAPIURL   = "https://api.uptimerobot.com/v3"
)

// testRunID is a unique identifier for this test run to avoid conflicts
var testRunID = fmt.Sprintf("e2e-%d", time.Now().Unix())

// skipCRDReconciliation determines if CRD reconciliation tests should be skipped
// Tests require UPTIME_ROBOT_API_KEY environment variable to be set
var skipCRDReconciliation = os.Getenv("UPTIME_ROBOT_API_KEY") == ""

// Common helper functions for monitor tests

// applyMonitor applies a monitor YAML manifest via kubectl
func applyMonitor(monitorYAML string) {
	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(monitorYAML)
	output, err := utils.Run(cmd)
	debugLog("Monitor apply output: %s", output)
	Expect(err).NotTo(HaveOccurred())
}

// waitMonitorReadyAndGetID waits for a monitor to become ready and returns its ID
func waitMonitorReadyAndGetID(monitorName string) string {
	By("waiting for Monitor to become ready")
	Eventually(func(g Gomega) {
		cmd := exec.Command("kubectl", "get", "monitor", monitorName, "-o", "jsonpath={.status.ready}")
		output, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(output).To(Equal("true"))
	}, e2ePollTimeout, e2ePollInterval).Should(Succeed())

	cmd := exec.Command("kubectl", "get", "monitor", monitorName, "-o", "jsonpath={.status.id}")
	monitorID, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
	return strings.TrimSpace(monitorID)
}

// waitForAccountReady waits for an account to report status.ready=true and emits
// reconciliation diagnostics when the timeout is reached.
func waitForAccountReady(accountName string) {
	By(fmt.Sprintf("waiting for Account %s to become ready", accountName))
	deadline := time.Now().Add(e2ePollTimeout)
	lastReady := ""
	lastObservedGeneration := ""
	lastGeneration := ""
	lastConditionSummary := ""

	for time.Now().Before(deadline) {
		cmd := exec.Command("kubectl", "get", "account", accountName, "-o", "jsonpath={.status.ready}")
		ready, err := utils.Run(cmd)
		if err == nil {
			lastReady = strings.TrimSpace(ready)
		}

		cmd = exec.Command("kubectl", "get", "account", accountName, "-o", "jsonpath={.status.observedGeneration}")
		observedGeneration, err := utils.Run(cmd)
		if err == nil {
			lastObservedGeneration = strings.TrimSpace(observedGeneration)
		}

		cmd = exec.Command("kubectl", "get", "account", accountName, "-o", "jsonpath={.metadata.generation}")
		generation, err := utils.Run(cmd)
		if err == nil {
			lastGeneration = strings.TrimSpace(generation)
		}

		cmd = exec.Command("kubectl", "get", "account", accountName,
			"-o", "jsonpath={range .status.conditions[*]}{.type}={.status}:{.reason}:{.message}{\"\\n\"}{end}")
		conditionSummary, err := utils.Run(cmd)
		if err == nil {
			lastConditionSummary = strings.TrimSpace(conditionSummary)
		}

		if lastReady == "true" {
			return
		}

		time.Sleep(e2ePollInterval)
	}

	By("collecting Account reconciliation diagnostics")
	cmd := exec.Command("kubectl", "get", "account", accountName, "-o", "yaml")
	if out, err := utils.Run(cmd); err == nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "Account YAML:\n%s\n", out)
	}

	cmd = exec.Command("kubectl", "describe", "account", accountName)
	if out, err := utils.Run(cmd); err == nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "Account describe:\n%s\n", out)
	}

	cmd = exec.Command("kubectl", "get", "events", "-A",
		"--field-selector", fmt.Sprintf("involvedObject.kind=Account,involvedObject.name=%s", accountName),
		"--sort-by=.lastTimestamp")
	if out, err := utils.Run(cmd); err == nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "Account events:\n%s\n", out)
	}

	cmd = exec.Command("kubectl", "get", "pods", "-n", namespace, "-l", "control-plane=controller-manager",
		"-o", "jsonpath={.items[0].metadata.name}")
	podName, err := utils.Run(cmd)
	if err == nil && strings.TrimSpace(podName) != "" {
		cmd = exec.Command("kubectl", "logs", strings.TrimSpace(podName), "-n", namespace, "--tail=300")
		if out, logsErr := utils.Run(cmd); logsErr == nil {
			_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n%s\n", out)
		}
	}

	Fail(fmt.Sprintf("Account %s did not become ready within %s (ready=%q observedGeneration=%q generation=%q conditions=%q)",
		accountName, e2ePollTimeout, lastReady, lastObservedGeneration, lastGeneration, lastConditionSummary))
}

// deleteMonitorAndWaitForAPICleanup deletes a monitor CR and waits for it to be removed from the API
func deleteMonitorAndWaitForAPICleanup(monitorName string) {
	// Try to get the monitor ID first - if the resource doesn't exist, skip cleanup
	cmd := exec.Command("kubectl", "get", "monitor", monitorName, "-o", "jsonpath={.status.id}")
	monitorID, err := utils.Run(cmd)
	if err != nil {
		// Monitor resource doesn't exist, nothing to clean up
		return
	}

	cmd = exec.Command("kubectl", "delete", "monitor", monitorName, "--ignore-not-found=true")
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())

	apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")
	WaitForMonitorDeletedFromAPI(apiKey, monitorID)
}

// deleteMonitorGroupAndWaitForAPICleanup deletes a MonitorGroup and waits for it to be removed from the API.
func deleteMonitorGroupAndWaitForAPICleanup(monitorGroupName, namespace string) {
	// Try to get the monitor group ID first - if the resource doesn't exist, skip cleanup
	cmd := exec.Command("kubectl", "get", "monitorgroup", monitorGroupName, "-n", namespace, "-o", "jsonpath={.status.id}")
	groupID, err := utils.Run(cmd)
	if err != nil {
		// MonitorGroup resource doesn't exist, nothing to clean up
		return
	}

	cmd = exec.Command("kubectl", "delete", "monitorgroup", monitorGroupName, "-n", namespace, "--ignore-not-found=true")
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())

	apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")
	WaitForMonitorGroupDeletedFromAPI(apiKey, groupID)
}

// WaitForMonitorGroupDeletedFromAPI waits for a monitor group to be removed from the UptimeRobot API.
func WaitForMonitorGroupDeletedFromAPI(apiKey, groupID string) {
	if apiKey == "" || groupID == "" {
		debugLog("Skipping monitor group API deletion wait: apiKey or groupID is empty")
		return
	}
	By("waiting for monitor group to be removed from UptimeRobot API")
	debugLog("Polling for monitor group deletion from API: ID=%s", groupID)
	Eventually(func(g Gomega) {
		_, err := getMonitorGroupFromAPI(apiKey, groupID)
		g.Expect(err).To(HaveOccurred())
		// Check for 404 or not found error
		g.Expect(err.Error()).To(ContainSubstring("404"))
	}, 90*time.Second, 5*time.Second).Should(Succeed())
	debugLog("Monitor group successfully deleted from API: ID=%s", groupID)
}

// getMonitorGroupFromAPI retrieves a monitor group from the UptimeRobot API.
func getMonitorGroupFromAPI(apiKey, groupID string) (map[string]interface{}, error) {
	if apiKey == "" {
		return nil, errors.New("API key is empty")
	}

	apiURL := os.Getenv("UPTIME_ROBOT_API")
	if apiURL == "" {
		apiURL = defaultAPIURL
	}

	endpoint := fmt.Sprintf("%s/monitor-groups/%s", apiURL, groupID)
	debugLog("Calling GetMonitorGroup for ID=%s", groupID)
	debugLog("Using API endpoint: %s", apiURL)

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		debugLog("GetMonitorGroup failed: monitor group not found")
		return nil, errors.New("monitor group not found: 404")
	}

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		debugLog("GetMonitorGroup failed: %s - %s", resp.Status, string(body))
		return nil, fmt.Errorf("API error: %s - %s", resp.Status, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	debugLog("GetMonitorGroup succeeded: %+v", result)
	return result, nil
}

// cleanupMonitors deletes all test monitors
func cleanupMonitors() {
	monitorNames := []string{
		fmt.Sprintf("e2e-http-%s", testRunID),
		fmt.Sprintf("e2e-https-full-%s", testRunID),
		fmt.Sprintf("e2e-region-%s", testRunID),
		fmt.Sprintf("e2e-https-auth-%s", testRunID),
		fmt.Sprintf("e2e-https-post-%s", testRunID),
		fmt.Sprintf("e2e-keyword-%s", testRunID),
		fmt.Sprintf("e2e-keyword-notexists-%s", testRunID),
		fmt.Sprintf("e2e-ping-%s", testRunID),
		fmt.Sprintf("e2e-port-%s", testRunID),
		fmt.Sprintf("e2e-heartbeat-%s", testRunID),
		fmt.Sprintf("e2e-heartbeat-secret-defaults-%s", testRunID),
		fmt.Sprintf("e2e-heartbeat-secret-update-%s", testRunID),
		fmt.Sprintf("e2e-heartbeat-configmap-update-%s", testRunID),
		fmt.Sprintf("e2e-http-no-heartbeat-publish-%s", testRunID),
		fmt.Sprintf("e2e-dns-%s", testRunID),
		fmt.Sprintf("e2e-dns-cname-%s", testRunID),
		fmt.Sprintf("e2e-dns-mx-%s", testRunID),
		fmt.Sprintf("e2e-dns-ns-%s", testRunID),
		fmt.Sprintf("e2e-dns-txt-%s", testRunID),
		fmt.Sprintf("e2e-contacts-%s", testRunID),
		fmt.Sprintf("e2e-dup-base-%s", testRunID),
		fmt.Sprintf("e2e-dup-attempt-%s", testRunID),
		fmt.Sprintf("e2e-adopt-%s", testRunID),
		fmt.Sprintf("e2e-adopted-%s", testRunID),
	}

	for _, name := range monitorNames {
		cmd := exec.Command("kubectl", "delete", "monitor", name, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
	}
}

// Common helper functions for maintenance window tests

// applyMaintenanceWindow applies a maintenance window YAML manifest via kubectl
func applyMaintenanceWindow(mwYAML string) {
	debugLog("Applying MaintenanceWindow YAML:\n%s", mwYAML)
	output, err := applyYAMLWithWebhookRetry("MaintenanceWindow", mwYAML)
	debugLog("MaintenanceWindow apply output: %s", output)
	Expect(err).NotTo(HaveOccurred())
}

// applyYAMLWithWebhookRetry applies YAML via kubectl and retries transient webhook call failures.
func applyYAMLWithWebhookRetry(resourceKind, yaml string) (string, error) {
	deadline := time.Now().Add(60 * time.Second)
	attempt := 1

	for {
		cmd := exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(yaml)
		output, err := utils.Run(cmd)
		if err == nil {
			return output, nil
		}

		if !isRetryableWebhookError(err.Error()) || time.Now().After(deadline) {
			return output, err
		}

		debugLog("%s apply attempt %d failed with retryable webhook error: %v", resourceKind, attempt, err)
		attempt++
		time.Sleep(3 * time.Second)
	}
}

func isRetryableWebhookError(errMsg string) bool {
	if !strings.Contains(errMsg, "failed calling webhook") {
		return false
	}

	return strings.Contains(errMsg, "context deadline exceeded") ||
		strings.Contains(errMsg, "i/o timeout") ||
		strings.Contains(errMsg, "no endpoints available")
}

// waitMaintenanceWindowReady waits for a maintenance window to become ready
func waitMaintenanceWindowReady(mwName string) {
	By(fmt.Sprintf("waiting for MaintenanceWindow %s to become ready", mwName))
	debugLog("Polling for MaintenanceWindow readiness: %s", mwName)
	pollCount := 0
	Eventually(func(g Gomega) {
		pollCount++
		cmd := exec.Command("kubectl", "get", "maintenancewindow", mwName, "-o", "jsonpath={.status.ready}")
		output, err := utils.Run(cmd)
		if err != nil {
			debugLog("Poll #%d: MaintenanceWindow %s error: %v", pollCount, mwName, err)
		} else {
			debugLog("Poll #%d: MaintenanceWindow %s status.ready: %q", pollCount, mwName, output)
		}
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(output).To(Equal("true"))
	}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
	debugLog("MaintenanceWindow %s is ready after %d polls", mwName, pollCount)
}

// waitMaintenanceWindowReadyAndGetID waits for a maintenance window to become ready and returns its ID
func waitMaintenanceWindowReadyAndGetID(mwName string) string {
	waitMaintenanceWindowReady(mwName)
	cmd := exec.Command("kubectl", "get", "maintenancewindow", mwName, "-o", "jsonpath={.status.id}")
	mwID, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
	return strings.TrimSpace(mwID)
}
