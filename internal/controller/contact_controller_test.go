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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	uptimerobotv1 "github.com/joelp172/uptime-robot-operator/api/v1"
)

var _ = Describe("Contact Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()
		namespacedName := types.NamespacedName{Name: resourceName}
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
		})
	})
})

func CreateContact(ctx context.Context, accountName string) *uptimerobotv1.Contact {
	By("creating the secret for the Kind Contact")
	contact := &uptimerobotv1.Contact{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-resource",
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
