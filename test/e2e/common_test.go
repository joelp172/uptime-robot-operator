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

// cleanupMonitors deletes all test monitors
func cleanupMonitors() {
	monitorNames := []string{
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

	for _, name := range monitorNames {
		cmd := exec.Command("kubectl", "delete", "monitor", name, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
	}
}

// Common helper functions for maintenance window tests

// applyMaintenanceWindow applies a maintenance window YAML manifest via kubectl
func applyMaintenanceWindow(mwYAML string) {
	debugLog("Applying MaintenanceWindow YAML:\n%s", mwYAML)
	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(mwYAML)
	output, err := utils.Run(cmd)
	debugLog("MaintenanceWindow apply output: %s", output)
	Expect(err).NotTo(HaveOccurred())
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
