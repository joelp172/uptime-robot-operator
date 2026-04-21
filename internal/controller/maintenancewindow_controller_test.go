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
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	uptimerobotv1 "github.com/joelp172/uptime-robot-operator/api/v1alpha1"
)

const (
	// testTimeout is the default timeout for Eventually checks in tests
	testTimeout = 5 * time.Second
)

type errorReader struct {
	err error
}

func (e *errorReader) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return e.err
}

func (e *errorReader) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return e.err
}

// conflictInjectingClient wraps a client.Client and returns a 409 Conflict error
// on the first Status().Update call that matches a predicate, then delegates
// subsequent calls to the underlying client. Used to simulate the
// resourceVersion race that updateMaintenanceWindowStatus is designed to
// recover from.
type conflictInjectingClient struct {
	client.Client
	triggered bool
	shouldFn  func(obj client.Object) bool
}

func (c *conflictInjectingClient) Status() client.SubResourceWriter {
	return &conflictInjectingStatusWriter{parent: c, inner: c.Client.Status()}
}

type conflictInjectingStatusWriter struct {
	parent *conflictInjectingClient
	inner  client.SubResourceWriter
}

func (w *conflictInjectingStatusWriter) Create(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
	return w.inner.Create(ctx, obj, subResource, opts...)
}

func (w *conflictInjectingStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	if !w.parent.triggered && w.parent.shouldFn != nil && w.parent.shouldFn(obj) {
		w.parent.triggered = true
		return errors.NewConflict(schema.GroupResource{Group: "uptimerobot.com", Resource: "maintenancewindows"}, obj.GetName(), fmt.Errorf("injected conflict"))
	}
	return w.inner.Update(ctx, obj, opts...)
}

func (w *conflictInjectingStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	return w.inner.Patch(ctx, obj, patch, opts...)
}

