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

	"github.com/joelp172/uptime-robot-operator/internal/uptimerobot/urtypes"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	uptimerobotv1 "github.com/joelp172/uptime-robot-operator/api/v1alpha1"
)

var _ = Describe("Monitor Controller", func() {
	Context("When reconciling a resource", func() {
		ctx := context.Background()
		var namespacedName types.NamespacedName
		monitor := &uptimerobotv1.Monitor{}
		var (
			secret  *corev1.Secret
			account *uptimerobotv1.Account
			contact *uptimerobotv1.Contact
		)

		BeforeEach(func() {
			resourceName := fmt.Sprintf("test-resource-%d", time.Now().UnixNano())
			namespacedName = types.NamespacedName{
				Name:      resourceName,
				Namespace: "default",
			}
			By("creating the custom resource for the Kind Account")
			account, secret = CreateAccount(ctx)
			ReconcileAccount(ctx, account)

			By("creating the custom resource for the Kind Contact")
			contact = CreateContact(ctx, account.Name)
			ReconcileContact(ctx, contact)

			By("creating the custom resource for the Kind Monitor")
			err := k8sClient.Get(ctx, namespacedName, monitor)
			if err != nil && errors.IsNotFound(err) {
				resource := &uptimerobotv1.Monitor{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: uptimerobotv1.MonitorSpec{
						Account: corev1.LocalObjectReference{
							Name: account.Name,
						},
						Monitor: uptimerobotv1.MonitorValues{
							Name: "Test Monitor",
							URL:  "https://example.com",
						},
						Contacts: []uptimerobotv1.MonitorContactRef{
							{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: contact.Name,
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
				namespacedName = types.NamespacedName{Name: resourceName, Namespace: "default"}
			}
		})

		AfterEach(func() {
			resource := &uptimerobotv1.Monitor{}
			err := k8sClient.Get(ctx, namespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Monitor")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())

			By("Cleanup the specific resource instance Contact")
			CleanupContact(ctx, contact)

			By("Cleanup the specific resource instance Account")
			CleanupAccount(ctx, account, secret)
		})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &MonitorReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, namespacedName, monitor)).To(Succeed())
			Expect(monitor.Status.Ready).To(Equal(true))
			Expect(monitor.Status.ID).To(Equal("777810874"))
			Expect(monitor.Status.Type).To(Equal(urtypes.TypeHTTPS))
			Expect(monitor.Status.Status).To(Equal(uint8(1)))
			Expect(monitor.Status.LastSyncedTime).NotTo(BeNil())

			ready := findCondition(monitor.Status.Conditions, TypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionTrue))
			Expect(ready.Reason).To(Equal(ReasonReconcileSuccess))

			synced := findCondition(monitor.Status.Conditions, TypeSynced)
			Expect(synced).NotTo(BeNil())
			Expect(synced.Status).To(Equal(metav1.ConditionTrue))
			Expect(synced.Reason).To(Equal(ReasonSyncSuccess))

			errCond := findCondition(monitor.Status.Conditions, TypeError)
			Expect(errCond).NotTo(BeNil())
			Expect(errCond.Status).To(Equal(metav1.ConditionFalse))
		})

		It("should refresh lastSyncedTime on repeated successful reconciles", func() {
			controllerReconciler := &MonitorReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, namespacedName, monitor)).To(Succeed())
			firstSync := monitor.Status.LastSyncedTime
			Expect(firstSync).NotTo(BeNil())
			readyBefore := findCondition(monitor.Status.Conditions, TypeReady)
			Expect(readyBefore).NotTo(BeNil())
			firstReadyTransition := readyBefore.LastTransitionTime

			time.Sleep(1100 * time.Millisecond)

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, namespacedName, monitor)).To(Succeed())
			Expect(monitor.Status.LastSyncedTime).NotTo(BeNil())
			Expect(monitor.Status.LastSyncedTime.Time.After(firstSync.Time)).To(BeTrue())

			readyAfter := findCondition(monitor.Status.Conditions, TypeReady)
			Expect(readyAfter).NotTo(BeNil())
			Expect(readyAfter.LastTransitionTime).To(Equal(firstReadyTransition))
		})

		It("should preserve status.ready when edit of an existing monitor fails", func() {
			controllerReconciler := &MonitorReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, namespacedName, monitor)).To(Succeed())
			Expect(monitor.Status.Ready).To(BeTrue())

			// Force update-path failure by pointing the client at an unreachable API endpoint.
			originalAPI := os.Getenv("UPTIME_ROBOT_API")
			Expect(os.Setenv("UPTIME_ROBOT_API", "http://127.0.0.1:1")).To(Succeed())
			DeferCleanup(func() {
				Expect(os.Setenv("UPTIME_ROBOT_API", originalAPI)).To(Succeed())
			})

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).To(HaveOccurred())

			Expect(k8sClient.Get(ctx, namespacedName, monitor)).To(Succeed())
			Expect(monitor.Status.Ready).To(BeTrue())

			ready := findCondition(monitor.Status.Conditions, TypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(ReasonAPIError))

			synced := findCondition(monitor.Status.Conditions, TypeSynced)
			Expect(synced).NotTo(BeNil())
			Expect(synced.Status).To(Equal(metav1.ConditionFalse))
			Expect(synced.Reason).To(Equal(ReasonSyncError))

			errCond := findCondition(monitor.Status.Conditions, TypeError)
			Expect(errCond).NotTo(BeNil())
			Expect(errCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(errCond.Reason).To(Equal(ReasonAPIError))
		})

		It("should preserve status.ready when type-change delete fails", func() {
			controllerReconciler := &MonitorReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			By("Reconciling initially to create the monitor")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, namespacedName, monitor)).To(Succeed())
			Expect(monitor.Status.Ready).To(BeTrue())
			Expect(monitor.Status.Type).To(Equal(urtypes.TypeHTTPS))

			By("Changing monitor type to trigger delete-and-recreate path")
			monitor.Spec.Monitor.Type = urtypes.TypePing
			monitor.Spec.Monitor.URL = "8.8.8.8"
			Expect(k8sClient.Update(ctx, monitor)).To(Succeed())

			// Force delete-path failure by pointing the client at an unreachable API endpoint.
			originalAPI := os.Getenv("UPTIME_ROBOT_API")
			Expect(os.Setenv("UPTIME_ROBOT_API", "http://127.0.0.1:1")).To(Succeed())
			DeferCleanup(func() {
				Expect(os.Setenv("UPTIME_ROBOT_API", originalAPI)).To(Succeed())
			})

			By("Reconciling and expecting delete failure")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).To(HaveOccurred())

			Expect(k8sClient.Get(ctx, namespacedName, monitor)).To(Succeed())
			Expect(monitor.Status.Ready).To(BeTrue())
			Expect(monitor.Status.Type).To(Equal(urtypes.TypeHTTPS))

			ready := findCondition(monitor.Status.Conditions, TypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(ReasonAPIError))

			synced := findCondition(monitor.Status.Conditions, TypeSynced)
			Expect(synced).NotTo(BeNil())
			Expect(synced.Status).To(Equal(metav1.ConditionFalse))
			Expect(synced.Reason).To(Equal(ReasonSyncError))

			errCond := findCondition(monitor.Status.Conditions, TypeError)
			Expect(errCond).NotTo(BeNil())
			Expect(errCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(errCond.Reason).To(Equal(ReasonAPIError))
		})
	})

	Context("When adopting an existing monitor", func() {
		const resourceName = "test-adopt-monitor"
		// This ID matches a monitor that will be created by the mock UptimeRobot API
		const existingMonitorID = "777810874"
		ctx := context.Background()
		namespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		monitor := &uptimerobotv1.Monitor{}
		var (
			secret  *corev1.Secret
			account *uptimerobotv1.Account
			contact *uptimerobotv1.Contact
		)

		BeforeEach(func() {
			By("creating the custom resource for the Kind Account")
			account, secret = CreateAccount(ctx)
			ReconcileAccount(ctx, account)

			By("creating the custom resource for the Kind Contact")
			contact = CreateContact(ctx, account.Name)
			ReconcileContact(ctx, contact)

			By("creating the custom resource for the Kind Monitor with adopt-id annotation")
			err := k8sClient.Get(ctx, namespacedName, monitor)
			if err != nil && errors.IsNotFound(err) {
				resource := &uptimerobotv1.Monitor{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
						Annotations: map[string]string{
							AdoptIDAnnotation: existingMonitorID,
						},
					},
					Spec: uptimerobotv1.MonitorSpec{
						Account: corev1.LocalObjectReference{
							Name: account.Name,
						},
						Monitor: uptimerobotv1.MonitorValues{
							Name: "Adopted Monitor",
							URL:  "https://example.com",
							Type: urtypes.TypeHTTPS,
						},
						Contacts: []uptimerobotv1.MonitorContactRef{
							{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: contact.Name,
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &uptimerobotv1.Monitor{}
			err := k8sClient.Get(ctx, namespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Monitor")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())

			By("Cleanup the specific resource instance Contact")
			CleanupContact(ctx, contact)

			By("Cleanup the specific resource instance Account")
			CleanupAccount(ctx, account, secret)
		})

		It("should successfully adopt the existing monitor", func() {
			By("Reconciling the created resource with adoption annotation")
			controllerReconciler := &MonitorReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the monitor was adopted with the correct ID")
			Expect(k8sClient.Get(ctx, namespacedName, monitor)).To(Succeed())
			Expect(monitor.Status.Ready).To(Equal(true))
			Expect(monitor.Status.ID).To(Equal(existingMonitorID))
			Expect(monitor.Status.Type).To(Equal(urtypes.TypeHTTPS))
		})

		It("should fail to adopt monitor with type mismatch", func() {
			By("Creating a monitor resource with wrong type for adoption")
			mismatchNamespacedName := types.NamespacedName{
				Name:      "test-adopt-type-mismatch",
				Namespace: "default",
			}
			wrongTypeMonitor := &uptimerobotv1.Monitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      mismatchNamespacedName.Name,
					Namespace: mismatchNamespacedName.Namespace,
					Annotations: map[string]string{
						AdoptIDAnnotation: existingMonitorID,
					},
				},
				Spec: uptimerobotv1.MonitorSpec{
					Account: corev1.LocalObjectReference{
						Name: account.Name,
					},
					Monitor: uptimerobotv1.MonitorValues{
						Name: "Wrong Type Monitor",
						URL:  "8.8.8.8",
						Type: urtypes.TypePing, // Mismatch - existing is HTTPS
					},
					Contacts: []uptimerobotv1.MonitorContactRef{
						{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: contact.Name,
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, wrongTypeMonitor)).To(Succeed())

			By("Reconciling and expecting an error due to type mismatch")
			controllerReconciler := &MonitorReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: mismatchNamespacedName,
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("type mismatch"))

			By("Cleanup the type mismatch monitor")
			Expect(k8sClient.Delete(ctx, wrongTypeMonitor)).To(Succeed())
		})
	})

	Context("When deleting monitors with adoption protection", func() {
		ctx := context.Background()
		var (
			secret  *corev1.Secret
			account *uptimerobotv1.Account
			contact *uptimerobotv1.Contact
		)

		BeforeEach(func() {
			By("resetting mock server state")
			serverState.Reset()

			By("creating the custom resource for the Kind Account")
			account, secret = CreateAccount(ctx)
			ReconcileAccount(ctx, account)

			By("creating the custom resource for the Kind Contact")
			contact = CreateContact(ctx, account.Name)
			ReconcileContact(ctx, contact)
		})

		AfterEach(func() {
			By("Cleanup the specific resource instance Contact")
			CleanupContact(ctx, contact)

			By("Cleanup the specific resource instance Account")
			CleanupAccount(ctx, account, secret)
		})

		It("should not delete monitor from UptimeRobot when another monitor has adopted it", func() {
			// Create first monitor that creates the UptimeRobot monitor
			originalMonitor := &uptimerobotv1.Monitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-original-monitor",
					Namespace: "default",
				},
				Spec: uptimerobotv1.MonitorSpec{
					Prune: true,
					Account: corev1.LocalObjectReference{
						Name: account.Name,
					},
					Monitor: uptimerobotv1.MonitorValues{
						Name: "Original Monitor",
						URL:  "https://example.com",
						Type: urtypes.TypeHTTPS,
					},
					Contacts: []uptimerobotv1.MonitorContactRef{
						{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: contact.Name,
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, originalMonitor)).To(Succeed())

			By("Reconciling the original monitor to create it in UptimeRobot")
			controllerReconciler := &MonitorReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      originalMonitor.Name,
					Namespace: originalMonitor.Namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Get the created monitor ID
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      originalMonitor.Name,
				Namespace: originalMonitor.Namespace,
			}, originalMonitor)).To(Succeed())
			monitorID := originalMonitor.Status.ID
			Expect(monitorID).NotTo(BeEmpty())

			By("Creating an adopting monitor that references the same ID")
			adoptingMonitor := &uptimerobotv1.Monitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-adopting-monitor",
					Namespace: "default",
					Annotations: map[string]string{
						AdoptIDAnnotation: monitorID,
					},
				},
				Spec: uptimerobotv1.MonitorSpec{
					Prune: false,
					Account: corev1.LocalObjectReference{
						Name: account.Name,
					},
					Monitor: uptimerobotv1.MonitorValues{
						Name: "Adopting Monitor",
						URL:  "https://example.com",
						Type: urtypes.TypeHTTPS,
					},
					Contacts: []uptimerobotv1.MonitorContactRef{
						{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: contact.Name,
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, adoptingMonitor)).To(Succeed())

			By("Reconciling the adopting monitor")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      adoptingMonitor.Name,
					Namespace: adoptingMonitor.Namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Deleting the original monitor")
			Expect(k8sClient.Delete(ctx, originalMonitor)).To(Succeed())

			By("Reconciling the deletion")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      originalMonitor.Name,
					Namespace: originalMonitor.Namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the monitor was NOT deleted from the mock API")
			// Check the mock server state to ensure DeleteMonitor was not called for this ID
			Expect(serverState.IsMonitorDeleted(monitorID)).To(BeFalse(), "Monitor should not be deleted from API when another resource has adopted it")

			By("Verifying the adopting monitor still has the correct ID")
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      adoptingMonitor.Name,
				Namespace: adoptingMonitor.Namespace,
			}, adoptingMonitor)).To(Succeed())
			Expect(adoptingMonitor.Status.ID).To(Equal(monitorID))

			By("Cleanup the adopting monitor")
			Expect(k8sClient.Delete(ctx, adoptingMonitor)).To(Succeed())
		})

		It("should delete monitor from UptimeRobot when no other monitor has adopted it", func() {
			standaloneMonitor := &uptimerobotv1.Monitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-standalone-monitor",
					Namespace: "default",
				},
				Spec: uptimerobotv1.MonitorSpec{
					Prune: true,
					Account: corev1.LocalObjectReference{
						Name: account.Name,
					},
					Monitor: uptimerobotv1.MonitorValues{
						Name: "Standalone Monitor",
						URL:  "https://example.com",
						Type: urtypes.TypeHTTPS,
					},
					Contacts: []uptimerobotv1.MonitorContactRef{
						{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: contact.Name,
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, standaloneMonitor)).To(Succeed())

			By("Reconciling the standalone monitor to create it in UptimeRobot")
			controllerReconciler := &MonitorReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      standaloneMonitor.Name,
					Namespace: standaloneMonitor.Namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the monitor was created")
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      standaloneMonitor.Name,
				Namespace: standaloneMonitor.Namespace,
			}, standaloneMonitor)).To(Succeed())
			Expect(standaloneMonitor.Status.Ready).To(BeTrue())

			By("Deleting the standalone monitor")
			Expect(k8sClient.Delete(ctx, standaloneMonitor)).To(Succeed())

			By("Reconciling the deletion - monitor should be deleted from UptimeRobot")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      standaloneMonitor.Name,
					Namespace: standaloneMonitor.Namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the monitor no longer exists in k8s")
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      standaloneMonitor.Name,
				Namespace: standaloneMonitor.Namespace,
			}, standaloneMonitor)
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})

		It("should handle deletion gracefully when monitor has no Status.ID", func() {
			notReadyMonitor := &uptimerobotv1.Monitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-not-ready-monitor",
					Namespace: "default",
				},
				Spec: uptimerobotv1.MonitorSpec{
					Prune: true,
					Account: corev1.LocalObjectReference{
						Name: account.Name,
					},
					Monitor: uptimerobotv1.MonitorValues{
						Name: "Not Ready Monitor",
						URL:  "https://example.com",
						Type: urtypes.TypeHTTPS,
					},
					Contacts: []uptimerobotv1.MonitorContactRef{
						{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: contact.Name,
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, notReadyMonitor)).To(Succeed())

			By("Deleting the monitor before it becomes ready (no Status.ID)")
			Expect(k8sClient.Delete(ctx, notReadyMonitor)).To(Succeed())

			By("Reconciling the deletion - should not error")
			controllerReconciler := &MonitorReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      notReadyMonitor.Name,
					Namespace: notReadyMonitor.Namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should not delete monitor when adopter has adopt-id annotation but is not yet ready", func() {
			// Create first monitor
			originalMonitor := &uptimerobotv1.Monitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-original-for-early-adopt",
					Namespace: "default",
				},
				Spec: uptimerobotv1.MonitorSpec{
					Prune: true,
					Account: corev1.LocalObjectReference{
						Name: account.Name,
					},
					Monitor: uptimerobotv1.MonitorValues{
						Name: "Original for Early Adopt",
						URL:  "https://example.com",
						Type: urtypes.TypeHTTPS,
					},
					Contacts: []uptimerobotv1.MonitorContactRef{
						{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: contact.Name,
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, originalMonitor)).To(Succeed())

			By("Reconciling the original monitor")
			controllerReconciler := &MonitorReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      originalMonitor.Name,
					Namespace: originalMonitor.Namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      originalMonitor.Name,
				Namespace: originalMonitor.Namespace,
			}, originalMonitor)).To(Succeed())
			monitorID := originalMonitor.Status.ID

			By("Creating an adopting monitor with adopt-id annotation but not yet reconciled")
			adoptingMonitor := &uptimerobotv1.Monitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-early-adopter",
					Namespace: "default",
					Annotations: map[string]string{
						AdoptIDAnnotation: monitorID,
					},
				},
				Spec: uptimerobotv1.MonitorSpec{
					Prune: false,
					Account: corev1.LocalObjectReference{
						Name: account.Name,
					},
					Monitor: uptimerobotv1.MonitorValues{
						Name: "Early Adopter",
						URL:  "https://example.com",
						Type: urtypes.TypeHTTPS,
					},
					Contacts: []uptimerobotv1.MonitorContactRef{
						{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: contact.Name,
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, adoptingMonitor)).To(Succeed())

			By("Deleting the original monitor before adopter becomes ready")
			Expect(k8sClient.Delete(ctx, originalMonitor)).To(Succeed())

			By("Reconciling the deletion - should not delete from API due to adopt-id annotation")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      originalMonitor.Name,
					Namespace: originalMonitor.Namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the adopting monitor")
			Expect(k8sClient.Delete(ctx, adoptingMonitor)).To(Succeed())
		})

		It("should delete monitor when other monitor is being deleted", func() {
			// Create first monitor
			firstMonitor := &uptimerobotv1.Monitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-first-deleting",
					Namespace: "default",
				},
				Spec: uptimerobotv1.MonitorSpec{
					Prune: true,
					Account: corev1.LocalObjectReference{
						Name: account.Name,
					},
					Monitor: uptimerobotv1.MonitorValues{
						Name: "First Deleting",
						URL:  "https://example.com",
						Type: urtypes.TypeHTTPS,
					},
					Contacts: []uptimerobotv1.MonitorContactRef{
						{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: contact.Name,
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, firstMonitor)).To(Succeed())

			By("Reconciling the first monitor")
			controllerReconciler := &MonitorReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      firstMonitor.Name,
					Namespace: firstMonitor.Namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      firstMonitor.Name,
				Namespace: firstMonitor.Namespace,
			}, firstMonitor)).To(Succeed())
			monitorID := firstMonitor.Status.ID

			By("Creating a second monitor with same ID and marking it for deletion")
			secondMonitor := &uptimerobotv1.Monitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-second-deleting",
					Namespace: "default",
					Annotations: map[string]string{
						AdoptIDAnnotation: monitorID,
					},
				},
				Spec: uptimerobotv1.MonitorSpec{
					Prune: true,
					Account: corev1.LocalObjectReference{
						Name: account.Name,
					},
					Monitor: uptimerobotv1.MonitorValues{
						Name: "Second Deleting",
						URL:  "https://example.com",
						Type: urtypes.TypeHTTPS,
					},
					Contacts: []uptimerobotv1.MonitorContactRef{
						{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: contact.Name,
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, secondMonitor)).To(Succeed())

			By("Marking second monitor for deletion")
			Expect(k8sClient.Delete(ctx, secondMonitor)).To(Succeed())

			By("Deleting first monitor - should delete from API since second is being deleted")
			Expect(k8sClient.Delete(ctx, firstMonitor)).To(Succeed())

			By("Reconciling first monitor deletion")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      firstMonitor.Name,
					Namespace: firstMonitor.Namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When publishing heartbeat URLs", func() {
		ctx := context.Background()
		var (
			secret  *corev1.Secret
			account *uptimerobotv1.Account
			contact *uptimerobotv1.Contact
		)

		BeforeEach(func() {
			account, secret = CreateAccount(ctx)
			ReconcileAccount(ctx, account)

			contact = CreateContact(ctx, account.Name)
			ReconcileContact(ctx, contact)
		})

		AfterEach(func() {
			CleanupContact(ctx, contact)
			CleanupAccount(ctx, account, secret)
		})

		It("publishes heartbeat URL to a Secret with default target type, name and key", func() {
			monitor := &uptimerobotv1.Monitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "heartbeat-secret-monitor",
					Namespace: "default",
				},
				Spec: uptimerobotv1.MonitorSpec{
					Account: corev1.LocalObjectReference{Name: account.Name},
					Contacts: []uptimerobotv1.MonitorContactRef{
						{
							LocalObjectReference: corev1.LocalObjectReference{Name: contact.Name},
						},
					},
					Monitor: uptimerobotv1.MonitorValues{
						Name: "Heartbeat Secret Monitor",
						Type: urtypes.TypeHeartbeat,
						Heartbeat: &uptimerobotv1.MonitorHeartbeat{
							Interval: &metav1.Duration{Duration: 5 * time.Minute},
						},
					},
					HeartbeatURLPublish: &uptimerobotv1.HeartbeatURLPublish{},
				},
			}
			Expect(k8sClient.Create(ctx, monitor)).To(Succeed())

			controllerReconciler := &MonitorReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: monitor.Name, Namespace: monitor.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: monitor.Name, Namespace: monitor.Namespace}, monitor)).To(Succeed())
			Expect(monitor.Status.HeartbeatURL).NotTo(BeEmpty())

			published := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "heartbeat-secret-monitor-heartbeat-url", Namespace: "default"}, published)).To(Succeed())
			Expect(string(published.Data["heartbeatURL"])).To(Equal(monitor.Status.HeartbeatURL))
		})

		It("publishes heartbeat URL to a ConfigMap with defaults", func() {
			monitor := &uptimerobotv1.Monitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "heartbeat-configmap-monitor",
					Namespace: "default",
				},
				Spec: uptimerobotv1.MonitorSpec{
					Account: corev1.LocalObjectReference{Name: account.Name},
					Contacts: []uptimerobotv1.MonitorContactRef{
						{
							LocalObjectReference: corev1.LocalObjectReference{Name: contact.Name},
						},
					},
					Monitor: uptimerobotv1.MonitorValues{
						Name: "Heartbeat ConfigMap Monitor",
						Type: urtypes.TypeHeartbeat,
						Heartbeat: &uptimerobotv1.MonitorHeartbeat{
							Interval: &metav1.Duration{Duration: 5 * time.Minute},
						},
					},
					HeartbeatURLPublish: &uptimerobotv1.HeartbeatURLPublish{
						Type: uptimerobotv1.HeartbeatURLPublishTypeConfigMap,
					},
				},
			}
			Expect(k8sClient.Create(ctx, monitor)).To(Succeed())

			controllerReconciler := &MonitorReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: monitor.Name, Namespace: monitor.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: monitor.Name, Namespace: monitor.Namespace}, monitor)).To(Succeed())
			Expect(monitor.Status.HeartbeatURL).NotTo(BeEmpty())

			published := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "heartbeat-configmap-monitor-heartbeat-url", Namespace: "default"}, published)).To(Succeed())
			Expect(published.Data["heartbeatURL"]).To(Equal(monitor.Status.HeartbeatURL))
		})

		It("rejects publishing to an existing Secret not managed by the monitor", func() {
			existing := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "existing-heartbeat-secret",
					Namespace: "default",
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"url": []byte("https://old.example.invalid"),
				},
			}
			Expect(k8sClient.Create(ctx, existing)).To(Succeed())

			monitor := &uptimerobotv1.Monitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "heartbeat-existing-secret-monitor",
					Namespace: "default",
				},
				Spec: uptimerobotv1.MonitorSpec{
					Account: corev1.LocalObjectReference{Name: account.Name},
					Contacts: []uptimerobotv1.MonitorContactRef{
						{
							LocalObjectReference: corev1.LocalObjectReference{Name: contact.Name},
						},
					},
					Monitor: uptimerobotv1.MonitorValues{
						Name: "Heartbeat Existing Secret Monitor",
						Type: urtypes.TypeHeartbeat,
						Heartbeat: &uptimerobotv1.MonitorHeartbeat{
							Interval: &metav1.Duration{Duration: 5 * time.Minute},
						},
					},
					HeartbeatURLPublish: &uptimerobotv1.HeartbeatURLPublish{
						Type: uptimerobotv1.HeartbeatURLPublishTypeSecret,
						Name: "existing-heartbeat-secret",
						Key:  "url",
					},
				},
			}
			Expect(k8sClient.Create(ctx, monitor)).To(Succeed())

			controllerReconciler := &MonitorReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: monitor.Name, Namespace: monitor.Namespace},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not managed by Monitor"))

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: monitor.Name, Namespace: monitor.Namespace}, monitor)).To(Succeed())
			Expect(monitor.Status.HeartbeatURL).NotTo(BeEmpty())
			Expect(monitor.Status.Ready).To(BeFalse())
			Expect(monitor.Status.LastSyncedTime).NotTo(BeNil())

			ready := findCondition(monitor.Status.Conditions, TypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(ReasonReconcileError))

			synced := findCondition(monitor.Status.Conditions, TypeSynced)
			Expect(synced).NotTo(BeNil())
			Expect(synced.Status).To(Equal(metav1.ConditionTrue))
			Expect(synced.Reason).To(Equal(ReasonSyncSuccess))

			errCond := findCondition(monitor.Status.Conditions, TypeError)
			Expect(errCond).NotTo(BeNil())
			Expect(errCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(errCond.Reason).To(Equal(ReasonReconcileError))

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "existing-heartbeat-secret", Namespace: "default"}, existing)).To(Succeed())
			Expect(string(existing.Data["url"])).To(Equal("https://old.example.invalid"))
		})

		It("rejects publishing to an existing ConfigMap not managed by the monitor", func() {
			existing := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "existing-heartbeat-configmap",
					Namespace: "default",
				},
				Data: map[string]string{
					"url": "https://old.example.invalid",
				},
			}
			Expect(k8sClient.Create(ctx, existing)).To(Succeed())

			monitor := &uptimerobotv1.Monitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "heartbeat-existing-configmap-monitor",
					Namespace: "default",
				},
				Spec: uptimerobotv1.MonitorSpec{
					Account: corev1.LocalObjectReference{Name: account.Name},
					Contacts: []uptimerobotv1.MonitorContactRef{
						{
							LocalObjectReference: corev1.LocalObjectReference{Name: contact.Name},
						},
					},
					Monitor: uptimerobotv1.MonitorValues{
						Name: "Heartbeat Existing ConfigMap Monitor",
						Type: urtypes.TypeHeartbeat,
						Heartbeat: &uptimerobotv1.MonitorHeartbeat{
							Interval: &metav1.Duration{Duration: 5 * time.Minute},
						},
					},
					HeartbeatURLPublish: &uptimerobotv1.HeartbeatURLPublish{
						Type: uptimerobotv1.HeartbeatURLPublishTypeConfigMap,
						Name: "existing-heartbeat-configmap",
						Key:  "url",
					},
				},
			}
			Expect(k8sClient.Create(ctx, monitor)).To(Succeed())

			controllerReconciler := &MonitorReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: monitor.Name, Namespace: monitor.Namespace},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not managed by Monitor"))

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: monitor.Name, Namespace: monitor.Namespace}, monitor)).To(Succeed())
			Expect(monitor.Status.HeartbeatURL).NotTo(BeEmpty())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "existing-heartbeat-configmap", Namespace: "default"}, existing)).To(Succeed())
			Expect(existing.Data["url"]).To(Equal("https://old.example.invalid"))
		})

		It("does not publish heartbeat URL for non-heartbeat monitor types", func() {
			monitor := &uptimerobotv1.Monitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "non-heartbeat-no-publish-monitor",
					Namespace: "default",
				},
				Spec: uptimerobotv1.MonitorSpec{
					Account: corev1.LocalObjectReference{Name: account.Name},
					Contacts: []uptimerobotv1.MonitorContactRef{
						{
							LocalObjectReference: corev1.LocalObjectReference{Name: contact.Name},
						},
					},
					Monitor: uptimerobotv1.MonitorValues{
						Name: "Non Heartbeat No Publish Monitor",
						URL:  "https://example.com",
						Type: urtypes.TypeHTTPS,
					},
					HeartbeatURLPublish: &uptimerobotv1.HeartbeatURLPublish{
						Type: uptimerobotv1.HeartbeatURLPublishTypeSecret,
						Name: "should-not-be-created",
						Key:  "url",
					},
				},
			}
			Expect(k8sClient.Create(ctx, monitor)).To(Succeed())

			controllerReconciler := &MonitorReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: monitor.Name, Namespace: monitor.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: monitor.Name, Namespace: monitor.Namespace}, monitor)).To(Succeed())
			Expect(monitor.Status.HeartbeatURL).To(BeEmpty())
			Expect(monitor.Status.HeartbeatURLPublishTargetName).To(BeEmpty())

			notPublished := &corev1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: "should-not-be-created", Namespace: "default"}, notPublished)
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})

		It("deletes previously managed publish target when heartbeatURLPublish is removed", func() {
			monitor := &uptimerobotv1.Monitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "heartbeat-remove-publish-monitor",
					Namespace: "default",
				},
				Spec: uptimerobotv1.MonitorSpec{
					Account: corev1.LocalObjectReference{Name: account.Name},
					Contacts: []uptimerobotv1.MonitorContactRef{
						{
							LocalObjectReference: corev1.LocalObjectReference{Name: contact.Name},
						},
					},
					Monitor: uptimerobotv1.MonitorValues{
						Name: "Heartbeat Remove Publish Monitor",
						Type: urtypes.TypeHeartbeat,
						Heartbeat: &uptimerobotv1.MonitorHeartbeat{
							Interval: &metav1.Duration{Duration: 5 * time.Minute},
						},
					},
					HeartbeatURLPublish: &uptimerobotv1.HeartbeatURLPublish{
						Type: uptimerobotv1.HeartbeatURLPublishTypeSecret,
					},
				},
			}
			Expect(k8sClient.Create(ctx, monitor)).To(Succeed())

			controllerReconciler := &MonitorReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: monitor.Name, Namespace: monitor.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			createdSecretName := "heartbeat-remove-publish-monitor-heartbeat-url"
			createdSecret := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: createdSecretName, Namespace: monitor.Namespace}, createdSecret)).To(Succeed())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: monitor.Name, Namespace: monitor.Namespace}, monitor)).To(Succeed())
			monitor.Spec.HeartbeatURLPublish = nil
			Expect(k8sClient.Update(ctx, monitor)).To(Succeed())

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: monitor.Name, Namespace: monitor.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, types.NamespacedName{Name: createdSecretName, Namespace: monitor.Namespace}, &corev1.Secret{})
			Expect(errors.IsNotFound(err)).To(BeTrue())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: monitor.Name, Namespace: monitor.Namespace}, monitor)).To(Succeed())
			Expect(monitor.Status.HeartbeatURLPublishTargetType).To(BeEmpty())
			Expect(monitor.Status.HeartbeatURLPublishTargetName).To(BeEmpty())
			Expect(monitor.Status.HeartbeatURLPublishTargetKey).To(BeEmpty())
		})

		It("deletes previous managed target when heartbeatURLPublish target changes", func() {
			monitor := &uptimerobotv1.Monitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "heartbeat-switch-publish-target-monitor",
					Namespace: "default",
				},
				Spec: uptimerobotv1.MonitorSpec{
					Account: corev1.LocalObjectReference{Name: account.Name},
					Contacts: []uptimerobotv1.MonitorContactRef{
						{
							LocalObjectReference: corev1.LocalObjectReference{Name: contact.Name},
						},
					},
					Monitor: uptimerobotv1.MonitorValues{
						Name: "Heartbeat Switch Publish Target Monitor",
						Type: urtypes.TypeHeartbeat,
						Heartbeat: &uptimerobotv1.MonitorHeartbeat{
							Interval: &metav1.Duration{Duration: 5 * time.Minute},
						},
					},
					HeartbeatURLPublish: &uptimerobotv1.HeartbeatURLPublish{
						Type: uptimerobotv1.HeartbeatURLPublishTypeSecret,
					},
				},
			}
			Expect(k8sClient.Create(ctx, monitor)).To(Succeed())

			controllerReconciler := &MonitorReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: monitor.Name, Namespace: monitor.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			oldSecretName := "heartbeat-switch-publish-target-monitor-heartbeat-url"
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: oldSecretName, Namespace: monitor.Namespace}, &corev1.Secret{})).To(Succeed())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: monitor.Name, Namespace: monitor.Namespace}, monitor)).To(Succeed())
			monitor.Spec.HeartbeatURLPublish = &uptimerobotv1.HeartbeatURLPublish{
				Type: uptimerobotv1.HeartbeatURLPublishTypeConfigMap,
				Name: "heartbeat-switch-publish-target-cm",
				Key:  "url",
			}
			Expect(k8sClient.Update(ctx, monitor)).To(Succeed())

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: monitor.Name, Namespace: monitor.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, types.NamespacedName{Name: oldSecretName, Namespace: monitor.Namespace}, &corev1.Secret{})
			Expect(errors.IsNotFound(err)).To(BeTrue())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "heartbeat-switch-publish-target-cm", Namespace: monitor.Namespace}, &corev1.ConfigMap{})).To(Succeed())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: monitor.Name, Namespace: monitor.Namespace}, monitor)).To(Succeed())
			Expect(monitor.Status.HeartbeatURLPublishTargetType).To(Equal(uptimerobotv1.HeartbeatURLPublishTypeConfigMap))
			Expect(monitor.Status.HeartbeatURLPublishTargetName).To(Equal("heartbeat-switch-publish-target-cm"))
			Expect(monitor.Status.HeartbeatURLPublishTargetKey).To(Equal("url"))
		})

		It("updates managed Secret in place when only publish key changes", func() {
			monitor := &uptimerobotv1.Monitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "heartbeat-switch-publish-key-monitor",
					Namespace: "default",
				},
				Spec: uptimerobotv1.MonitorSpec{
					Account: corev1.LocalObjectReference{Name: account.Name},
					Contacts: []uptimerobotv1.MonitorContactRef{
						{
							LocalObjectReference: corev1.LocalObjectReference{Name: contact.Name},
						},
					},
					Monitor: uptimerobotv1.MonitorValues{
						Name: "Heartbeat Switch Publish Key Monitor",
						Type: urtypes.TypeHeartbeat,
						Heartbeat: &uptimerobotv1.MonitorHeartbeat{
							Interval: &metav1.Duration{Duration: 5 * time.Minute},
						},
					},
					HeartbeatURLPublish: &uptimerobotv1.HeartbeatURLPublish{
						Type: uptimerobotv1.HeartbeatURLPublishTypeSecret,
						Name: "heartbeat-switch-key-secret",
						Key:  "oldKey",
					},
				},
			}
			Expect(k8sClient.Create(ctx, monitor)).To(Succeed())

			controllerReconciler := &MonitorReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: monitor.Name, Namespace: monitor.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			secret := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "heartbeat-switch-key-secret", Namespace: monitor.Namespace}, secret)).To(Succeed())
			originalUID := secret.UID
			secret.Data["other"] = []byte("keep")
			Expect(k8sClient.Update(ctx, secret)).To(Succeed())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: monitor.Name, Namespace: monitor.Namespace}, monitor)).To(Succeed())
			monitor.Spec.HeartbeatURLPublish.Key = "newKey"
			Expect(k8sClient.Update(ctx, monitor)).To(Succeed())

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: monitor.Name, Namespace: monitor.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "heartbeat-switch-key-secret", Namespace: monitor.Namespace}, secret)).To(Succeed())
			Expect(secret.UID).To(Equal(originalUID))
			Expect(string(secret.Data["other"])).To(Equal("keep"))
			_, oldKeyExists := secret.Data["oldKey"]
			Expect(oldKeyExists).To(BeFalse())
			Expect(string(secret.Data["newKey"])).To(Equal(monitor.Status.HeartbeatURL))

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: monitor.Name, Namespace: monitor.Namespace}, monitor)).To(Succeed())
			Expect(monitor.Status.HeartbeatURLPublishTargetType).To(Equal(uptimerobotv1.HeartbeatURLPublishTypeSecret))
			Expect(monitor.Status.HeartbeatURLPublishTargetName).To(Equal("heartbeat-switch-key-secret"))
			Expect(monitor.Status.HeartbeatURLPublishTargetKey).To(Equal("newKey"))
		})
	})

	Context("buildHeartbeatURL", func() {
		It("handles empty and already-full URLs correctly", func() {
			Expect(buildHeartbeatURL("", "1", "")).To(Equal(""))
			Expect(buildHeartbeatURL("", "1", "https://heartbeat.uptimerobot.com/m1-token")).To(Equal("https://heartbeat.uptimerobot.com/m1-token"))
		})

		It("expands token paths", func() {
			Expect(buildHeartbeatURL("", "1", "m1-token")).To(Equal("https://heartbeat.uptimerobot.com/m1-token"))
			Expect(buildHeartbeatURL("", "123456789", "abcdef0123456789deadbeefcafebabe")).To(Equal("https://heartbeat.uptimerobot.com/m123456789-abcdef0123456789deadbeefcafebabe"))
			Expect(buildHeartbeatURL("", "123456789", "mabc-token-part")).To(Equal("https://heartbeat.uptimerobot.com/m123456789-mabc-token-part"))
		})

		It("supports a custom heartbeat base URL", func() {
			Expect(buildHeartbeatURL("https://heartbeat.example.internal", "42", "token-value")).To(Equal("https://heartbeat.example.internal/m42-token-value"))
			Expect(buildHeartbeatURL("heartbeat.example.internal/", "42", "m42-token")).To(Equal("https://heartbeat.example.internal/m42-token"))
		})
	})

	Context("normalizeHeartbeatBaseURL", func() {
		It("normalizes empty, host-only, and slash-suffixed values", func() {
			Expect(normalizeHeartbeatBaseURL("")).To(Equal(defaultHeartbeatBaseURL))
			Expect(normalizeHeartbeatBaseURL("heartbeat.example.internal")).To(Equal("https://heartbeat.example.internal"))
			Expect(normalizeHeartbeatBaseURL("https://heartbeat.example.internal/")).To(Equal("https://heartbeat.example.internal"))
		})
	})

	Context("configuredHeartbeatBaseURL", func() {
		It("reads and normalizes env var, defaulting when unset", func() {
			originalValue, hadOriginal := os.LookupEnv(heartbeatBaseURLEnvVar)
			DeferCleanup(func() {
				if hadOriginal {
					_ = os.Setenv(heartbeatBaseURLEnvVar, originalValue)
					return
				}
				_ = os.Unsetenv(heartbeatBaseURLEnvVar)
			})

			_ = os.Unsetenv(heartbeatBaseURLEnvVar)
			Expect(configuredHeartbeatBaseURL()).To(Equal(defaultHeartbeatBaseURL))

			_ = os.Setenv(heartbeatBaseURLEnvVar, "heartbeat.example.internal/")
			Expect(configuredHeartbeatBaseURL()).To(Equal("https://heartbeat.example.internal"))
		})
	})
})
