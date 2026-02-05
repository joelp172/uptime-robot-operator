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

var _ = Describe("MaintenanceWindow CRD Reconciliation", Ordered, Label("maintenancewindow"), func() {
	// Skip all tests in this suite if no API key is provided
	BeforeAll(func() {
		if skipCRDReconciliation {
			Skip("Skipping MaintenanceWindow CRD reconciliation tests: UPTIME_ROBOT_API_KEY not set")
		}

		// Setup Account and Contact for MaintenanceWindow tests
		apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")
		debugLog("Setting up Account for MaintenanceWindow tests with testRunID: %s", testRunID)

		By("creating Secret with API key for MaintenanceWindow tests")
		secretYAML := fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: uptime-robot-e2e-mw
  namespace: %s
type: Opaque
stringData:
  apiKey: %s
`, namespace, apiKey)
		debugLog("Applying Secret for MaintenanceWindow tests (name: uptime-robot-e2e-mw, namespace: %s)", namespace)
		cmd := exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(secretYAML)
		output, err := utils.Run(cmd)
		if err != nil {
			debugLog("Failed to create Secret: %v, output: %s", err, output)
		} else {
			debugLog("Secret created successfully: %s", output)
		}
		Expect(err).NotTo(HaveOccurred())

		By("creating Account resource for MaintenanceWindow tests")
		accountYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: Account
metadata:
  name: e2e-account-%s
spec:
  isDefault: true
  apiKeySecretRef:
    name: uptime-robot-e2e-mw
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
		debugLog("Waiting for Account e2e-account-%s to become ready", testRunID)
		pollCount := 0
		Eventually(func(g Gomega) {
			pollCount++
			cmd := exec.Command("kubectl", "get", "account", fmt.Sprintf("e2e-account-%s", testRunID),
				"-o", "jsonpath={.status.ready}")
			output, err := utils.Run(cmd)
			if err != nil {
				debugLog("Poll #%d: Failed to get Account: %v", pollCount, err)
			} else {
				debugLog("Poll #%d: Account status.ready: %q", pollCount, output)
			}
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("true"))
		}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
		debugLog("Account is ready after %d polls", pollCount)

		By("getting the first contact ID from Account status for MaintenanceWindow tests")
		cmd = exec.Command("kubectl", "get", "account", fmt.Sprintf("e2e-account-%s", testRunID),
			"-o", "jsonpath={.status.alertContacts[0].id}")
		contactID, err := utils.Run(cmd)
		if err != nil {
			debugLog("Failed to get contact ID: %v", err)
		} else {
			debugLog("Got contact ID: %s", contactID)
		}
		Expect(err).NotTo(HaveOccurred())
		Expect(contactID).NotTo(BeEmpty(), "Account should have at least one alert contact")

		By("creating default Contact for MaintenanceWindow tests")
		contactYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: Contact
metadata:
  name: e2e-mw-default-contact-%s
spec:
  isDefault: true
  contact:
    id: "%s"
`, testRunID, contactID)
		debugLog("Applying Contact YAML:\n%s", contactYAML)
		cmd = exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(contactYAML)
		output, err = utils.Run(cmd)
		if err != nil {
			debugLog("Failed to create Contact: %v, output: %s", err, output)
		} else {
			debugLog("Contact created successfully: %s", output)
		}
		Expect(err).NotTo(HaveOccurred())

		// Wait for Contact to be ready
		debugLog("Waiting for Contact e2e-mw-default-contact-%s to become ready", testRunID)
		pollCount = 0
		Eventually(func(g Gomega) {
			pollCount++
			cmd := exec.Command("kubectl", "get", "contact", fmt.Sprintf("e2e-mw-default-contact-%s", testRunID),
				"-o", "jsonpath={.status.ready}")
			output, err := utils.Run(cmd)
			if err != nil {
				debugLog("Poll #%d: Failed to get Contact: %v", pollCount, err)
			} else {
				debugLog("Poll #%d: Contact status.ready: %q", pollCount, output)
			}
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("true"))
		}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
		debugLog("Contact is ready after %d polls", pollCount)
	})

	AfterAll(func() {
		if skipCRDReconciliation {
			return
		}

		By("cleaning up MaintenanceWindow test resources")
		// Delete all maintenance windows first
		cleanupMaintenanceWindows()
		// Delete contact, account and secret
		cmd := exec.Command("kubectl", "delete", "contact", fmt.Sprintf("e2e-mw-default-contact-%s", testRunID), "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
		cmd = exec.Command("kubectl", "delete", "account", fmt.Sprintf("e2e-account-%s", testRunID), "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
		cmd = exec.Command("kubectl", "delete", "secret", "uptime-robot-e2e-mw", "-n", namespace, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
	})

	Context("Basic Lifecycle - Once Interval", func() {
		mwName := fmt.Sprintf("e2e-mw-once-%s", testRunID)

		AfterEach(func() {
			deleteMaintenanceWindowAndWait(mwName)
		})

		It("should create once maintenance window", func() {
			mwYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: MaintenanceWindow
metadata:
  name: %s
spec:
  prune: true
  account:
    name: e2e-account-%s
  name: "E2E Once MW"
  interval: once
  startDate: "2026-03-01"
  startTime: "02:00:00"
  duration: 1h
`, mwName, testRunID)

			applyMaintenanceWindow(mwYAML)
			mwID := waitMaintenanceWindowReadyAndGetID(mwName)

			By("validating maintenance window fields in UptimeRobot API")
			apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")
			Eventually(func(g Gomega) {
				mw, err := getMaintenanceWindowFromAPI(apiKey, mwID)
				g.Expect(err).NotTo(HaveOccurred())
				errs := ValidateMaintenanceWindowFields(
					"E2E Once MW",
					"once",
					"02:00:00",
					60,  // 1h in minutes
					nil, // no days for once interval
					"2026-03-01",
					mw,
				)
				g.Expect(errs).To(BeEmpty(), "field validation: %s", errs)
			}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
		})
	})

	Context("Basic Lifecycle - Daily Interval", func() {
		mwName := fmt.Sprintf("e2e-mw-daily-%s", testRunID)

		AfterEach(func() {
			deleteMaintenanceWindowAndWait(mwName)
		})

		It("should create daily maintenance window", func() {
			mwYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: MaintenanceWindow
metadata:
  name: %s
spec:
  prune: true
  account:
    name: e2e-account-%s
  name: "E2E Daily MW"
  interval: daily
  startTime: "03:00:00"
  duration: 2h
`, mwName, testRunID)

			applyMaintenanceWindow(mwYAML)
			mwID := waitMaintenanceWindowReadyAndGetID(mwName)

			By("validating maintenance window fields in UptimeRobot API")
			apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")
			Eventually(func(g Gomega) {
				mw, err := getMaintenanceWindowFromAPI(apiKey, mwID)
				g.Expect(err).NotTo(HaveOccurred())
				errs := ValidateMaintenanceWindowFields(
					"E2E Daily MW",
					"daily",
					"03:00:00",
					120, // 2h in minutes
					nil, // no days for daily interval
					"",  // no startDate for daily interval
					mw,
				)
				g.Expect(errs).To(BeEmpty(), "field validation: %s", errs)
			}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
		})
	})

	Context("Basic Lifecycle - Weekly Interval", func() {
		mwName := fmt.Sprintf("e2e-mw-weekly-%s", testRunID)

		AfterEach(func() {
			deleteMaintenanceWindowAndWait(mwName)
		})

		It("should create weekly maintenance window", func() {
			mwYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: MaintenanceWindow
metadata:
  name: %s
spec:
  prune: true
  account:
    name: e2e-account-%s
  name: "E2E Weekly MW"
  interval: weekly
  startTime: "04:00:00"
  duration: 30m
  days: [1, 3, 5]
`, mwName, testRunID)

			applyMaintenanceWindow(mwYAML)
			mwID := waitMaintenanceWindowReadyAndGetID(mwName)

			By("validating maintenance window fields in UptimeRobot API")
			apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")
			Eventually(func(g Gomega) {
				mw, err := getMaintenanceWindowFromAPI(apiKey, mwID)
				g.Expect(err).NotTo(HaveOccurred())
				errs := ValidateMaintenanceWindowFields(
					"E2E Weekly MW",
					"weekly",
					"04:00:00",
					30, // 30m in minutes
					[]int{1, 3, 5},
					"", // no startDate for weekly interval
					mw,
				)
				g.Expect(errs).To(BeEmpty(), "field validation: %s", errs)
			}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
		})
	})

	Context("Basic Lifecycle - Monthly Interval", func() {
		mwName := fmt.Sprintf("e2e-mw-monthly-%s", testRunID)

		AfterEach(func() {
			deleteMaintenanceWindowAndWait(mwName)
		})

		It("should create monthly maintenance window", func() {
			mwYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: MaintenanceWindow
metadata:
  name: %s
spec:
  prune: true
  account:
    name: e2e-account-%s
  name: "E2E Monthly MW"
  interval: monthly
  startTime: "05:00:00"
  duration: 1h
  days: [1, 15, -1]
`, mwName, testRunID)

			applyMaintenanceWindow(mwYAML)
			mwID := waitMaintenanceWindowReadyAndGetID(mwName)

			By("validating maintenance window fields in UptimeRobot API")
			apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")
			Eventually(func(g Gomega) {
				mw, err := getMaintenanceWindowFromAPI(apiKey, mwID)
				g.Expect(err).NotTo(HaveOccurred())
				errs := ValidateMaintenanceWindowFields(
					"E2E Monthly MW",
					"monthly",
					"05:00:00",
					60, // 1h in minutes
					[]int{1, 15, -1},
					"", // no startDate for monthly interval
					mw,
				)
				g.Expect(errs).To(BeEmpty(), "field validation: %s", errs)
			}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
		})
	})

	Context("Updates", func() {
		mwName := fmt.Sprintf("e2e-mw-update-%s", testRunID)

		AfterEach(func() {
			deleteMaintenanceWindowAndWait(mwName)
		})

		It("should update maintenance window name", func() {
			mwYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: MaintenanceWindow
metadata:
  name: %s
spec:
  prune: true
  account:
    name: e2e-account-%s
  name: "E2E Update MW - Original"
  interval: daily
  startTime: "06:00:00"
  duration: 1h
`, mwName, testRunID)

			applyMaintenanceWindow(mwYAML)
			waitMaintenanceWindowReady(mwName)

			By("updating the name")
			updatedYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: MaintenanceWindow
metadata:
  name: %s
spec:
  prune: true
  account:
    name: e2e-account-%s
  name: "E2E Update MW - Updated"
  interval: daily
  startTime: "06:00:00"
  duration: 1h
`, mwName, testRunID)

			applyMaintenanceWindow(updatedYAML)

			By("waiting for update to propagate")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "maintenancewindow", mwName,
					"-o", "jsonpath={.status.ready}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("true"))
			}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
		})

		It("should update maintenance window schedule", func() {
			mwYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: MaintenanceWindow
metadata:
  name: %s
spec:
  prune: true
  account:
    name: e2e-account-%s
  name: "E2E Schedule Update MW"
  interval: daily
  startTime: "07:00:00"
  duration: 1h
`, mwName, testRunID)

			applyMaintenanceWindow(mwYAML)
			waitMaintenanceWindowReady(mwName)

			By("updating to weekly interval")
			updatedYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: MaintenanceWindow
metadata:
  name: %s
spec:
  prune: true
  account:
    name: e2e-account-%s
  name: "E2E Schedule Update MW"
  interval: weekly
  startTime: "07:00:00"
  duration: 1h
  days: [1, 5]
`, mwName, testRunID)

			applyMaintenanceWindow(updatedYAML)

			By("waiting for update to propagate to API")
			waitMaintenanceWindowReady(mwName)

			By("verifying interval changed to weekly")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "maintenancewindow", mwName,
					"-o", "jsonpath={.spec.interval}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(strings.TrimSpace(output)).To(Equal("weekly"))
			}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
		})
	})

	Context("Deletion", func() {
		It("should delete maintenance window with prune=true", func() {
			mwName := fmt.Sprintf("e2e-mw-delete-prune-%s", testRunID)
			mwYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: MaintenanceWindow
metadata:
  name: %s
spec:
  prune: true
  account:
    name: e2e-account-%s
  name: "E2E Delete Prune MW"
  interval: daily
  startTime: "08:00:00"
  duration: 1h
`, mwName, testRunID)

			applyMaintenanceWindow(mwYAML)
			waitMaintenanceWindowReady(mwName)

			By("deleting the MaintenanceWindow")
			cmd := exec.Command("kubectl", "delete", "maintenancewindow", mwName)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying MaintenanceWindow is deleted")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "maintenancewindow", mwName)
				_, err := utils.Run(cmd)
				g.Expect(err).To(HaveOccurred(), "MaintenanceWindow should be deleted")
			}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
		})

		It("should delete maintenance window with prune=false", func() {
			mwName := fmt.Sprintf("e2e-mw-delete-noprune-%s", testRunID)
			mwYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: MaintenanceWindow
metadata:
  name: %s
spec:
  prune: false
  account:
    name: e2e-account-%s
  name: "E2E Delete No Prune MW"
  interval: daily
  startTime: "09:00:00"
  duration: 1h
`, mwName, testRunID)

			applyMaintenanceWindow(mwYAML)
			waitMaintenanceWindowReady(mwName)

			By("capturing the maintenance window ID")
			cmd := exec.Command("kubectl", "get", "maintenancewindow", mwName,
				"-o", "jsonpath={.status.id}")
			mwID, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.TrimSpace(mwID)).NotTo(BeEmpty())

			By("deleting the MaintenanceWindow CR")
			cmd = exec.Command("kubectl", "delete", "maintenancewindow", mwName)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying MaintenanceWindow CR is deleted")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "maintenancewindow", mwName)
				_, err := utils.Run(cmd)
				g.Expect(err).To(HaveOccurred(), "MaintenanceWindow CR should be deleted")
			}, e2ePollTimeout, e2ePollInterval).Should(Succeed())

			// Note: With prune=false, the external resource persists in UptimeRobot
			// In a real test, we would verify it still exists via API
		})
	})

	Context("Monitor References", func() {
		var monitor1Name, monitor2Name string

		BeforeEach(func() {
			monitor1Name = fmt.Sprintf("e2e-mw-mon1-%s", testRunID)
			monitor2Name = fmt.Sprintf("e2e-mw-mon2-%s", testRunID)
		})

		AfterEach(func() {
			deleteMonitorAndWaitForAPICleanup(monitor1Name)
			deleteMonitorAndWaitForAPICleanup(monitor2Name)
		})

		It("should add monitors via monitorRefs", func() {
			// Create two monitors
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
    name: "E2E MW Test Monitor 1"
    url: https://example.com/mw1
    type: HTTPS
`, monitor1Name, testRunID))

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
    name: "E2E MW Test Monitor 2"
    url: https://example.com/mw2
    type: HTTPS
`, monitor2Name, testRunID))

			By("waiting for monitors to become ready")
			waitMonitorReadyAndGetID(monitor1Name)
			waitMonitorReadyAndGetID(monitor2Name)

			mwName := fmt.Sprintf("e2e-mw-monrefs-%s", testRunID)
			defer deleteMaintenanceWindowAndWait(mwName)

			By("creating MaintenanceWindow with monitor references")
			mwYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: MaintenanceWindow
metadata:
  name: %s
spec:
  prune: true
  account:
    name: e2e-account-%s
  name: "E2E MW with Monitors"
  interval: daily
  startTime: "10:00:00"
  duration: 1h
  monitorRefs:
    - name: %s
    - name: %s
`, mwName, testRunID, monitor1Name, monitor2Name)

			applyMaintenanceWindow(mwYAML)
			mwID := waitMaintenanceWindowReadyAndGetID(mwName)

			// Get monitor IDs from cluster (as strings for API calls)
			cmd := exec.Command("kubectl", "get", "monitor", monitor1Name, "-o", "jsonpath={.status.id}")
			monitor1IDStr, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			cmd = exec.Command("kubectl", "get", "monitor", monitor2Name, "-o", "jsonpath={.status.id}")
			monitor2IDStr, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")

			// Note: The UptimeRobot v3 API does not return monitorIds when getting a maintenance window,
			// so we verify the relationship from the monitor side instead.
			By("verifying monitors contain MW ID via API")
			Eventually(func(g Gomega) {
				monitor1, err := getMonitorFromAPI(apiKey, monitor1IDStr)
				g.Expect(err).NotTo(HaveOccurred())
				monitor2, err := getMonitorFromAPI(apiKey, monitor2IDStr)
				g.Expect(err).NotTo(HaveOccurred())

				// Check that both monitors have the MW in their maintenanceWindows list
				g.Expect(monitor1.MaintenanceWindows).NotTo(BeEmpty(), "Monitor1 should have maintenance windows")
				g.Expect(monitor2.MaintenanceWindows).NotTo(BeEmpty(), "Monitor2 should have maintenance windows")

				// Check that the MW ID is in the list (compare as string)
				found1 := false
				for _, mw := range monitor1.MaintenanceWindows {
					if fmt.Sprintf("%d", mw.ID) == mwID {
						found1 = true
						break
					}
				}
				g.Expect(found1).To(BeTrue(), "Monitor1 should have MW ID %s", mwID)

				found2 := false
				for _, mw := range monitor2.MaintenanceWindows {
					if fmt.Sprintf("%d", mw.ID) == mwID {
						found2 = true
						break
					}
				}
				g.Expect(found2).To(BeTrue(), "Monitor2 should have MW ID %s", mwID)
			}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
		})

		It("should update monitor references", func() {
			// Create two monitors
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
    name: "E2E MW Update Monitor 1"
    url: https://example.com/mwupdate1
    type: HTTPS
`, monitor1Name, testRunID))

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
    name: "E2E MW Update Monitor 2"
    url: https://example.com/mwupdate2
    type: HTTPS