var _ = Describe("MaintenanceWindow Controller", func() {
	Context("Basic Reconciliation", func() {
		var (
			ctx     context.Context
			secret  *corev1.Secret
			account *uptimerobotv1.Account
		)

		BeforeEach(func() {
			ctx = context.Background()
			account, secret = CreateAccount(ctx)
			ReconcileAccount(ctx, account)
		})

		AfterEach(func() {
			CleanupAccount(ctx, account, secret)
		})

		It("should create maintenance window successfully", func() {
			mw := CreateMaintenanceWindow(ctx, "test-create-mw", account.Name, uptimerobotv1.MaintenanceWindowSpec{
				Name:            "Test Create MW",
				Interval:        "daily",
				StartTime:       "02:00:00",
				Duration:        metav1.Duration{Duration: time.Hour},
				AutoAddMonitors: true,
			})
			defer CleanupMaintenanceWindow(ctx, mw)

			By("Reconciling the created resource")
			result, err := ReconcileMaintenanceWindow(ctx, mw)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			By("Checking if finalizer is added")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, mw)
			Expect(err).NotTo(HaveOccurred())
			Expect(mw.Finalizers).To(ContainElement("uptimerobot.com/finalizer"))

			By("Checking if MaintenanceWindow status was updated")
			Expect(mw.Status.Ready).To(BeTrue())
			Expect(mw.Status.ID).NotTo(BeEmpty())

			ready := findCondition(mw.Status.Conditions, TypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionTrue))
			Expect(ready.Reason).To(Equal(ReasonReconcileSuccess))

			synced := findCondition(mw.Status.Conditions, TypeSynced)
			Expect(synced).NotTo(BeNil())
			Expect(synced.Status).To(Equal(metav1.ConditionTrue))
			Expect(synced.Reason).To(Equal(ReasonSyncSuccess))

			errCond := findCondition(mw.Status.Conditions, TypeError)
			Expect(errCond).NotTo(BeNil())
			Expect(errCond.Status).To(Equal(metav1.ConditionFalse))
		})

		It("should flag Ready/Synced/Error when circuit breaker blocks and recover when it closes", func() {
			mw := CreateMaintenanceWindow(ctx, "test-circuit-breaker-mw", account.Name, uptimerobotv1.MaintenanceWindowSpec{
				Name:      "Circuit Breaker MW",
				Interval:  "daily",
				StartTime: "02:00:00",
				Duration:  metav1.Duration{Duration: time.Hour},
			})
			defer CleanupMaintenanceWindow(ctx, mw)

			recorder := record.NewFakeRecorder(10)
			reconciler := &MaintenanceWindowReconciler{
				Client:    k8sClient,
				APIReader: k8sClient,
				Scheme:    k8sClient.Scheme(),
				Recorder:  recorder,
			}
			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}}

			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			origBreaker := DefaultCircuitBreaker
			const cooldown = 10 * time.Second
			DefaultCircuitBreaker = NewCircuitBreaker(1, cooldown)
			DeferCleanup(func() {
				DefaultCircuitBreaker = origBreaker
			})

			accountKey := account.Name
			Expect(DefaultCircuitBreaker.RecordFailure(accountKey)).To(BeTrue())
			reason := ReasonCircuitBreakerOpen
			eventReason := ReasonCircuitBreakerOpen
			msg := fmt.Sprintf("Circuit breaker open for account %s; reconciliation paused for %s", accountKey, cooldown)

			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(cooldown))

			Expect(k8sClient.Get(ctx, req.NamespacedName, mw)).To(Succeed())
			ready := findCondition(mw.Status.Conditions, TypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(reason))
			Expect(ready.Message).To(Equal(msg))
			firstReadyTransition := ready.LastTransitionTime

			synced := findCondition(mw.Status.Conditions, TypeSynced)
			Expect(synced).NotTo(BeNil())
			Expect(synced.Status).To(Equal(metav1.ConditionFalse))
			Expect(synced.Reason).To(Equal(reason))
			Expect(synced.Message).To(Equal(msg))
			firstSyncedTransition := synced.LastTransitionTime

			errCond := findCondition(mw.Status.Conditions, TypeError)
			Expect(errCond).NotTo(BeNil())
			Expect(errCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(errCond.Reason).To(Equal(reason))
			Expect(errCond.Message).To(Equal(msg))
			firstErrorTransition := errCond.LastTransitionTime

			Eventually(recorder.Events, 2*time.Second, 50*time.Millisecond).Should(Receive(
				And(ContainSubstring(eventReason), ContainSubstring(accountKey)),
			))

			result, err = reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(cooldown))
			Expect(k8sClient.Get(ctx, req.NamespacedName, mw)).To(Succeed())
			ready = findCondition(mw.Status.Conditions, TypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.LastTransitionTime).To(Equal(firstReadyTransition))
			synced = findCondition(mw.Status.Conditions, TypeSynced)
			Expect(synced).NotTo(BeNil())
			Expect(synced.LastTransitionTime).To(Equal(firstSyncedTransition))
			errCond = findCondition(mw.Status.Conditions, TypeError)
			Expect(errCond).NotTo(BeNil())
			Expect(errCond.LastTransitionTime).To(Equal(firstErrorTransition))
			Consistently(recorder.Events, 200*time.Millisecond, 50*time.Millisecond).ShouldNot(Receive())

			Expect(DefaultCircuitBreaker.RecordSuccess(accountKey)).To(BeTrue())
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Get(ctx, req.NamespacedName, mw)).To(Succeed())

			ready = findCondition(mw.Status.Conditions, TypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionTrue))
			Expect(ready.Reason).To(Equal(ReasonReconcileSuccess))

			synced = findCondition(mw.Status.Conditions, TypeSynced)
			Expect(synced).NotTo(BeNil())
			Expect(synced.Status).To(Equal(metav1.ConditionTrue))
			Expect(synced.Reason).To(Equal(ReasonSyncSuccess))

			errCond = findCondition(mw.Status.Conditions, TypeError)
			Expect(errCond).NotTo(BeNil())
			Expect(errCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(errCond.Reason).To(Equal(ReasonReconcileSuccess))
		})

		It("should update maintenance window successfully", func() {
			mw := CreateMaintenanceWindow(ctx, "test-update-mw", account.Name, uptimerobotv1.MaintenanceWindowSpec{
				Name:      "Test Update MW",
				Interval:  "daily",
				StartTime: "02:00:00",
				Duration:  metav1.Duration{Duration: time.Hour},
			})
			defer CleanupMaintenanceWindow(ctx, mw)

			// Create it first
			_, err := ReconcileMaintenanceWindow(ctx, mw)
			Expect(err).NotTo(HaveOccurred())

			// Update the name
			err = k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, mw)
			Expect(err).NotTo(HaveOccurred())
			mw.Spec.Name = "Updated MW Name"
			Expect(k8sClient.Update(ctx, mw)).To(Succeed())

			// Reconcile again
			result, err := ReconcileMaintenanceWindow(ctx, mw)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			// Verify it's still ready
			err = k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, mw)
			Expect(err).NotTo(HaveOccurred())
			Expect(mw.Status.Ready).To(BeTrue())
		})

		It("should recover from status update conflict", func() {
			mw := CreateMaintenanceWindow(ctx, "test-status-conflict-mw", account.Name, uptimerobotv1.MaintenanceWindowSpec{
				Name:      "Status Conflict MW",
				Interval:  "daily",
				StartTime: "02:00:00",
				Duration:  metav1.Duration{Duration: time.Hour},
			})
			defer CleanupMaintenanceWindow(ctx, mw)

			reconciler := &MaintenanceWindowReconciler{
				Client:    k8sClient,
				APIReader: k8sClient,
				Scheme:    k8sClient.Scheme(),
				Recorder:  record.NewFakeRecorder(100),
			}

			stale := &uptimerobotv1.MaintenanceWindow{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, stale)).To(Succeed())

			latest := &uptimerobotv1.MaintenanceWindow{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, latest)).To(Succeed())
			latest.Status.Ready = true
			latest.Status.ID = "11111"
			Expect(k8sClient.Status().Update(ctx, latest)).To(Succeed())

			stale.Status.Ready = true
			stale.Status.ID = "22222"
			stale.Status.MonitorCount = 3
			stale.Status.ObservedGeneration = stale.Generation
			Expect(reconciler.updateMaintenanceWindowStatus(ctx, stale)).To(Succeed())

			updated := &uptimerobotv1.MaintenanceWindow{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, updated)).To(Succeed())
			Expect(updated.Status.ID).To(Equal("22222"))
			Expect(updated.Status.MonitorCount).To(Equal(3))
			Expect(updated.Status.Ready).To(BeTrue())
		})

		It("should align observedGeneration and conditions when retrying conflicted status updates", func() {
			mw := CreateMaintenanceWindow(ctx, "test-status-conflict-conditions-mw", account.Name, uptimerobotv1.MaintenanceWindowSpec{
				Name:      "Status Conflict Conditions MW",
				Interval:  "daily",
				StartTime: "02:00:00",
				Duration:  metav1.Duration{Duration: time.Hour},
			})
			defer CleanupMaintenanceWindow(ctx, mw)

			stale := &uptimerobotv1.MaintenanceWindow{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, stale)).To(Succeed())

			latest := &uptimerobotv1.MaintenanceWindow{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, latest)).To(Succeed())
			latest.Status.Ready = true
			latest.Status.ID = "current-id"
			latest.Status.ObservedGeneration = 0
			latest.Status.Conditions = []metav1.Condition{
				{
					Type:               TypeReady,
					Status:             metav1.ConditionTrue,
					Reason:             ReasonReconcileSuccess,
					ObservedGeneration: 0,
					LastTransitionTime: metav1.Now(),
				},
			}
			Expect(k8sClient.Status().Update(ctx, latest)).To(Succeed())

			stale.Status.Ready = true
			stale.Status.ID = "new-id"
			stale.Status.MonitorCount = 5
			stale.Status.ObservedGeneration = stale.Generation
			stale.Status.Conditions = []metav1.Condition{
				{
					Type:               TypeReady,
					Status:             metav1.ConditionTrue,
					Reason:             ReasonReconcileSuccess,
					ObservedGeneration: stale.Generation,
					LastTransitionTime: metav1.Now(),
				},
			}

			reconciler := &MaintenanceWindowReconciler{
				Client:    k8sClient,
				APIReader: k8sClient,
				Scheme:    k8sClient.Scheme(),
			}
			Expect(reconciler.updateMaintenanceWindowStatus(ctx, stale)).To(Succeed())

			updated := &uptimerobotv1.MaintenanceWindow{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, updated)).To(Succeed())
			Expect(updated.Status.ID).To(Equal("new-id"))
			Expect(updated.Status.MonitorCount).To(Equal(5))
			Expect(updated.Status.ObservedGeneration).To(Equal(stale.Generation))

			ready := findCondition(updated.Status.Conditions, TypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.ObservedGeneration).To(Equal(stale.Generation))
		})

		It("should abort status retry when spec generation changed under us", func() {
			mw := CreateMaintenanceWindow(ctx, "test-status-conflict-genchange-mw", account.Name, uptimerobotv1.MaintenanceWindowSpec{
				Name:      "Status Conflict Gen Change MW",
				Interval:  "daily",
				StartTime: "02:00:00",
				Duration:  metav1.Duration{Duration: time.Hour},
			})
			defer CleanupMaintenanceWindow(ctx, mw)

			stale := &uptimerobotv1.MaintenanceWindow{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, stale)).To(Succeed())
			reconciledGeneration := stale.Generation

			bumped := &uptimerobotv1.MaintenanceWindow{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, bumped)).To(Succeed())
			bumped.Spec.Name = "spec-changed-mid-reconcile"
			Expect(k8sClient.Update(ctx, bumped)).To(Succeed())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, bumped)).To(Succeed())
			bumped.Status.ID = "backend-id"
			Expect(k8sClient.Status().Update(ctx, bumped)).To(Succeed())

			stale.Status.ID = "stale-write"
			stale.Status.Ready = true

			reconciler := &MaintenanceWindowReconciler{
				Client:    k8sClient,
				APIReader: k8sClient,
				Scheme:    k8sClient.Scheme(),
			}
			err := reconciler.updateMaintenanceWindowStatus(ctx, stale)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec changed during status retry"))
			Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("%d -> %d", reconciledGeneration, bumped.Generation)))

			final := &uptimerobotv1.MaintenanceWindow{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, final)).To(Succeed())
			Expect(final.Status.ID).To(Equal("backend-id"))
		})

		It("should surface refetch errors when retrying conflicted status updates", func() {
			mw := CreateMaintenanceWindow(ctx, "test-status-conflict-refetch-mw", account.Name, uptimerobotv1.MaintenanceWindowSpec{
				Name:      "Status Conflict Refetch MW",
				Interval:  "daily",
				StartTime: "02:00:00",
				Duration:  metav1.Duration{Duration: time.Hour},
			})
			defer CleanupMaintenanceWindow(ctx, mw)

			stale := &uptimerobotv1.MaintenanceWindow{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, stale)).To(Succeed())

			latest := &uptimerobotv1.MaintenanceWindow{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, latest)).To(Succeed())
			latest.Status.Ready = true
			latest.Status.ID = "existing-id"
			Expect(k8sClient.Status().Update(ctx, latest)).To(Succeed())

			stale.Status.Ready = true
			stale.Status.ID = "newer-id"

			reconciler := &MaintenanceWindowReconciler{
				Client:    k8sClient,
				APIReader: &errorReader{err: fmt.Errorf("refetch failure")},
				Scheme:    k8sClient.Scheme(),
			}

			err := reconciler.updateMaintenanceWindowStatus(ctx, stale)
			Expect(err).To(MatchError(ContainSubstring("refetch for status retry")))
			Expect(err).To(MatchError(ContainSubstring("refetch failure")))
		})

		It("should not re-create backend window when Status.ID exists even if Status.Ready is false", func() {
			mw := CreateMaintenanceWindow(ctx, "test-no-duplicate-create-mw", account.Name, uptimerobotv1.MaintenanceWindowSpec{
				Name:      "No Duplicate Create MW",
				Interval:  "daily",
				StartTime: "02:00:00",
				Duration:  metav1.Duration{Duration: time.Hour},
			})
			defer CleanupMaintenanceWindow(ctx, mw)

			_, err := ReconcileMaintenanceWindow(ctx, mw)
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, mw)).To(Succeed())
			createdID := mw.Status.ID
			Expect(createdID).NotTo(BeEmpty())

			mw.Status.Ready = false
			Expect(k8sClient.Status().Update(ctx, mw)).To(Succeed())

			recorder := record.NewFakeRecorder(100)
			reconciler := &MaintenanceWindowReconciler{
				Client:    k8sClient,
				APIReader: k8sClient,
				Scheme:    k8sClient.Scheme(),
				Recorder:  recorder,
			}
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

		drain:
			for {
				select {
				case event := <-recorder.Events:
					Expect(event).NotTo(ContainSubstring("Created"))
				default:
					break drain
				}
			}

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, mw)).To(Succeed())
			Expect(mw.Status.ID).To(Equal(createdID))
		})

		It("should not re-create backend window when status write loses a resourceVersion race", func() {
			mw := CreateMaintenanceWindow(ctx, "test-race-no-duplicate-mw", account.Name, uptimerobotv1.MaintenanceWindowSpec{
				Name:      "Race No Duplicate MW",
				Interval:  "daily",
				StartTime: "02:00:00",
				Duration:  metav1.Duration{Duration: time.Hour},
			})
			defer CleanupMaintenanceWindow(ctx, mw)

			wrapped := &conflictInjectingClient{
				Client: k8sClient,
				shouldFn: func(obj client.Object) bool {
					got, ok := obj.(*uptimerobotv1.MaintenanceWindow)
					return ok && got.Name == mw.Name && got.Status.ID != ""
				},
			}

			recorder := record.NewFakeRecorder(100)
			reconciler := &MaintenanceWindowReconciler{
				Client:    wrapped,
				APIReader: k8sClient,
				Scheme:    k8sClient.Scheme(),
				Recorder:  recorder,
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(wrapped.triggered).To(BeTrue(), "expected the injected conflict to fire on the first post-create status write")

			persisted := &uptimerobotv1.MaintenanceWindow{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, persisted)).To(Succeed())
			Expect(persisted.Status.ID).NotTo(BeEmpty(), "status.ID must be persisted despite the conflict so a second reconcile does not re-create")
			firstID := persisted.Status.ID

			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, persisted)).To(Succeed())
			Expect(persisted.Status.ID).To(Equal(firstID), "second reconcile must not replace the backend ID")

			createCount := 0
		drain:
			for {
				select {
				case event := <-recorder.Events:
					if event != "" && (len(event) >= 14 && event[:14] == "Normal Created") {
						createCount++
					}
				default:
					break drain
				}
			}
			Expect(createCount).To(Equal(1), "exactly one Created event expected across both reconciles")
		})

		It("should preserve status.ready when update of an existing maintenance window fails", func() {
			mw := CreateMaintenanceWindow(ctx, "test-update-failure-mw", account.Name, uptimerobotv1.MaintenanceWindowSpec{
				Name:      "Test Update Failure MW",
				Interval:  "daily",
				StartTime: "02:00:00",
				Duration:  metav1.Duration{Duration: time.Hour},
			})
			defer CleanupMaintenanceWindow(ctx, mw)

			_, err := ReconcileMaintenanceWindow(ctx, mw)
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, mw)).To(Succeed())
			Expect(mw.Status.Ready).To(BeTrue())

			originalAPI := os.Getenv("UPTIME_ROBOT_API")
			Expect(os.Setenv("UPTIME_ROBOT_API", "http://127.0.0.1:1")).To(Succeed())
			DeferCleanup(func() {
				Expect(os.Setenv("UPTIME_ROBOT_API", originalAPI)).To(Succeed())
			})

			_, err = ReconcileMaintenanceWindow(ctx, mw)
			Expect(err).To(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, mw)).To(Succeed())
			Expect(mw.Status.Ready).To(BeTrue())

			ready := findCondition(mw.Status.Conditions, TypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(ReasonAPIError))

			synced := findCondition(mw.Status.Conditions, TypeSynced)
			Expect(synced).NotTo(BeNil())
			Expect(synced.Status).To(Equal(metav1.ConditionFalse))
			Expect(synced.Reason).To(Equal(ReasonSyncError))

			errCond := findCondition(mw.Status.Conditions, TypeError)
			Expect(errCond).NotTo(BeNil())
			Expect(errCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(errCond.Reason).To(Equal(ReasonAPIError))
		})

		It("should set failure conditions when account secret is missing", func() {
			mw := CreateMaintenanceWindow(ctx, "test-missing-secret-mw", account.Name, uptimerobotv1.MaintenanceWindowSpec{
				Name:      "Test Missing Secret MW",
				Interval:  "daily",
				StartTime: "02:00:00",
				Duration:  metav1.Duration{Duration: time.Hour},
			})
			defer CleanupMaintenanceWindow(ctx, mw)

			Expect(k8sClient.Delete(ctx, secret)).To(Succeed())

			_, err := ReconcileMaintenanceWindow(ctx, mw)
			Expect(err).To(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, mw)).To(Succeed())
			Expect(mw.Status.Ready).To(BeFalse())

			ready := findCondition(mw.Status.Conditions, TypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(ReasonSecretNotFound))

			errCond := findCondition(mw.Status.Conditions, TypeError)
			Expect(errCond).NotTo(BeNil())
			Expect(errCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(errCond.Reason).To(Equal(ReasonSecretNotFound))

			Expect(findCondition(mw.Status.Conditions, TypeSynced)).To(BeNil())
		})

		It("should preserve status.ready on transient api key lookup failure for existing maintenance window", func() {
			mw := CreateMaintenanceWindow(ctx, "test-transient-secret-mw", account.Name, uptimerobotv1.MaintenanceWindowSpec{
				Name:      "Test Transient Secret MW",
				Interval:  "daily",
				StartTime: "02:00:00",
				Duration:  metav1.Duration{Duration: time.Hour},
			})
			defer CleanupMaintenanceWindow(ctx, mw)

			_, err := ReconcileMaintenanceWindow(ctx, mw)
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, mw)).To(Succeed())
			Expect(mw.Status.Ready).To(BeTrue())
			Expect(mw.Status.ID).NotTo(BeEmpty())

			Expect(k8sClient.Delete(ctx, secret)).To(Succeed())

			_, err = ReconcileMaintenanceWindow(ctx, mw)
			Expect(err).To(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, mw)).To(Succeed())
			Expect(mw.Status.Ready).To(BeTrue())
			Expect(mw.Status.ID).NotTo(BeEmpty())

			ready := findCondition(mw.Status.Conditions, TypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(ReasonSecretNotFound))

			errCond := findCondition(mw.Status.Conditions, TypeError)
			Expect(errCond).NotTo(BeNil())
			Expect(errCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(errCond.Reason).To(Equal(ReasonSecretNotFound))
		})

		It("should delete maintenance window with prune=true", func() {
			mw := CreateMaintenanceWindow(ctx, "test-delete-prune-mw", account.Name, uptimerobotv1.MaintenanceWindowSpec{
				Name:      "Test Delete Prune MW",
				Interval:  "daily",
				StartTime: "02:00:00",
				Duration:  metav1.Duration{Duration: time.Hour},
				Prune:     true,
			})

			// Create it first
			_, err := ReconcileMaintenanceWindow(ctx, mw)
			Expect(err).NotTo(HaveOccurred())

			// Get the latest version
			err = k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, mw)
			Expect(err).NotTo(HaveOccurred())
			Expect(mw.Status.Ready).To(BeTrue())

			// Delete it
			Expect(k8sClient.Delete(ctx, mw)).To(Succeed())

			// Reconcile to process deletion
			_, err = ReconcileMaintenanceWindow(ctx, mw)
			Expect(err).NotTo(HaveOccurred())

			// Verify it's deleted
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, mw)
				return errors.IsNotFound(err)
			}, testTimeout).Should(BeTrue())
		})

		It("should delete maintenance window with prune=false", func() {
			mw := CreateMaintenanceWindow(ctx, "test-delete-noprune-mw", account.Name, uptimerobotv1.MaintenanceWindowSpec{
				Name:      "Test Delete No Prune MW",
				Interval:  "daily",
				StartTime: "02:00:00",
				Duration:  metav1.Duration{Duration: time.Hour},
				Prune:     false,
			})

			// Create it first
			_, err := ReconcileMaintenanceWindow(ctx, mw)
			Expect(err).NotTo(HaveOccurred())

			// Get the latest version
			err = k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, mw)
			Expect(err).NotTo(HaveOccurred())
			mwID := mw.Status.ID

			// Delete it
			Expect(k8sClient.Delete(ctx, mw)).To(Succeed())

			// Reconcile to process deletion
			_, err = ReconcileMaintenanceWindow(ctx, mw)
			Expect(err).NotTo(HaveOccurred())

			// Verify CR is deleted but prune=false means external resource persists
			// (Note: In mock environment, we can't verify API state, but we verify finalizer is removed)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, mw)
				return errors.IsNotFound(err)
			}, testTimeout).Should(BeTrue())
			Expect(mwID).NotTo(BeEmpty(), "ID should have been set before deletion")
		})
	})

	Context("Monitor Reference Resolution", func() {
		var (
			ctx     context.Context
			secret  *corev1.Secret
			account *uptimerobotv1.Account
			contact *uptimerobotv1.Contact
		)

		BeforeEach(func() {
			ctx = context.Background()
			account, secret = CreateAccount(ctx)
			ReconcileAccount(ctx, account)
			contact = CreateContact(ctx, account.Name)
			ReconcileContact(ctx, contact)
		})

		AfterEach(func() {
			if contact != nil {
				CleanupContact(ctx, contact)
			}
			CleanupAccount(ctx, account, secret)
		})

		It("should resolve monitor references successfully", func() {
			// Create two monitors
			monitor1 := CreateMonitor(ctx, "test-mon1", account.Name, contact.Name)
			monitor2 := CreateMonitor(ctx, "test-mon2", account.Name, contact.Name)
			defer func() {
				CleanupMonitor(ctx, monitor1)
				CleanupMonitor(ctx, monitor2)
			}()

			// Reconcile monitors
			ReconcileMonitor(ctx, monitor1)
			ReconcileMonitor(ctx, monitor2)

			// Create maintenance window with monitor refs
			mw := CreateMaintenanceWindow(ctx, "test-monref-mw", account.Name, uptimerobotv1.MaintenanceWindowSpec{
				Name:      "Test Monitor Refs MW",
				Interval:  "daily",
				StartTime: "02:00:00",
				Duration:  metav1.Duration{Duration: time.Hour},
				MonitorRefs: []corev1.LocalObjectReference{
					{Name: monitor1.Name},
					{Name: monitor2.Name},
				},
			})
			defer CleanupMaintenanceWindow(ctx, mw)

			// Reconcile maintenance window
			_, err := ReconcileMaintenanceWindow(ctx, mw)
			Expect(err).NotTo(HaveOccurred())

			// Verify monitors were included
			err = k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, mw)
			Expect(err).NotTo(HaveOccurred())
			Expect(mw.Status.Ready).To(BeTrue())
			// Note: MonitorCount might be 0 in mock if API doesn't return monitor IDs
		})

		It("should skip monitors that are not ready", func() {
			// Create a monitor but don't reconcile it (so it's not ready)
			monitor1 := CreateMonitor(ctx, "test-notready-mon", account.Name, contact.Name)
			defer CleanupMonitor(ctx, monitor1)

			// Create maintenance window with monitor ref
			mw := CreateMaintenanceWindow(ctx, "test-notready-mw", account.Name, uptimerobotv1.MaintenanceWindowSpec{
				Name:      "Test Not Ready MW",
				Interval:  "daily",
				StartTime: "02:00:00",
				Duration:  metav1.Duration{Duration: time.Hour},
				MonitorRefs: []corev1.LocalObjectReference{
					{Name: monitor1.Name},
				},
			})
			defer CleanupMaintenanceWindow(ctx, mw)

			// Reconcile maintenance window
			_, err := ReconcileMaintenanceWindow(ctx, mw)
			Expect(err).NotTo(HaveOccurred())

			// Should succeed even though monitor is not ready
			err = k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, mw)
			Expect(err).NotTo(HaveOccurred())
			Expect(mw.Status.Ready).To(BeTrue())
		})

		It("should handle missing monitor references gracefully", func() {
			mw := CreateMaintenanceWindow(ctx, "test-missing-monref-mw", account.Name, uptimerobotv1.MaintenanceWindowSpec{
				Name:      "Test Missing Monitor MW",
				Interval:  "daily",
				StartTime: "02:00:00",
				Duration:  metav1.Duration{Duration: time.Hour},
				MonitorRefs: []corev1.LocalObjectReference{
					{Name: "non-existent-monitor"},
				},
			})
			defer CleanupMaintenanceWindow(ctx, mw)

			// Reconcile should succeed despite missing monitor
			_, err := ReconcileMaintenanceWindow(ctx, mw)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, mw)
			Expect(err).NotTo(HaveOccurred())
			Expect(mw.Status.Ready).To(BeTrue())
		})

		It("should clear all monitors when monitorRefs is empty", func() {
			// Create a monitor
			monitor1 := CreateMonitor(ctx, "test-clear-mon", account.Name, contact.Name)
			defer CleanupMonitor(ctx, monitor1)
			ReconcileMonitor(ctx, monitor1)

			// Create MW with monitor ref
			mw := CreateMaintenanceWindow(ctx, "test-clear-mw", account.Name, uptimerobotv1.MaintenanceWindowSpec{
				Name:      "Test Clear Monitors MW",
				Interval:  "daily",
				StartTime: "02:00:00",
				Duration:  metav1.Duration{Duration: time.Hour},
				MonitorRefs: []corev1.LocalObjectReference{
					{Name: monitor1.Name},
				},
			})
			defer CleanupMaintenanceWindow(ctx, mw)

			// Create it
			_, err := ReconcileMaintenanceWindow(ctx, mw)
			Expect(err).NotTo(HaveOccurred())

			// Clear monitor refs
			err = k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, mw)
			Expect(err).NotTo(HaveOccurred())
			mw.Spec.MonitorRefs = []corev1.LocalObjectReference{}
			Expect(k8sClient.Update(ctx, mw)).To(Succeed())

			// Reconcile again
			_, err = ReconcileMaintenanceWindow(ctx, mw)
			Expect(err).NotTo(HaveOccurred())

			// Verify still ready
			err = k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, mw)
			Expect(err).NotTo(HaveOccurred())
			Expect(mw.Status.Ready).To(BeTrue())
		})
	})

	Context("Duration Handling", func() {
		var (
			ctx     context.Context
			secret  *corev1.Secret
			account *uptimerobotv1.Account
		)

		BeforeEach(func() {
			ctx = context.Background()
			account, secret = CreateAccount(ctx)
			ReconcileAccount(ctx, account)
		})

		AfterEach(func() {
			CleanupAccount(ctx, account, secret)
		})

		It("should round up fractional minutes", func() {
			mw := CreateMaintenanceWindow(ctx, "test-roundup-mw", account.Name, uptimerobotv1.MaintenanceWindowSpec{
				Name:      "Test Round Up MW",
				Interval:  "daily",
				StartTime: "02:00:00",
				Duration:  metav1.Duration{Duration: 90 * time.Second}, // 1.5 minutes -> should round to 2
			})
			defer CleanupMaintenanceWindow(ctx, mw)

			_, err := ReconcileMaintenanceWindow(ctx, mw)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, mw)
			Expect(err).NotTo(HaveOccurred())
			Expect(mw.Status.Ready).To(BeTrue())
			// Note: We can't directly verify the API received 2 minutes in mock,
			// but the controller logic does the rounding
		})

		It("should enforce minimum duration of 1 minute", func() {
			mw := CreateMaintenanceWindow(ctx, "test-minduration-mw", account.Name, uptimerobotv1.MaintenanceWindowSpec{
				Name:      "Test Min Duration MW",
				Interval:  "daily",
				StartTime: "02:00:00",
				Duration:  metav1.Duration{Duration: 30 * time.Second}, // 0.5 minutes -> should be 1
			})
			defer CleanupMaintenanceWindow(ctx, mw)

			_, err := ReconcileMaintenanceWindow(ctx, mw)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, mw)
			Expect(err).NotTo(HaveOccurred())
			Expect(mw.Status.Ready).To(BeTrue())
		})
	})

	Context("Error Handling", func() {
		var (
			ctx     context.Context
			secret  *corev1.Secret
			account *uptimerobotv1.Account
		)

		BeforeEach(func() {
			ctx = context.Background()
			account, secret = CreateAccount(ctx)
			ReconcileAccount(ctx, account)
		})

		AfterEach(func() {
			CleanupAccount(ctx, account, secret)
		})

		It("should handle invalid monitor ID format", func() {
			// Create a monitor with normal ID
			contact := CreateContact(ctx, account.Name)
			ReconcileContact(ctx, contact)
			defer CleanupContact(ctx, contact)

			monitor := CreateMonitor(ctx, "test-invalidid-mon", account.Name, contact.Name)
			defer CleanupMonitor(ctx, monitor)
			ReconcileMonitor(ctx, monitor)

			// Manually set invalid ID format
			err := k8sClient.Get(ctx, types.NamespacedName{Name: monitor.Name, Namespace: monitor.Namespace}, monitor)
			Expect(err).NotTo(HaveOccurred())
			monitor.Status.ID = "invalid-id-not-numeric"
			monitor.Status.Ready = true
			Expect(k8sClient.Status().Update(ctx, monitor)).To(Succeed())

			// Create MW referencing this monitor
			mw := CreateMaintenanceWindow(ctx, "test-invalidid-mw", account.Name, uptimerobotv1.MaintenanceWindowSpec{
				Name:      "Test Invalid ID MW",
				Interval:  "daily",
				StartTime: "02:00:00",
				Duration:  metav1.Duration{Duration: time.Hour},
				MonitorRefs: []corev1.LocalObjectReference{
					{Name: monitor.Name},
				},
			})
			defer CleanupMaintenanceWindow(ctx, mw)

			// Should succeed despite invalid monitor ID (it will be skipped)
			_, err = ReconcileMaintenanceWindow(ctx, mw)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, mw)
			Expect(err).NotTo(HaveOccurred())
			Expect(mw.Status.Ready).To(BeTrue())
		})
	})

	Context("Finalizer Management", func() {
		var (
			ctx     context.Context
			secret  *corev1.Secret
			account *uptimerobotv1.Account
		)

		BeforeEach(func() {
			ctx = context.Background()
			account, secret = CreateAccount(ctx)
			ReconcileAccount(ctx, account)
		})

		AfterEach(func() {
			CleanupAccount(ctx, account, secret)
		})

		It("should add finalizer before external resource creation", func() {
			mw := CreateMaintenanceWindow(ctx, "test-finalizer-mw", account.Name, uptimerobotv1.MaintenanceWindowSpec{
				Name:      "Test Finalizer MW",
				Interval:  "daily",
				StartTime: "02:00:00",
				Duration:  metav1.Duration{Duration: time.Hour},
			})
			defer CleanupMaintenanceWindow(ctx, mw)

			// Initially no finalizer
			err := k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, mw)
			Expect(err).NotTo(HaveOccurred())
			Expect(mw.Finalizers).To(BeEmpty())

			// Reconcile
			_, err = ReconcileMaintenanceWindow(ctx, mw)
			Expect(err).NotTo(HaveOccurred())

			// Finalizer should be added
			err = k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, mw)
			Expect(err).NotTo(HaveOccurred())
			Expect(mw.Finalizers).To(ContainElement("uptimerobot.com/finalizer"))
		})
	})
})

