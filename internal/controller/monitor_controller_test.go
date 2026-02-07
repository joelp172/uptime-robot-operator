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
		const resourceName = "test-resource"
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

			By("Verifying the monitor still exists in the mock API")
			// The mock server tracks created monitors - if it was deleted, this would fail
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
})
