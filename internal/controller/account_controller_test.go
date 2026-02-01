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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	uptimerobotv1 "github.com/joelp172/uptime-robot-operator/api/v1"
)

var _ = Describe("Account Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()
		namespacedName := types.NamespacedName{Name: resourceName}
		var (
			secret  *corev1.Secret
			account *uptimerobotv1.Account
		)

		BeforeEach(func() {
			account, secret = CreateAccount(ctx)
		})

		AfterEach(func() {
			resource := &uptimerobotv1.Account{}
			err := k8sClient.Get(ctx, namespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Account")
			CleanupAccount(ctx, account, secret)
		})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			ReconcileAccount(ctx, account)
		})
	})
})

func CreateAccount(ctx context.Context) (*uptimerobotv1.Account, *corev1.Secret) {
	By("creating the secret for the Kind Account")
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "uptime-robot",
			Namespace: ClusterResourceNamespace,
		},
		Data: map[string][]byte{
			"apiKey": []byte("1234"),
		},
	}
	Expect(k8sClient.Create(ctx, secret)).To(Succeed())

	By("creating the custom resource for the Kind Account")
	account := &uptimerobotv1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-resource",
		},
		Spec: uptimerobotv1.AccountSpec{
			ApiKeySecretRef: corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "uptime-robot",
				},
				Key: "apiKey",
			},
		},
	}
	Expect(k8sClient.Create(ctx, account)).To(Succeed())

	return account, secret
}

func ReconcileAccount(ctx context.Context, account *uptimerobotv1.Account) {
	controllerReconciler := &AccountReconciler{
		Client: k8sClient,
		Scheme: k8sClient.Scheme(),
	}

	namespacedName := types.NamespacedName{Name: account.Name}

	_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
		NamespacedName: namespacedName,
	})
	Expect(err).NotTo(HaveOccurred())

	Expect(k8sClient.Get(ctx, namespacedName, account)).To(Succeed())
	Expect(account.Status.Ready).To(Equal(true))
}

func CleanupAccount(ctx context.Context, account *uptimerobotv1.Account, secret *corev1.Secret) {
	if account != nil {
		Expect(k8sClient.Delete(ctx, account)).To(Succeed())
	}
	if secret != nil {
		Expect(k8sClient.Delete(ctx, secret)).To(Succeed())
	}
}
