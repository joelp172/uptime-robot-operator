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
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	uptimerobotv1 "github.com/joelp172/uptime-robot-operator/api/v1alpha1"
)

var _ = Describe("MonitorGroup Controller", func() {
	Context("Basic Reconciliation", func() {
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
			CleanupContact(ctx, contact)
			CleanupAccount(ctx, account, secret)
		})

		It("should create monitor group successfully", func() {
			mg := CreateMonitorGroup(ctx, "test-create-mg", account.Name, uptimerobotv1.MonitorGroupSpec{
				FriendlyName: "Test Create Group",
			})
			defer CleanupMonitorGroup(ctx, mg)

			By("Reconciling the created resource")
			result, err := ReconcileMonitorGroup(ctx, mg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			By("Checking if finalizer is added")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: mg.Name, Namespace: mg.Namespace}, mg)
			Expect(err).NotTo(HaveOccurred())
			Expect(mg.Finalizers).To(ContainElement("uptimerobot.com/finalizer"))

			By("Checking if MonitorGroup status was updated")
			Expect(mg.Status.Ready).To(BeTrue())
			Expect(mg.Status.ID).NotTo(BeEmpty())

			ready := findCondition(mg.Status.Conditions, TypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionTrue))
			Expect(ready.Reason).To(Equal(ReasonReconcileSuccess))

			synced := findCondition(mg.Status.Conditions, TypeSynced)
			Expect(synced).NotTo(BeNil())
			Expect(synced.Status).To(Equal(metav1.ConditionTrue))
			Expect(synced.Reason).To(Equal(ReasonSyncSuccess))

			errCond := findCondition(mg.Status.Conditions, TypeError)
			Expect(errCond).NotTo(BeNil())
			Expect(errCond.Status).To(Equal(metav1.ConditionFalse))
		})

		It("should update monitor group name successfully", func() {
			mg := CreateMonitorGroup(ctx, "test-update-mg", account.Name, uptimerobotv1.MonitorGroupSpec{
				FriendlyName: "Test Update Group",
			})
			defer CleanupMonitorGroup(ctx, mg)

			// Create it first
			_, err := ReconcileMonitorGroup(ctx, mg)
			Expect(err).NotTo(HaveOccurred())

			// Update the name
			err = k8sClient.Get(ctx, types.NamespacedName{Name: mg.Name, Namespace: mg.Namespace}, mg)
			Expect(err).NotTo(HaveOccurred())
			mg.Spec.FriendlyName = "Updated Group Name"
			Expect(k8sClient.Update(ctx, mg)).To(Succeed())

			// Reconcile again
			_, err = ReconcileMonitorGroup(ctx, mg)
			Expect(err).NotTo(HaveOccurred())

			// Verify update
			err = k8sClient.Get(ctx, types.NamespacedName{Name: mg.Name, Namespace: mg.Namespace}, mg)
			Expect(err).NotTo(HaveOccurred())
			Expect(mg.Status.Ready).To(BeTrue())
		})

		It("should preserve status.ready when update of an existing monitor group fails", func() {
			mg := CreateMonitorGroup(ctx, "test-update-failure-mg", account.Name, uptimerobotv1.MonitorGroupSpec{
				FriendlyName: "Update Failure Group",
			})
			defer CleanupMonitorGroup(ctx, mg)

			_, err := ReconcileMonitorGroup(ctx, mg)
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mg.Name, Namespace: mg.Namespace}, mg)).To(Succeed())
			Expect(mg.Status.Ready).To(BeTrue())

			originalAPI := os.Getenv("UPTIME_ROBOT_API")
			Expect(os.Setenv("UPTIME_ROBOT_API", "http://127.0.0.1:1")).To(Succeed())
			DeferCleanup(func() {
				Expect(os.Setenv("UPTIME_ROBOT_API", originalAPI)).To(Succeed())
			})

			_, err = ReconcileMonitorGroup(ctx, mg)
			Expect(err).To(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mg.Name, Namespace: mg.Namespace}, mg)).To(Succeed())
			Expect(mg.Status.Ready).To(BeTrue())

			ready := findCondition(mg.Status.Conditions, TypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(ReasonAPIError))

			synced := findCondition(mg.Status.Conditions, TypeSynced)
			Expect(synced).NotTo(BeNil())
			Expect(synced.Status).To(Equal(metav1.ConditionFalse))
			Expect(synced.Reason).To(Equal(ReasonSyncError))

			errCond := findCondition(mg.Status.Conditions, TypeError)
			Expect(errCond).NotTo(BeNil())
			Expect(errCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(errCond.Reason).To(Equal(ReasonAPIError))
		})

		It("should set failure conditions when account secret is missing", func() {
			mg := CreateMonitorGroup(ctx, "test-missing-secret-mg", account.Name, uptimerobotv1.MonitorGroupSpec{
				FriendlyName: "Missing Secret Group",
			})
			defer CleanupMonitorGroup(ctx, mg)

			Expect(k8sClient.Delete(ctx, secret)).To(Succeed())

			_, err := ReconcileMonitorGroup(ctx, mg)
			Expect(err).To(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mg.Name, Namespace: mg.Namespace}, mg)).To(Succeed())
			Expect(mg.Status.Ready).To(BeFalse())

			ready := findCondition(mg.Status.Conditions, TypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(ReasonSecretNotFound))

			synced := findCondition(mg.Status.Conditions, TypeSynced)
			Expect(synced).To(BeNil())

			errCond := findCondition(mg.Status.Conditions, TypeError)
			Expect(errCond).NotTo(BeNil())
			Expect(errCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(errCond.Reason).To(Equal(ReasonSecretNotFound))
		})

		It("should preserve status.ready on transient api key lookup failure for existing group", func() {
			mg := CreateMonitorGroup(ctx, "test-transient-secret-group", account.Name, uptimerobotv1.MonitorGroupSpec{
				FriendlyName: "Transient Secret Group",
			})
			defer CleanupMonitorGroup(ctx, mg)

			_, err := ReconcileMonitorGroup(ctx, mg)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, types.NamespacedName{Name: mg.Name, Namespace: mg.Namespace}, mg)
			Expect(err).NotTo(HaveOccurred())
			Expect(mg.Status.Ready).To(BeTrue())
			Expect(mg.Status.ID).NotTo(BeEmpty())

			Expect(k8sClient.Delete(ctx, secret)).To(Succeed())

			_, err = ReconcileMonitorGroup(ctx, mg)
			Expect(err).To(HaveOccurred())

			err = k8sClient.Get(ctx, types.NamespacedName{Name: mg.Name, Namespace: mg.Namespace}, mg)
			Expect(err).NotTo(HaveOccurred())
			Expect(mg.Status.Ready).To(BeTrue())
			Expect(mg.Status.ID).NotTo(BeEmpty())

			ready := findCondition(mg.Status.Conditions, TypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(ReasonSecretNotFound))

			errCond := findCondition(mg.Status.Conditions, TypeError)
			Expect(errCond).NotTo(BeNil())
			Expect(errCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(errCond.Reason).To(Equal(ReasonSecretNotFound))

			// No backend sync attempt was made; keep last sync state from prior successful reconcile.
			synced := findCondition(mg.Status.Conditions, TypeSynced)
			Expect(synced).NotTo(BeNil())
			Expect(synced.Status).To(Equal(metav1.ConditionTrue))
			Expect(synced.Reason).To(Equal(ReasonSyncSuccess))
		})

		It("should handle monitor group deletion with prune", func() {
			mg := CreateMonitorGroup(ctx, "test-delete-mg", account.Name, uptimerobotv1.MonitorGroupSpec{
				FriendlyName: "Test Delete Group",
			})

			// Create it first
			_, err := ReconcileMonitorGroup(ctx, mg)
			Expect(err).NotTo(HaveOccurred())

			// Verify it was created
			err = k8sClient.Get(ctx, types.NamespacedName{Name: mg.Name, Namespace: mg.Namespace}, mg)
			Expect(err).NotTo(HaveOccurred())
			Expect(mg.Status.Ready).To(BeTrue())

			// Delete it
			CleanupMonitorGroup(ctx, mg)

			// Verify finalizer was removed
			err = k8sClient.Get(ctx, types.NamespacedName{Name: mg.Name, Namespace: mg.Namespace}, mg)
			Expect(err).To(HaveOccurred())
		})

		It("should retain finalizer when prune deletion fails", func() {
			mg := CreateMonitorGroup(ctx, "test-delete-failure-mg", account.Name, uptimerobotv1.MonitorGroupSpec{
				FriendlyName: "Test Delete Failure Group",
				Prune:        true,
			})

			By("Creating the group in backend")
			_, err := ReconcileMonitorGroup(ctx, mg)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, types.NamespacedName{Name: mg.Name, Namespace: mg.Namespace}, mg)
			Expect(err).NotTo(HaveOccurred())
			Expect(mg.Status.Ready).To(BeTrue())
			Expect(mg.Finalizers).To(ContainElement("uptimerobot.com/finalizer"))

			By("Deleting the CR")
			Expect(k8sClient.Delete(ctx, mg)).To(Succeed())

			By("Forcing purge failure during finalization")
			originalAPI := os.Getenv("UPTIME_ROBOT_API")
			Expect(os.Setenv("UPTIME_ROBOT_API", "http://127.0.0.1:1")).To(Succeed())
			DeferCleanup(func() {
				Expect(os.Setenv("UPTIME_ROBOT_API", originalAPI)).To(Succeed())
			})

			_, err = ReconcileMonitorGroup(ctx, mg)
			Expect(err).To(HaveOccurred())

			By("Ensuring the resource is still present with finalizer for retry")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: mg.Name, Namespace: mg.Namespace}, mg)
			Expect(err).NotTo(HaveOccurred())
			Expect(mg.DeletionTimestamp.IsZero()).To(BeFalse())
			Expect(mg.Finalizers).To(ContainElement("uptimerobot.com/finalizer"))
			Expect(mg.Status.Ready).To(BeTrue())

			// Check that Deleting condition is set with error
			deleting := findCondition(mg.Status.Conditions, TypeDeleting)
			Expect(deleting).NotTo(BeNil())
			Expect(deleting.Status).To(Equal(metav1.ConditionTrue))
			Expect(deleting.Reason).To(Equal(ReasonCleanupError))
			Expect(deleting.Message).To(ContainSubstring("Cleanup failed"))

			By("Restoring API and allowing finalization to complete")
			Expect(os.Setenv("UPTIME_ROBOT_API", originalAPI)).To(Succeed())
			_, err = ReconcileMonitorGroup(ctx, mg)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, types.NamespacedName{Name: mg.Name, Namespace: mg.Namespace}, mg)
			Expect(err).To(HaveOccurred())
		})

		It("should add monitors to group successfully", func() {
			// Create a monitor first
			monitor := CreateMonitor(ctx, "test-monitor-for-group", account.Name, contact.Name)
			defer func() {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: monitor.Name, Namespace: monitor.Namespace}, monitor)
				if err == nil {
					Expect(k8sClient.Delete(ctx, monitor)).To(Succeed())
				}
			}()

			// Reconcile the monitor so it has an ID
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

			// Get the monitor to verify it has an ID
			err = k8sClient.Get(ctx, types.NamespacedName{Name: monitor.Name, Namespace: monitor.Namespace}, monitor)
			Expect(err).NotTo(HaveOccurred())
			Expect(monitor.Status.Ready).To(BeTrue())
			Expect(monitor.Status.ID).NotTo(BeEmpty())

			// Create monitor group with monitor reference
			mg := CreateMonitorGroup(ctx, "test-group-with-monitors", account.Name, uptimerobotv1.MonitorGroupSpec{
				FriendlyName: "Test Group With Monitors",
				Monitors: []corev1.LocalObjectReference{
					{Name: monitor.Name},
				},
			})
			defer CleanupMonitorGroup(ctx, mg)

			// Reconcile the monitor group
			_, err = ReconcileMonitorGroup(ctx, mg)
			Expect(err).NotTo(HaveOccurred())

			// Verify monitor group was created successfully
			err = k8sClient.Get(ctx, types.NamespacedName{Name: mg.Name, Namespace: mg.Namespace}, mg)
			Expect(err).NotTo(HaveOccurred())
			Expect(mg.Status.Ready).To(BeTrue())
			// Note: MonitorCount will be 0 in unit tests since mock server returns static response
			// The actual monitor membership is tested in e2e tests
		})
	})
})

