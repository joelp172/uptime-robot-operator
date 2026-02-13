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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	uptimerobotv1 "github.com/joelp172/uptime-robot-operator/api/v1alpha1"
)

var _ = Describe("SlackIntegration Controller", func() {
	Context("When reconciling a resource", func() {
		ctx := context.Background()
		var (
			resourceName     string
			namespacedName   types.NamespacedName
			secret           *corev1.Secret
			account          *uptimerobotv1.Account
			webhookSecret    *corev1.Secret
			slackIntegration *uptimerobotv1.SlackIntegration
		)

		BeforeEach(func() {
			resourceName = fmt.Sprintf("test-slackintegration-%d", time.Now().UnixNano())
			namespacedName = types.NamespacedName{
				Name:      resourceName,
				Namespace: "default",
			}

			account, secret = CreateAccount(ctx)
			ReconcileAccount(ctx, account)

			webhookSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-slack-webhook-%d", time.Now().UnixNano()),
					Namespace: "default",
				},
				Data: map[string][]byte{
					"webhookURL": []byte("https://hooks.slack.com/services/T000/B000/TEST"),
				},
			}
			Expect(k8sClient.Create(ctx, webhookSecret)).To(Succeed())

			slackIntegration = &uptimerobotv1.SlackIntegration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: uptimerobotv1.SlackIntegrationSpec{
					Account: corev1.LocalObjectReference{Name: account.Name},
					Integration: uptimerobotv1.SlackIntegrationValues{
						FriendlyName:           "Test Slack Integration",
						EnableNotificationsFor: "Down",
						SecretName:             webhookSecret.Name,
						WebhookURLKey:          "webhookURL",
						CustomValue:            "from controller test",
					},
				},
			}
			Expect(k8sClient.Create(ctx, slackIntegration)).To(Succeed())
		})

		AfterEach(func() {
			resource := &uptimerobotv1.SlackIntegration{}
			Expect(k8sClient.Get(ctx, namespacedName, resource)).To(Succeed())
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())

			if webhookSecret != nil {
				secret := &corev1.Secret{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: webhookSecret.Name, Namespace: webhookSecret.Namespace}, secret); err == nil {
					Expect(k8sClient.Delete(ctx, secret)).To(Succeed())
				}
			}

			CleanupAccount(ctx, account, secret)
		})

		It("should successfully reconcile and create a Slack integration", func() {
			controllerReconciler := &SlackIntegrationReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, namespacedName, slackIntegration)).To(Succeed())
			Expect(slackIntegration.Status.Ready).To(BeTrue())
			Expect(slackIntegration.Status.ID).NotTo(BeEmpty())
			Expect(slackIntegration.Status.Type).To(Equal("Slack"))

			ready := findCondition(slackIntegration.Status.Conditions, TypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionTrue))
			Expect(ready.Reason).To(Equal(ReasonReconcileSuccess))

			synced := findCondition(slackIntegration.Status.Conditions, TypeSynced)
			Expect(synced).NotTo(BeNil())
			Expect(synced.Status).To(Equal(metav1.ConditionTrue))
			Expect(synced.Reason).To(Equal(ReasonSyncSuccess))

			errCond := findCondition(slackIntegration.Status.Conditions, TypeError)
			Expect(errCond).NotTo(BeNil())
			Expect(errCond.Status).To(Equal(metav1.ConditionFalse))
		})

		It("should recreate integration when spec drifts from existing integration", func() {
			controllerReconciler := &SlackIntegrationReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Get(ctx, namespacedName, slackIntegration)).To(Succeed())
			originalID := slackIntegration.Status.ID
			Expect(originalID).NotTo(BeEmpty())

			slackIntegration.Spec.Integration.FriendlyName = "Updated Slack Integration Name"
			Expect(k8sClient.Update(ctx, slackIntegration)).To(Succeed())

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Get(ctx, namespacedName, slackIntegration)).To(Succeed())

			Expect(slackIntegration.Status.ID).NotTo(BeEmpty())
			Expect(slackIntegration.Status.ID).NotTo(Equal(originalID))
			Expect(slackIntegration.Status.Type).To(Equal("Slack"))
		})

		It("should set failure conditions when webhook secret is missing", func() {
			controllerReconciler := &SlackIntegrationReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			Expect(k8sClient.Delete(ctx, webhookSecret)).To(Succeed())

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).To(HaveOccurred())

			Expect(k8sClient.Get(ctx, namespacedName, slackIntegration)).To(Succeed())
			Expect(slackIntegration.Status.Ready).To(BeFalse())

			ready := findCondition(slackIntegration.Status.Conditions, TypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(ReasonReconcileError))

			errCond := findCondition(slackIntegration.Status.Conditions, TypeError)
			Expect(errCond).NotTo(BeNil())
			Expect(errCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(errCond.Reason).To(Equal(ReasonReconcileError))

			Expect(findCondition(slackIntegration.Status.Conditions, TypeSynced)).To(BeNil())
		})

		It("should preserve status.ready when list integrations fails for an existing resource", func() {
			controllerReconciler := &SlackIntegrationReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Get(ctx, namespacedName, slackIntegration)).To(Succeed())
			Expect(slackIntegration.Status.Ready).To(BeTrue())

			originalAPI := os.Getenv("UPTIME_ROBOT_API")
			Expect(os.Setenv("UPTIME_ROBOT_API", "http://127.0.0.1:1")).To(Succeed())
			DeferCleanup(func() {
				Expect(os.Setenv("UPTIME_ROBOT_API", originalAPI)).To(Succeed())
			})

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).To(HaveOccurred())

			Expect(k8sClient.Get(ctx, namespacedName, slackIntegration)).To(Succeed())
			Expect(slackIntegration.Status.Ready).To(BeTrue())

			ready := findCondition(slackIntegration.Status.Conditions, TypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(ReasonAPIError))

			synced := findCondition(slackIntegration.Status.Conditions, TypeSynced)
			Expect(synced).NotTo(BeNil())
			Expect(synced.Status).To(Equal(metav1.ConditionFalse))
			Expect(synced.Reason).To(Equal(ReasonSyncError))

			errCond := findCondition(slackIntegration.Status.Conditions, TypeError)
			Expect(errCond).NotTo(BeNil())
			Expect(errCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(errCond.Reason).To(Equal(ReasonAPIError))
		})
	})
})