// Helper functions

// CreateMaintenanceWindow creates a MaintenanceWindow CR for testing
func CreateMaintenanceWindow(ctx context.Context, name string, accountName string, spec uptimerobotv1.MaintenanceWindowSpec) *uptimerobotv1.MaintenanceWindow {
	spec.Account = corev1.LocalObjectReference{Name: accountName}
	mw := &uptimerobotv1.MaintenanceWindow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: spec,
	}
	Expect(k8sClient.Create(ctx, mw)).To(Succeed())
	return mw
}

// ReconcileMaintenanceWindow reconciles a MaintenanceWindow resource
func ReconcileMaintenanceWindow(ctx context.Context, mw *uptimerobotv1.MaintenanceWindow) (reconcile.Result, error) {
	reconciler := &MaintenanceWindowReconciler{
		Client:    k8sClient,
		APIReader: k8sClient,
		Scheme:    k8sClient.Scheme(),
	}
	return reconciler.Reconcile(ctx, reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      mw.Name,
			Namespace: mw.Namespace,
		},
	})
}

// CleanupMaintenanceWindow cleans up a MaintenanceWindow resource
func CleanupMaintenanceWindow(ctx context.Context, mw *uptimerobotv1.MaintenanceWindow) {
	err := k8sClient.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, mw)
	if err == nil {
		Expect(k8sClient.Delete(ctx, mw)).To(Succeed())
		// Force reconcile to process deletion
		_, _ = ReconcileMaintenanceWindow(ctx, mw)
	}
}