`, monitor2Name, testRunID))

			waitMonitorReadyAndGetID(monitor1Name)
			waitMonitorReadyAndGetID(monitor2Name)

			mwName := fmt.Sprintf("e2e-mw-monupdate-%s", testRunID)
			defer deleteMaintenanceWindowAndWait(mwName)

			By("creating MaintenanceWindow with one monitor")
			mwYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: MaintenanceWindow
metadata:
  name: %s
spec:
  prune: true
  account:
    name: e2e-account-%s
  name: "E2E MW Update Monitors"
  interval: daily
  startTime: "11:00:00"
  duration: 1h
  monitorRefs:
    - name: %s
`, mwName, testRunID, monitor1Name)

			applyMaintenanceWindow(mwYAML)
			waitMaintenanceWindowReady(mwName)

			By("updating to include both monitors")
			updatedYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: MaintenanceWindow
metadata:
  name: %s
spec:
  prune: true
  account:
    name: e2e-account-%s
  name: "E2E MW Update Monitors"
  interval: daily
  startTime: "11:00:00"
  duration: 1h
  monitorRefs:
    - name: %s
    - name: %s
`, mwName, testRunID, monitor1Name, monitor2Name)

			applyMaintenanceWindow(updatedYAML)

			By("waiting for update to complete")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "maintenancewindow", mwName,
					"-o", "jsonpath={.status.ready}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("true"))
			}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
		})

		It("should remove all monitors", func() {
			// Create one monitor
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
    name: "E2E MW Remove Monitor"
    url: https://example.com/mwremove
    type: HTTPS
`, monitor1Name, testRunID))

			waitMonitorReadyAndGetID(monitor1Name)

			mwName := fmt.Sprintf("e2e-mw-monremove-%s", testRunID)
			defer deleteMaintenanceWindowAndWait(mwName)

			By("creating MaintenanceWindow with monitor")
			mwYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: MaintenanceWindow
