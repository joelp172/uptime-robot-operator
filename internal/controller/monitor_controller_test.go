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

	uptimerobotv1 "github.com/joelp172/uptime-robot-operator/api/v1"
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
})
