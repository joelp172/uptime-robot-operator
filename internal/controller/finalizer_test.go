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

package controller

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	uptimerobotv1 "github.com/joelp172/uptime-robot-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
)

var _ = Describe("Finalizer Cleanup", func() {
	var (
		ctx         context.Context
		testMonitor *uptimerobotv1.Monitor
		recorder    *record.FakeRecorder
	)

	BeforeEach(func() {
		ctx = context.Background()
		recorder = record.NewFakeRecorder(100)
		testMonitor = &uptimerobotv1.Monitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("test-monitor-%d", time.Now().UnixNano()),
				Namespace: "default",
			},
			Spec: uptimerobotv1.MonitorSpec{
				Account: corev1.LocalObjectReference{
					Name: "test-account",
				},
				Monitor: uptimerobotv1.MonitorValues{
					Name: "Test Monitor",
					URL:  "https://example.com",
				},
			},
			Status: uptimerobotv1.MonitorStatus{
				ID:    "12345",
				Ready: true,
			},
		}
		testMonitor.Status.ObservedGeneration = testMonitor.Generation
		testMonitor.Status.Conditions = []metav1.Condition{}
	})

	Describe("HandleFinalizerCleanup", func() {
		Context("when cleanup succeeds immediately", func() {
			It("should return success and record success event", func() {
				cleanupCalled := false
				cleanupFunc := func(ctx context.Context) error {
					cleanupCalled = true
					return nil
				}

				result, err := HandleFinalizerCleanup(ctx, CleanupOptions{
					Object:             testMonitor,
					Conditions:         &testMonitor.Status.Conditions,
					ObservedGeneration: testMonitor.Generation,
					Recorder:           recorder,
					CleanupFunc:        cleanupFunc,
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(cleanupCalled).To(BeTrue())
				Expect(result.Success).To(BeTrue())
				Expect(result.ForceRemove).To(BeFalse())

				// Check that Deleting condition was set to success
				deletingCond := findCondition(testMonitor.Status.Conditions, TypeDeleting)
				Expect(deletingCond).NotTo(BeNil())
				Expect(deletingCond.Status).To(Equal(metav1.ConditionTrue))
				Expect(deletingCond.Reason).To(Equal(ReasonCleanupSuccess))

				// Check that success event was recorded
				Eventually(recorder.Events).Should(Receive(ContainSubstring("CleanupSuccess")))
			})
		})

		Context("when cleanup fails", func() {
			It("should return error and schedule retry", func() {
				cleanupError := errors.New("API unavailable")
				cleanupFunc := func(ctx context.Context) error {
					return cleanupError
				}

				// Initialize annotations with start time to test retry logic
				testMonitor.Annotations = map[string]string{
					CleanupStartTimeAnnotation: time.Now().Add(-30 * time.Second).Format(time.RFC3339),
				}

				result, err := HandleFinalizerCleanup(ctx, CleanupOptions{
					Object:             testMonitor,
					Conditions:         &testMonitor.Status.Conditions,
					ObservedGeneration: testMonitor.Generation,
					Recorder:           recorder,
					CleanupFunc:        cleanupFunc,
				})

				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(cleanupError))
				Expect(result.Success).To(BeFalse())
				Expect(result.ForceRemove).To(BeFalse())
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))

				// Check that error condition was set
				deletingCond := findCondition(testMonitor.Status.Conditions, TypeDeleting)
				Expect(deletingCond).NotTo(BeNil())
				Expect(deletingCond.Status).To(Equal(metav1.ConditionTrue))
				Expect(deletingCond.Reason).To(Equal(ReasonCleanupError))
				Expect(deletingCond.Message).To(ContainSubstring("API unavailable"))

				// Check that error event was recorded
				Eventually(recorder.Events).Should(Receive(ContainSubstring("CleanupError")))
			})
		})

		Context("when cleanup times out", func() {
			It("should force-remove finalizer after timeout", func() {
				cleanupFunc := func(ctx context.Context) error {
					return errors.New("API still unavailable")
				}

				// Set start time to be past the timeout
				testMonitor.Annotations = map[string]string{
					CleanupStartTimeAnnotation: time.Now().Add(-15 * time.Minute).Format(time.RFC3339),
				}

				result, err := HandleFinalizerCleanup(ctx, CleanupOptions{
					Object:             testMonitor,
					Conditions:         &testMonitor.Status.Conditions,
					ObservedGeneration: testMonitor.Generation,
					Recorder:           recorder,
					CleanupTimeout:     10 * time.Minute,
					CleanupFunc:        cleanupFunc,
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Success).To(BeFalse())
				Expect(result.ForceRemove).To(BeTrue())

				// Check that timeout condition was set
				deletingCond := findCondition(testMonitor.Status.Conditions, TypeDeleting)
				Expect(deletingCond).NotTo(BeNil())
				Expect(deletingCond.Status).To(Equal(metav1.ConditionTrue))
				Expect(deletingCond.Reason).To(Equal(ReasonCleanupTimeout))
				Expect(deletingCond.Message).To(ContainSubstring("timed out"))

				// Check that timeout warning event was recorded
				Eventually(recorder.Events).Should(Receive(ContainSubstring("CleanupTimeout")))
			})
		})

		Context("when skip-cleanup annotation is set", func() {
			It("should skip cleanup and force-remove finalizer", func() {
				cleanupCalled := false
				cleanupFunc := func(ctx context.Context) error {
					cleanupCalled = true
					return nil
				}

				testMonitor.Annotations = map[string]string{
					SkipCleanupAnnotation: "true",
				}

				result, err := HandleFinalizerCleanup(ctx, CleanupOptions{
					Object:             testMonitor,
					Conditions:         &testMonitor.Status.Conditions,
					ObservedGeneration: testMonitor.Generation,
					Recorder:           recorder,
					CleanupFunc:        cleanupFunc,
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(cleanupCalled).To(BeFalse())
				Expect(result.Success).To(BeTrue())
				Expect(result.ForceRemove).To(BeTrue())

				// Check that skipped condition was set
				deletingCond := findCondition(testMonitor.Status.Conditions, TypeDeleting)
				Expect(deletingCond).NotTo(BeNil())
				Expect(deletingCond.Status).To(Equal(metav1.ConditionTrue))
				Expect(deletingCond.Reason).To(Equal(ReasonCleanupSkipped))

				// Check that skipped event was recorded
				Eventually(recorder.Events).Should(Receive(ContainSubstring("CleanupSkipped")))
			})
		})

		Context("when skip-cleanup annotation is set to false", func() {
			It("should proceed with normal cleanup", func() {
				cleanupCalled := false
				cleanupFunc := func(ctx context.Context) error {
					cleanupCalled = true
					return nil
				}

				testMonitor.Annotations = map[string]string{
					SkipCleanupAnnotation: "false",
				}

				result, err := HandleFinalizerCleanup(ctx, CleanupOptions{
					Object:             testMonitor,
					Conditions:         &testMonitor.Status.Conditions,
					ObservedGeneration: testMonitor.Generation,
					Recorder:           recorder,
					CleanupFunc:        cleanupFunc,
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(cleanupCalled).To(BeTrue())
				Expect(result.Success).To(BeTrue())
				Expect(result.ForceRemove).To(BeFalse())

				// Check that success condition was set (not skipped)
				deletingCond := findCondition(testMonitor.Status.Conditions, TypeDeleting)
				Expect(deletingCond).NotTo(BeNil())
				Expect(deletingCond.Reason).To(Equal(ReasonCleanupSuccess))
			})
		})

		Context("with custom timeout", func() {
			It("should respect custom timeout", func() {
				cleanupFunc := func(ctx context.Context) error {
					return errors.New("API unavailable")
				}

				// Set start time to be past the custom timeout but not default
				testMonitor.Annotations = map[string]string{
					CleanupStartTimeAnnotation: time.Now().Add(-3 * time.Minute).Format(time.RFC3339),
				}

				result, err := HandleFinalizerCleanup(ctx, CleanupOptions{
					Object:             testMonitor,
					Conditions:         &testMonitor.Status.Conditions,
					ObservedGeneration: testMonitor.Generation,
					Recorder:           recorder,
					CleanupTimeout:     2 * time.Minute, // Custom shorter timeout
					CleanupFunc:        cleanupFunc,
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Success).To(BeFalse())
				Expect(result.ForceRemove).To(BeTrue())

				// Check that timeout condition was set
				deletingCond := findCondition(testMonitor.Status.Conditions, TypeDeleting)
				Expect(deletingCond).NotTo(BeNil())
				Expect(deletingCond.Reason).To(Equal(ReasonCleanupTimeout))
			})
		})
	})

	Describe("InitializeCleanupTracking", func() {
		It("should set cleanup start time annotation if not present", func() {
			// This test requires a real Kubernetes client
			// which is set up by the test suite
			Skip("Integration test - requires full test suite")
		})

		It("should not overwrite existing cleanup start time", func() {
			// This test requires a real Kubernetes client
			// which is set up by the test suite
			Skip("Integration test - requires full test suite")
		})
	})
})

func TestFinalizerCleanupUnit(t *testing.T) {
	// Unit tests that don't require Kubernetes client
	t.Run("CleanupResult backoff calculation", func(t *testing.T) {
		ctx := context.Background()
		recorder := record.NewFakeRecorder(10)

		testMonitor := &uptimerobotv1.Monitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "default",
				Annotations: map[string]string{
					CleanupStartTimeAnnotation: time.Now().Add(-1 * time.Minute).Format(time.RFC3339),
				},
			},
		}
		testMonitor.Status.Conditions = []metav1.Condition{}

		result, _ := HandleFinalizerCleanup(ctx, CleanupOptions{
			Object:             testMonitor,
			Conditions:         &testMonitor.Status.Conditions,
			ObservedGeneration: 1,
			Recorder:           recorder,
			CleanupTimeout:     10 * time.Minute,
			CleanupFunc: func(ctx context.Context) error {
				return errors.New("temporary error")
			},
		})

		// Verify backoff is reasonable (should be between 30s and 5m)
		if result.RequeueAfter < 30*time.Second || result.RequeueAfter > 5*time.Minute {
			t.Errorf("Expected backoff between 30s and 5m, got %v", result.RequeueAfter)
		}
	})

	t.Run("CleanupResult handles nil recorder", func(t *testing.T) {
		ctx := context.Background()

		testMonitor := &uptimerobotv1.Monitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "default",
			},
		}
		testMonitor.Status.Conditions = []metav1.Condition{}

		// Should not panic with nil recorder
		result, err := HandleFinalizerCleanup(ctx, CleanupOptions{
			Object:             testMonitor,
			Conditions:         &testMonitor.Status.Conditions,
			ObservedGeneration: 1,
			Recorder:           nil, // nil recorder
			CleanupFunc: func(ctx context.Context) error {
				return nil
			},
		})

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !result.Success {
			t.Errorf("Expected success")
		}
	})
}