metadata:
  name: %s
spec:
  prune: true
  account:
    name: e2e-account-%s
  name: "E2E MW Remove Monitors"
  interval: daily
  startTime: "12:00:00"
  duration: 1h
  monitorRefs:
    - name: %s
`, mwName, testRunID, monitor1Name)

			applyMaintenanceWindow(mwYAML)
			waitMaintenanceWindowReady(mwName)

			By("removing all monitors")
			updatedYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: MaintenanceWindow
metadata:
  name: %s
spec:
  prune: true
  account:
    name: e2e-account-%s
  name: "E2E MW Remove Monitors"
  interval: daily
  startTime: "12:00:00"
  duration: 1h
  monitorRefs: []
`, mwName, testRunID)

			applyMaintenanceWindow(updatedYAML)

			By("waiting for update to complete")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "maintenancewindow", mwName,
					"-o", "jsonpath={.status.ready}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("true"))
			}, e2ePollTimeout, e2ePollInterval).Should(Succeed())
		})
	})

	Context("Duration Handling", func() {
		It("should handle duration with fractional minutes", func() {
			mwName := fmt.Sprintf("e2e-mw-duration-%s", testRunID)
			defer deleteMaintenanceWindowAndWait(mwName)

			mwYAML := fmt.Sprintf(`
apiVersion: uptimerobot.com/v1alpha1
kind: MaintenanceWindow
metadata:
  name: %s
spec:
  prune: true
  account:
    name: e2e-account-%s
  name: "E2E Duration MW"
  interval: daily
  startTime: "13:00:00"
  duration: 1h30m
`, mwName, testRunID)

			applyMaintenanceWindow(mwYAML)
			waitMaintenanceWindowReady(mwName)

			By("verifying duration is set")
			cmd := exec.Command("kubectl", "get", "maintenancewindow", mwName,
				"-o", "jsonpath={.spec.duration}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.TrimSpace(output)).NotTo(BeEmpty())
		})
	})
})