// CreateMonitorGroup creates a MonitorGroup CR for testing
func CreateMonitorGroup(ctx context.Context, name string, accountName string, spec uptimerobotv1.MonitorGroupSpec) *uptimerobotv1.MonitorGroup {
	spec.Account = corev1.LocalObjectReference{Name: accountName}
	mg := &uptimerobotv1.MonitorGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: spec,
	}
	Expect(k8sClient.Create(ctx, mg)).To(Succeed())
	return mg
}

// ReconcileMonitorGroup reconciles a MonitorGroup resource
func ReconcileMonitorGroup(ctx context.Context, mg *uptimerobotv1.MonitorGroup) (reconcile.Result, error) {
	reconciler := &MonitorGroupReconciler{
		Client:   k8sClient,
		Scheme:   k8sClient.Scheme(),
		Recorder: record.NewFakeRecorder(100),
	}
	return reconciler.Reconcile(ctx, reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      mg.Name,
			Namespace: mg.Namespace,
		},
	})
}

// CleanupMonitorGroup cleans up a MonitorGroup resource
func CleanupMonitorGroup(ctx context.Context, mg *uptimerobotv1.MonitorGroup) {
	err := k8sClient.Get(ctx, types.NamespacedName{Name: mg.Name, Namespace: mg.Namespace}, mg)
	if err == nil {
		Expect(k8sClient.Delete(ctx, mg)).To(Succeed())
		// Force reconcile to process deletion
		_, _ = ReconcileMonitorGroup(ctx, mg)
	}
}
