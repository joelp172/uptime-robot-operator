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
	"os/exec"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/joelp172/uptime-robot-operator/test/utils"
)

// projectImage is the name of the image which will be build and loaded
// with the code source changes to be tested.
var projectImage = "example.com/uptime-robot-operator:v0.0.1"

// TestE2E runs the end-to-end (e2e) test suite for the project. These tests execute in an isolated,
// temporary environment to validate project changes for use in CI jobs.
// The default setup requires Kind and builds/loads the Manager Docker image locally.
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting uptime-robot-operator integration test suite\n")
	RunSpecs(t, "e2e suite")
}

var _ = BeforeSuite(func() {
	By("building the manager(Operator) image")
	cmd := exec.Command("make", "docker-build", fmt.Sprintf("IMG=%s", projectImage))
	_, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the manager(Operator) image")

	// TODO(user): If you want to change the e2e test vendor from Kind, ensure the image is
	// built and available before running the tests. Also, remove the following block.

	// Check if deployment exists BEFORE loading new image
	// If it exists, we need to restart it after loading to pick up the new image
	checkCmd := exec.Command("kubectl", "get", "deployment", "uptime-robot-controller-manager", "-n", "uptime-robot-system")
	deploymentExisted := false
	if _, err := utils.Run(checkCmd); err == nil {
		deploymentExisted = true
	}

	By("loading the manager(Operator) image on Kind")
	err = utils.LoadImageToKindClusterWithName(projectImage)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load the manager(Operator) image into Kind")

	// Only restart if deployment existed before we loaded the new image
	// This ensures the new image is picked up, but doesn't restart unnecessarily on fresh deployments
	if deploymentExisted {
		By("restarting controller deployment to pick up new image")
		cmd = exec.Command("kubectl", "rollout", "restart", "deployment/uptime-robot-controller-manager", "-n", "uptime-robot-system")
		_, err = utils.Run(cmd)
		ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to restart controller deployment")
		cmd = exec.Command("kubectl", "rollout", "status", "deployment/uptime-robot-controller-manager", "-n", "uptime-robot-system", "--timeout=2m")
		_, err = utils.Run(cmd)
		ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Controller deployment failed to become ready after restart")
		waitForWebhookEndpointReady()
	}
})

var _ = AfterSuite(func() {
	// No cleanup needed
})

func waitForWebhookEndpointReady() {
	By("waiting for webhook endpoint to be ready")
	Eventually(func() string {
		cmd := exec.Command("kubectl", "get", "endpoints", "uptime-robot-webhook-service", "-n", "uptime-robot-system",
			"-o", "jsonpath={.subsets[0].addresses[0].ip}")
		output, err := utils.Run(cmd)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(output)
	}, 3*time.Minute, 5*time.Second).ShouldNot(BeEmpty(), "webhook endpoint was not ready in time")
}
