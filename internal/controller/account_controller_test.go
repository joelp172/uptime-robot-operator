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
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	uptimerobotv1 "github.com/joelp172/uptime-robot-operator/api/v1alpha1"
)

var _ = Describe("Account Controller", func() {
	Context("When reconciling a resource", func() {
		ctx := context.Background()
		var (
			secret  *corev1.Secret
			account *uptimerobotv1.Account
		)
		var namespacedName types.NamespacedName

		BeforeEach(func() {
			account, secret = CreateAccount(ctx)
			namespacedName = types.NamespacedName{Name: account.Name}
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

			Expect(account.Status.ObservedGeneration).To(Equal(account.Generation))
			ready := findCondition(account.Status.Conditions, TypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionTrue))
			Expect(ready.Reason).To(Equal(ReasonReconcileSuccess))

			synced := findCondition(account.Status.Conditions, TypeSynced)
			Expect(synced).NotTo(BeNil())
			Expect(synced.Status).To(Equal(metav1.ConditionTrue))
			Expect(synced.Reason).To(Equal(ReasonSyncSuccess))

			errCond := findCondition(account.Status.Conditions, TypeError)
			Expect(errCond).NotTo(BeNil())
			Expect(errCond.Status).To(Equal(metav1.ConditionFalse))
		})

		It("should set failure conditions when api key secret is missing", func() {
			recorder := record.NewFakeRecorder(10)
			controllerReconciler := &AccountReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: recorder,
			}

			Expect(k8sClient.Delete(ctx, secret)).To(Succeed())

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).To(HaveOccurred())

			Expect(k8sClient.Get(ctx, namespacedName, account)).To(Succeed())
			Expect(account.Status.Ready).To(BeFalse())
			Expect(account.Status.ObservedGeneration).To(Equal(account.Generation))

			ready := findCondition(account.Status.Conditions, TypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(ReasonSecretNotFound))

			errCond := findCondition(account.Status.Conditions, TypeError)
			Expect(errCond).NotTo(BeNil())
			Expect(errCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(errCond.Reason).To(Equal(ReasonSecretNotFound))

			Expect(findCondition(account.Status.Conditions, TypeSynced)).To(BeNil())

			// Verify that a Warning event was recorded for the failure
			Eventually(recorder.Events).Should(Receive(ContainSubstring("SecretNotFound")))
		})

		It("should recover when the missing api key secret is created later", func() {
			suffix := fmt.Sprintf("%d", time.Now().UnixNano())
			accountName := "recover-account-" + suffix
			secretName := "recover-secret-" + suffix

			account := &uptimerobotv1.Account{
				ObjectMeta: metav1.ObjectMeta{Name: accountName},
				Spec: uptimerobotv1.AccountSpec{
					ApiKeySecretRef: corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
						Key:                  "apiKey",
					},
				},
			}
			Expect(k8sClient.Create(ctx, account)).To(Succeed())
			defer CleanupAccount(ctx, account, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: ClusterResourceNamespace},
			})

			mgr, err := ctrl.NewManager(cfg, ctrl.Options{
				Scheme:                 scheme.Scheme,
				Metrics:                metricsserver.Options{BindAddress: "0"},
				HealthProbeBindAddress: "0",
				LeaderElection:         false,
			})
			Expect(err).NotTo(HaveOccurred())

			reconciler := &AccountReconciler{
				Client: mgr.GetClient(),
				Scheme: mgr.GetScheme(),
			}
			Expect(reconciler.SetupWithManager(mgr)).To(Succeed())

			mgrCtx, cancelMgr := context.WithCancel(ctx)
			defer cancelMgr()
			go func() {
				_ = mgr.Start(mgrCtx)
			}()

			Eventually(func(g Gomega) {
				current := &uptimerobotv1.Account{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: accountName}, current)).To(Succeed())
				ready := findCondition(current.Status.Conditions, TypeReady)
				g.Expect(ready).NotTo(BeNil())
				g.Expect(ready.Reason).To(Equal(ReasonSecretNotFound))
			}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: ClusterResourceNamespace,
				},
				Data: map[string][]byte{"apiKey": []byte("1234")},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			Eventually(func(g Gomega) {
				current := &uptimerobotv1.Account{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: accountName}, current)).To(Succeed())
				g.Expect(current.Status.Ready).To(BeTrue())
				ready := findCondition(current.Status.Conditions, TypeReady)
				g.Expect(ready).NotTo(BeNil())
				g.Expect(ready.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(ready.Reason).To(Equal(ReasonReconcileSuccess))
			}, 20*time.Second, 500*time.Millisecond).Should(Succeed())
		})

		It("should set failure conditions when api key secret has wrong key name", func() {
			recorder := record.NewFakeRecorder(10)
			controllerReconciler := &AccountReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: recorder,
			}

			By("updating the secret to have a different key name")
			secret.Data = map[string][]byte{"wrongKey": []byte("1234")}
			Expect(k8sClient.Update(ctx, secret)).To(Succeed())

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).To(HaveOccurred())

			Expect(k8sClient.Get(ctx, namespacedName, account)).To(Succeed())
			Expect(account.Status.Ready).To(BeFalse())
			Expect(account.Status.ObservedGeneration).To(Equal(account.Generation))

			ready := findCondition(account.Status.Conditions, TypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(ReasonSecretNotFound))

			errCond := findCondition(account.Status.Conditions, TypeError)
			Expect(errCond).NotTo(BeNil())
			Expect(errCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(errCond.Reason).To(Equal(ReasonSecretNotFound))

			Expect(findCondition(account.Status.Conditions, TypeSynced)).To(BeNil())

			Eventually(recorder.Events).Should(Receive(ContainSubstring("SecretNotFound")))
		})

		It("should set failure conditions when GetAccountDetails returns auth failure", func() {
			serverState.SetUserMeHTTPStatus(http.StatusUnauthorized)

			recorder := record.NewFakeRecorder(10)
			controllerReconciler := &AccountReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: recorder,
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).To(HaveOccurred())

			Expect(k8sClient.Get(ctx, namespacedName, account)).To(Succeed())
			Expect(account.Status.Ready).To(BeFalse())

			ready := findCondition(account.Status.Conditions, TypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(ReasonAPIError))

			synced := findCondition(account.Status.Conditions, TypeSynced)
			Expect(synced).NotTo(BeNil())
			Expect(synced.Status).To(Equal(metav1.ConditionFalse))
			Expect(synced.Reason).To(Equal(ReasonSyncError))

			errCond := findCondition(account.Status.Conditions, TypeError)
			Expect(errCond).NotTo(BeNil())
			Expect(errCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(errCond.Reason).To(Equal(ReasonAPIError))

			Eventually(recorder.Events).Should(Receive(ContainSubstring("SyncFailed")))
		})

		It("should set failure conditions and requeue when GetAccountDetails returns transient error", func() {
			serverState.SetUserMeHTTPStatus(http.StatusInternalServerError)

			controllerReconciler := &AccountReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})
			// Transient errors cause RequeueAfter, not a returned error
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			Expect(k8sClient.Get(ctx, namespacedName, account)).To(Succeed())
			Expect(account.Status.Ready).To(BeFalse())

			ready := findCondition(account.Status.Conditions, TypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(ReasonAPIError))

			synced := findCondition(account.Status.Conditions, TypeSynced)
			Expect(synced).NotTo(BeNil())
			Expect(synced.Status).To(Equal(metav1.ConditionFalse))
			Expect(synced.Reason).To(Equal(ReasonSyncError))
		})

		It("should still reconcile successfully when alert contacts fetch fails", func() {
			serverState.SetAlertContactsHTTPStatus(http.StatusInternalServerError)

			controllerReconciler := &AccountReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, namespacedName, account)).To(Succeed())
			Expect(account.Status.Ready).To(BeTrue())
			Expect(account.Status.AlertContacts).To(BeEmpty())

			ready := findCondition(account.Status.Conditions, TypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionTrue))
			Expect(ready.Reason).To(Equal(ReasonReconcileSuccess))
		})

		It("should populate status fields correctly from API response", func() {
			By("Reconciling the created resource")
			ReconcileAccount(ctx, account)

			Expect(account.Status.Email).To(Equal("test@example.com"))
			Expect(account.Status.AlertContacts).To(HaveLen(3))

			var johnDoe *uptimerobotv1.AlertContactInfo
			for i := range account.Status.AlertContacts {
				if account.Status.AlertContacts[i].FriendlyName == "John Doe" {
					johnDoe = &account.Status.AlertContacts[i]
					break
				}
			}
			Expect(johnDoe).NotTo(BeNil())
			Expect(johnDoe.ID).To(Equal("993765"))
			Expect(johnDoe.Type).To(Equal("Email"))
		})
	})
})

func CreateAccount(ctx context.Context) (*uptimerobotv1.Account, *corev1.Secret) {
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	secretName := "uptime-robot-" + suffix
	accountName := "test-resource-" + suffix

	By("creating the secret for the Kind Account")
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
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
			Name: accountName,
		},
		Spec: uptimerobotv1.AccountSpec{
			ApiKeySecretRef: corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secretName,
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
		resource := &uptimerobotv1.Account{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: account.Name}, resource); err == nil {
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		}
	}
	if secret != nil {
		resource := &corev1.Secret{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, resource); err == nil {
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		}
	}
}