// CreateMonitor creates a Monitor CR for testing
func CreateMonitor(ctx context.Context, name string, accountName string, contactName string) *uptimerobotv1.Monitor {
	monitor := &uptimerobotv1.Monitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: uptimerobotv1.MonitorSpec{
			Account: corev1.LocalObjectReference{Name: accountName},
			Monitor: uptimerobotv1.MonitorValues{
				Name: "Test Monitor " + name,
				URL:  "https://example.com/" + name,
			},
			Contacts: []uptimerobotv1.MonitorContactRef{
				{LocalObjectReference: corev1.LocalObjectReference{Name: contactName}},
			},
		},
	}
	Expect(k8sClient.Create(ctx, monitor)).To(Succeed())
	return monitor
}

// ReconcileMonitor reconciles a Monitor resource
func ReconcileMonitor(ctx context.Context, monitor *uptimerobotv1.Monitor) {
	reconciler := &MonitorReconciler{
		Client: k8sClient,
		Scheme: k8sClient.Scheme(),
	}
	_, err := reconciler.Reconcile(ctx, reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      monitor.Name,
			Namespace: monitor.Namespace,
		},
	})
	Expect(err).NotTo(HaveOccurred())
}

// CleanupMonitor cleans up a Monitor resource
func CleanupMonitor(ctx context.Context, monitor *uptimerobotv1.Monitor) {
	err := k8sClient.Get(ctx, types.NamespacedName{Name: monitor.Name, Namespace: monitor.Namespace}, monitor)
	if err == nil {
		Expect(k8sClient.Delete(ctx, monitor)).To(Succeed())
	}
}
