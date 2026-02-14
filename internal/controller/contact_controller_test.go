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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	uptimerobotv1 "github.com/joelp172/uptime-robot-operator/api/v1alpha1"
)

var _ = Describe("Contact Controller", func() {
	Context("When reconciling a resource", func() {
		ctx := context.Background()
		var (
			secret  *corev1.Secret
			account *uptimerobotv1.Account
			contact *uptimerobotv1.Contact
		)
		var namespacedName types.NamespacedName

		BeforeEach(func() {
			By("creating the custom resource for the Kind Account")
			account, secret = CreateAccount(ctx)
			ReconcileAccount(ctx, account)

			By("creating the custom resource for the Kind Contact")
			contact = CreateContact(ctx, account.Name)
			namespacedName = types.NamespacedName{Name: contact.Name}
		})

		AfterEach(func() {
			resource := &uptimerobotv1.Contact{}
			err := k8sClient.Get(ctx, namespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Contact")
			CleanupContact(ctx, contact)

			By("Cleanup the specific resource instance Account")
			CleanupAccount(ctx, account, secret)
		})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			ReconcileContact(ctx, contact)

			Expect(contact.Status.ObservedGeneration).To(Equal(contact.Generation))
			ready := findCondition(contact.Status.Conditions, TypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionTrue))
			Expect(ready.Reason).To(Equal(ReasonReconcileSuccess))

			synced := findCondition(contact.Status.Conditions, TypeSynced)
			Expect(synced).NotTo(BeNil())
			Expect(synced.Status).To(Equal(metav1.ConditionTrue))
			Expect(synced.Reason).To(Equal(ReasonSyncSuccess))

			errCond := findCondition(contact.Status.Conditions, TypeError)
			Expect(errCond).NotTo(BeNil())
			Expect(errCond.Status).To(Equal(metav1.ConditionFalse))
		})

		It("should set failure conditions when account secret is missing", func() {
			controllerReconciler := &ContactReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			Expect(k8sClient.Delete(ctx, secret)).To(Succeed())

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).To(HaveOccurred())

			Expect(k8sClient.Get(ctx, namespacedName, contact)).To(Succeed())
			Expect(contact.Status.Ready).To(BeFalse())
			Expect(contact.Status.ObservedGeneration).To(Equal(contact.Generation))

			ready := findCondition(contact.Status.Conditions, TypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(ReasonSecretNotFound))

			errCond := findCondition(contact.Status.Conditions, TypeError)
			Expect(errCond).NotTo(BeNil())
			Expect(errCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(errCond.Reason).To(Equal(ReasonSecretNotFound))

			Expect(findCondition(contact.Status.Conditions, TypeSynced)).To(BeNil())
		})

		It("should set ready false and restore success conditions after transient secret failure", func() {
			controllerReconciler := &ContactReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			By("Reconciling contact successfully first")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, namespacedName, contact)).To(Succeed())
			Expect(contact.Status.Ready).To(BeTrue())
			Expect(contact.Status.ID).NotTo(BeEmpty())

			By("Simulating transient dependency failure by deleting account secret")
			restoredSecret := secret.DeepCopy()
			Expect(k8sClient.Delete(ctx, secret)).To(Succeed())

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).To(HaveOccurred())

			Expect(k8sClient.Get(ctx, namespacedName, contact)).To(Succeed())
			Expect(contact.Status.Ready).To(BeFalse())

			ready := findCondition(contact.Status.Conditions, TypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(ReasonSecretNotFound))

			errCond := findCondition(contact.Status.Conditions, TypeError)
			Expect(errCond).NotTo(BeNil())
			Expect(errCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(errCond.Reason).To(Equal(ReasonSecretNotFound))

			By("Restoring secret and reconciling again")
			restoredSecret.ResourceVersion = ""
			restoredSecret.UID = ""
			Expect(k8sClient.Create(ctx, restoredSecret)).To(Succeed())
			secret = restoredSecret

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, namespacedName, contact)).To(Succeed())
			Expect(contact.Status.Ready).To(BeTrue())

			ready = findCondition(contact.Status.Conditions, TypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionTrue))
			Expect(ready.Reason).To(Equal(ReasonReconcileSuccess))

			synced := findCondition(contact.Status.Conditions, TypeSynced)
			Expect(synced).NotTo(BeNil())
			Expect(synced.Status).To(Equal(metav1.ConditionUnknown))
			Expect(synced.Reason).To(Equal(ReasonSyncSkipped))

			errCond = findCondition(contact.Status.Conditions, TypeError)
			Expect(errCond).NotTo(BeNil())
			Expect(errCond.Status).To(Equal(metav1.ConditionFalse))
		})
	})
})

func CreateContact(ctx context.Context, accountName string) *uptimerobotv1.Contact {
	By("creating the secret for the Kind Contact")
	name := fmt.Sprintf("test-resource-%d", time.Now().UnixNano())
	contact := &uptimerobotv1.Contact{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: uptimerobotv1.ContactSpec{
			Account: corev1.LocalObjectReference{
				Name: accountName,
			},
			Contact: uptimerobotv1.ContactValues{
				Name: "John Doe",
			},
		},
	}
	Expect(k8sClient.Create(ctx, contact)).To(Succeed())
	return contact
}

func ReconcileContact(ctx context.Context, contact *uptimerobotv1.Contact) {
	controllerReconciler := &ContactReconciler{
		Client: k8sClient,
		Scheme: k8sClient.Scheme(),
	}

	namespacedName := types.NamespacedName{Name: contact.Name}

	_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
		NamespacedName: namespacedName,
	})
	Expect(err).NotTo(HaveOccurred())

	Expect(k8sClient.Get(ctx, namespacedName, contact)).To(Succeed())
	Expect(contact.Status.Ready).To(Equal(true))
	Expect(contact.Status.ID).To(Equal("993765"))
}

func CleanupContact(ctx context.Context, contact *uptimerobotv1.Contact) {
	if contact != nil {
		Expect(k8sClient.Delete(ctx, contact)).To(Succeed())
	}
}