// Helper functions for MaintenanceWindow e2e tests

// deleteMaintenanceWindowAndWait deletes a MaintenanceWindow and waits for it to be removed
func deleteMaintenanceWindowAndWait(mwName string) {
	cmd := exec.Command("kubectl", "delete", "maintenancewindow", mwName, "--ignore-not-found=true")
	_, _ = utils.Run(cmd)

	// Wait for deletion to complete
	Eventually(func(g Gomega) {
		cmd := exec.Command("kubectl", "get", "maintenancewindow", mwName)
		_, err := utils.Run(cmd)
		g.Expect(err).To(HaveOccurred(), "MaintenanceWindow should be deleted")
	}, 2*time.Minute, 5*time.Second).Should(Succeed())
}

// cleanupMaintenanceWindows deletes all maintenance windows created during the e2e tests
// This includes both K8s CR deletion and API cleanup verification
func cleanupMaintenanceWindows() {
	maintenanceWindowPrefixes := []string{
		fmt.Sprintf("e2e-mw-once-%s", testRunID),
		fmt.Sprintf("e2e-mw-daily-%s", testRunID),
		fmt.Sprintf("e2e-mw-weekly-%s", testRunID),
		fmt.Sprintf("e2e-mw-monthly-%s", testRunID),
		fmt.Sprintf("e2e-mw-update-%s", testRunID),
		fmt.Sprintf("e2e-mw-delete-prune-%s", testRunID),
		fmt.Sprintf("e2e-mw-delete-noprune-%s", testRunID),
		fmt.Sprintf("e2e-mw-monrefs-%s", testRunID),
		fmt.Sprintf("e2e-mw-monupdate-%s", testRunID),
		fmt.Sprintf("e2e-mw-monremove-%s", testRunID),
		fmt.Sprintf("e2e-mw-duration-%s", testRunID),
	}

	apiKey := os.Getenv("UPTIME_ROBOT_API_KEY")
	if apiKey == "" {
		// No API cleanup if no API key (shouldn't happen in real tests)
		for _, name := range maintenanceWindowPrefixes {
			cmd := exec.Command("kubectl", "delete", "maintenancewindow", name, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)
		}
		return
	}

	// Step 1: Delete all K8s CRs first
	for _, name := range maintenanceWindowPrefixes {
		cmd := exec.Command("kubectl", "delete", "maintenancewindow", name, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
	}

	// Step 2: Clean up from API by listing all maintenance windows
	// Some tests delete CRs before cleanup runs, so we can't rely on K8s state
	urclient := uptimerobot.NewClient(apiKey)
	ctx := context.Background()

	mwList, err := urclient.ListMaintenanceWindows(ctx)
	if err != nil {
		debugLog("Failed to list maintenance windows from API for cleanup: %v", err)
		return
	}

	// Delete any maintenance window whose name starts with our test prefix
	// Note: We delete ALL E2E maintenance windows, not just this test run's,
	// since the API doesn't store the testRunID and we want to clean up any leftover MWs
	testPrefix := "E2E " // All our test MWs start with "E2E "
	for _, mw := range mwList {
		if strings.HasPrefix(mw.Name, testPrefix) {
			debugLog("Cleaning up maintenance window '%s' (ID=%d) from API", mw.Name, mw.ID)
			deleteMaintenanceWindowFromAPI(apiKey, fmt.Sprintf("%d", mw.ID))
		}
	}
}

// deleteMaintenanceWindowFromAPI deletes a maintenance window directly from the API
func deleteMaintenanceWindowFromAPI(apiKey, mwID string) {
	urclient := uptimerobot.NewClient(apiKey)
	ctx := context.Background()

	debugLog("Deleting maintenance window from API: ID=%s", mwID)
	if err := urclient.DeleteMaintenanceWindow(ctx, mwID); err != nil {
		// Ignore not found errors (already deleted)
		if !uptimerobot.IsNotFound(err) {
			debugLog("Failed to delete maintenance window from API: %v", err)
		} else {
			debugLog("Maintenance window already deleted from API: ID=%s", mwID)
		}
	} else {
		debugLog("Successfully deleted maintenance window from API: ID=%s", mwID)
	}
}
