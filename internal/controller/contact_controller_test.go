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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	uptimerobotv1 "github.com/joelp172/uptime-robot-operator/api/v1alpha1"
)

const mockSlackName = "Mock Slack"

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
			recorder := record.NewFakeRecorder(10)
			controllerReconciler := &ContactReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: recorder,
			}
			Expect(k8sClient.Delete(ctx, secret)).To(Succeed())

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).To(HaveOccurred())

			Expect(k8sClient.Get(ctx, namespacedName, contact)).To(Succeed())
			Expect(contact.Status.Ready).To(BeFalse())
			Expect(contact.Status.ObservedGeneration).To(BeNumerically("<", contact.Generation))

			ready := findCondition(contact.Status.Conditions, TypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(ReasonSecretNotFound))

			errCond := findCondition(contact.Status.Conditions, TypeError)
			Expect(errCond).NotTo(BeNil())
			Expect(errCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(errCond.Reason).To(Equal(ReasonSecretNotFound))

			Expect(findCondition(contact.Status.Conditions, TypeSynced)).To(BeNil())

			// Verify that a Warning event was recorded for the failure
			Eventually(recorder.Events).Should(Receive(ContainSubstring("SecretNotFound")))
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

		It("should record DependencyNotReady event when account is missing", func() {
			recorder := record.NewFakeRecorder(10)
			controllerReconciler := &ContactReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: recorder,
			}

			// Delete the account to simulate missing dependency
			Expect(k8sClient.Delete(ctx, account)).To(Succeed())

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).To(HaveOccurred())

			Expect(k8sClient.Get(ctx, namespacedName, contact)).To(Succeed())
			Expect(contact.Status.Ready).To(BeFalse())

			ready := findCondition(contact.Status.Conditions, TypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(ReasonReconcileError))

			// Verify that a Warning event was recorded for the dependency failure
			Eventually(recorder.Events).Should(Receive(ContainSubstring("DependencyNotReady")))
		})

		It("should successfully reconcile when contact id is specified directly", func() {
			name := fmt.Sprintf("test-direct-id-%d", time.Now().UnixNano())
			contactWithID := &uptimerobotv1.Contact{
				ObjectMeta: metav1.ObjectMeta{Name: name},
				Spec: uptimerobotv1.ContactSpec{
					Account: corev1.LocalObjectReference{Name: account.Name},
					Contact: uptimerobotv1.ContactValues{
						ID: "12345",
					},
				},
			}
			Expect(k8sClient.Create(ctx, contactWithID)).To(Succeed())
			defer CleanupContact(ctx, contactWithID)

			controllerReconciler := &ContactReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: name},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, contactWithID)).To(Succeed())
			Expect(contactWithID.Status.Ready).To(BeTrue())
			Expect(contactWithID.Status.ID).To(Equal("12345"))

			synced := findCondition(contactWithID.Status.Conditions, TypeSynced)
			Expect(synced).NotTo(BeNil())
			Expect(synced.Status).To(Equal(metav1.ConditionUnknown))
			Expect(synced.Reason).To(Equal(ReasonSyncSkipped))

			errCond := findCondition(contactWithID.Status.Conditions, TypeError)
			Expect(errCond).NotTo(BeNil())
			Expect(errCond.Status).To(Equal(metav1.ConditionFalse))
		})

		It("should re-resolve contact id when spec.contact.name changes", func() {
			name := fmt.Sprintf("test-name-change-%d", time.Now().UnixNano())
			contactByName := &uptimerobotv1.Contact{
				ObjectMeta: metav1.ObjectMeta{Name: name},
				Spec: uptimerobotv1.ContactSpec{
					Account: corev1.LocalObjectReference{Name: account.Name},
					Contact: uptimerobotv1.ContactValues{
						Name: "John Doe",
					},
				},
			}
			Expect(k8sClient.Create(ctx, contactByName)).To(Succeed())
			defer CleanupContact(ctx, contactByName)

			controllerReconciler := &ContactReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: name},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, contactByName)).To(Succeed())
			Expect(contactByName.Status.ID).To(Equal("993765"))
			Expect(contactByName.Status.ObservedGeneration).To(Equal(contactByName.Generation))

			originalGeneration := contactByName.Generation
			contactByName.Spec.Contact.Name = mockSlackName
			Expect(k8sClient.Update(ctx, contactByName)).To(Succeed())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, contactByName)).To(Succeed())
			Expect(contactByName.Generation).To(BeNumerically(">", originalGeneration))

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: name},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, contactByName)).To(Succeed())
			Expect(contactByName.Status.ID).To(Equal("101"))
			Expect(contactByName.Status.ObservedGeneration).To(Equal(contactByName.Generation))
		})

		It("should re-resolve contact id after generation change when upstream id changes for same friendly name", func() {
			name := fmt.Sprintf("test-generation-reresolve-%d", time.Now().UnixNano())
			contactByName := &uptimerobotv1.Contact{
				ObjectMeta: metav1.ObjectMeta{Name: name},
				Spec: uptimerobotv1.ContactSpec{
					Account: corev1.LocalObjectReference{Name: account.Name},
					Contact: uptimerobotv1.ContactValues{
						Name: mockSlackName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, contactByName)).To(Succeed())
			defer CleanupContact(ctx, contactByName)

			controllerReconciler := &ContactReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: name},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, contactByName)).To(Succeed())
			Expect(contactByName.Status.ID).To(Equal("101"))
			Expect(contactByName.Status.ObservedGeneration).To(Equal(contactByName.Generation))

			serverState.SetSlackIntegrations([]map[string]any{
				{
					"id":           845,
					"friendlyName": mockSlackName,
				},
			})

			// Bump generation without changing spec.contact.name so reconciliation
			// re-evaluates the existing name against upstream data.
			originalGeneration := contactByName.Generation
			contactByName.Spec.IsDefault = !contactByName.Spec.IsDefault
			Expect(k8sClient.Update(ctx, contactByName)).To(Succeed())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, contactByName)).To(Succeed())
			Expect(contactByName.Generation).To(BeNumerically(">", originalGeneration))

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: name},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, contactByName)).To(Succeed())
			Expect(contactByName.Status.ID).To(Equal("845"))
			Expect(contactByName.Status.ObservedGeneration).To(Equal(contactByName.Generation))
		})

		It("should not re-resolve contact id when generation is unchanged", func() {
			name := fmt.Sprintf("test-generation-unchanged-%d", time.Now().UnixNano())
			contactByName := &uptimerobotv1.Contact{
				ObjectMeta: metav1.ObjectMeta{Name: name},
				Spec: uptimerobotv1.ContactSpec{
					Account: corev1.LocalObjectReference{Name: account.Name},
					Contact: uptimerobotv1.ContactValues{
						Name: mockSlackName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, contactByName)).To(Succeed())
			defer CleanupContact(ctx, contactByName)

			controllerReconciler := &ContactReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: name},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, contactByName)).To(Succeed())
			Expect(contactByName.Status.ID).To(Equal("101"))
			Expect(contactByName.Status.ObservedGeneration).To(Equal(contactByName.Generation))
			observedAfterFirst := contactByName.Status.ObservedGeneration

			serverState.SetSlackIntegrations([]map[string]any{
				{
					"id":           999,
					"friendlyName": mockSlackName,
				},
			})

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: name},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, contactByName)).To(Succeed())
			Expect(contactByName.Status.ID).To(Equal("101"))
			Expect(contactByName.Status.ObservedGeneration).To(Equal(observedAfterFirst))
			Expect(contactByName.Status.ObservedGeneration).To(Equal(contactByName.Generation))
		})

		It("should not advance observedGeneration when re-resolution fails after prior success", func() {
			name := fmt.Sprintf("test-reresolve-failure-%d", time.Now().UnixNano())
			contactByName := &uptimerobotv1.Contact{
				ObjectMeta: metav1.ObjectMeta{Name: name},
				Spec: uptimerobotv1.ContactSpec{
					Account: corev1.LocalObjectReference{Name: account.Name},
					Contact: uptimerobotv1.ContactValues{
						Name: mockSlackName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, contactByName)).To(Succeed())
			defer CleanupContact(ctx, contactByName)

			controllerReconciler := &ContactReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: name},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, contactByName)).To(Succeed())
			Expect(contactByName.Status.ID).To(Equal("101"))
			Expect(contactByName.Status.ObservedGeneration).To(Equal(contactByName.Generation))
			successfulID := contactByName.Status.ID

			contactByName.Spec.Contact.Name = "Unknown Person Not In System"
			Expect(k8sClient.Update(ctx, contactByName)).To(Succeed())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, contactByName)).To(Succeed())
			newGeneration := contactByName.Generation

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: name},
			})
			Expect(err).To(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, contactByName)).To(Succeed())
			// Prior status.id remains; observedGeneration must NOT advance so the
			// next reconcile retries the new generation.
			Expect(contactByName.Status.ID).To(Equal(successfulID))
			Expect(contactByName.Status.ObservedGeneration).To(BeNumerically("<", newGeneration))
			Expect(contactByName.Status.Ready).To(BeFalse())
		})

		It("should set failure conditions when contact name is not found in UptimeRobot", func() {
			name := fmt.Sprintf("test-not-found-%d", time.Now().UnixNano())
			contactNotFound := &uptimerobotv1.Contact{
				ObjectMeta: metav1.ObjectMeta{Name: name},
				Spec: uptimerobotv1.ContactSpec{
					Account: corev1.LocalObjectReference{Name: account.Name},
					Contact: uptimerobotv1.ContactValues{
						Name: "Unknown Person Not In System",
					},
				},
			}
			Expect(k8sClient.Create(ctx, contactNotFound)).To(Succeed())
			defer CleanupContact(ctx, contactNotFound)

			recorder := record.NewFakeRecorder(10)
			controllerReconciler := &ContactReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: recorder,
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: name},
			})
			Expect(err).To(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, contactNotFound)).To(Succeed())
			Expect(contactNotFound.Status.Ready).To(BeFalse())

			ready := findCondition(contactNotFound.Status.Conditions, TypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(ReasonAPIError))

			synced := findCondition(contactNotFound.Status.Conditions, TypeSynced)
			Expect(synced).NotTo(BeNil())
			Expect(synced.Status).To(Equal(metav1.ConditionFalse))
			Expect(synced.Reason).To(Equal(ReasonSyncError))

			errCond := findCondition(contactNotFound.Status.Conditions, TypeError)
			Expect(errCond).NotTo(BeNil())
			Expect(errCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(errCond.Reason).To(Equal(ReasonAPIError))

			Eventually(recorder.Events).Should(Receive(ContainSubstring("SyncFailed")))
		})

		It("should resolve contact name from matching Slack integration when alert contact is missing", func() {
			name := fmt.Sprintf("test-slack-integration-%d", time.Now().UnixNano())
			contactFromIntegration := &uptimerobotv1.Contact{
				ObjectMeta: metav1.ObjectMeta{Name: name},
				Spec: uptimerobotv1.ContactSpec{
					Account: corev1.LocalObjectReference{Name: account.Name},
					Contact: uptimerobotv1.ContactValues{
						Name: mockSlackName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, contactFromIntegration)).To(Succeed())
			defer CleanupContact(ctx, contactFromIntegration)

			controllerReconciler := &ContactReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: name},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, contactFromIntegration)).To(Succeed())
			Expect(contactFromIntegration.Status.Ready).To(BeTrue())
			Expect(contactFromIntegration.Status.ID).To(Equal("101"))
		})

		It("should set failure conditions without transient requeue when Slack integration friendly name is ambiguous", func() {
			serverState.SetSlackIntegrations([]map[string]any{
				{
					"id":           501,
					"friendlyName": mockSlackName,
					"type":         "Slack",
					"status":       "Active",
					"value":        "https://hooks.slack.com/services/T000/B000/ONE",
				},
				{
					"id":           502,
					"friendlyName": mockSlackName,
					"type":         "Slack",
					"status":       "Active",
					"value":        "https://hooks.slack.com/services/T000/B000/TWO",
				},
			})

			name := fmt.Sprintf("test-slack-integration-ambiguous-%d", time.Now().UnixNano())
			contactFromAmbiguousIntegration := &uptimerobotv1.Contact{
				ObjectMeta: metav1.ObjectMeta{Name: name},
				Spec: uptimerobotv1.ContactSpec{
					Account: corev1.LocalObjectReference{Name: account.Name},
					Contact: uptimerobotv1.ContactValues{
						Name: mockSlackName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, contactFromAmbiguousIntegration)).To(Succeed())
			defer CleanupContact(ctx, contactFromAmbiguousIntegration)

			recorder := record.NewFakeRecorder(10)
			controllerReconciler := &ContactReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: recorder,
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: name},
			})
			Expect(err).To(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, contactFromAmbiguousIntegration)).To(Succeed())
			Expect(contactFromAmbiguousIntegration.Status.Ready).To(BeFalse())

			ready := findCondition(contactFromAmbiguousIntegration.Status.Conditions, TypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Message).To(ContainSubstring("ambiguous"))
			Expect(ready.Message).To(ContainSubstring(mockSlackName))

			synced := findCondition(contactFromAmbiguousIntegration.Status.Conditions, TypeSynced)
			Expect(synced).NotTo(BeNil())
			Expect(synced.Status).To(Equal(metav1.ConditionFalse))
			Expect(synced.Reason).To(Equal(ReasonSyncError))

			errCond := findCondition(contactFromAmbiguousIntegration.Status.Conditions, TypeError)
			Expect(errCond).NotTo(BeNil())
			Expect(errCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(errCond.Message).To(ContainSubstring("ambiguous"))
			Expect(errCond.Message).To(ContainSubstring(mockSlackName))

			Eventually(recorder.Events).Should(Receive(And(ContainSubstring("SyncFailed"), ContainSubstring("ambiguous"))))
		})

		It("should set failure conditions when API error occurs during contact name resolution", func() {
			serverState.SetAlertContactsHTTPStatus(http.StatusInternalServerError)

			name := fmt.Sprintf("test-api-error-%d", time.Now().UnixNano())
			contactAPIError := &uptimerobotv1.Contact{
				ObjectMeta: metav1.ObjectMeta{Name: name},
				Spec: uptimerobotv1.ContactSpec{
					Account: corev1.LocalObjectReference{Name: account.Name},
					Contact: uptimerobotv1.ContactValues{
						Name: "John Doe",
					},
				},
			}
			Expect(k8sClient.Create(ctx, contactAPIError)).To(Succeed())
			defer CleanupContact(ctx, contactAPIError)

			recorder := record.NewFakeRecorder(10)
			controllerReconciler := &ContactReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: recorder,
			}

			// With maxRetries=1, the 500 exhausts retries quickly.
			// ErrMaxRetriesExceeded is transient -> HandleReconcileError returns (RequeueAfter, nil).
			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: name},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, contactAPIError)).To(Succeed())
			Expect(contactAPIError.Status.Ready).To(BeFalse())

			ready := findCondition(contactAPIError.Status.Conditions, TypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(ReasonAPIError))

			errCond := findCondition(contactAPIError.Status.Conditions, TypeError)
			Expect(errCond).NotTo(BeNil())
			Expect(errCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(errCond.Reason).To(Equal(ReasonAPIError))

			Eventually(recorder.Events).Should(Receive(ContainSubstring("SyncFailed")))
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
