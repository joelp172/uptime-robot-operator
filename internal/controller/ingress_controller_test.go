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

	uptimerobotv1 "github.com/joelp172/uptime-robot-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("Ingress Controller", func() {
	Context("When reconciling a resource", func() {
		var (
			ctx            context.Context
			ingress        *networkingv1.Ingress
			namespacedName types.NamespacedName
			reconciler     *IngressReconciler
			eventRecorder  *record.FakeRecorder
			account        *uptimerobotv1.Account
			secret         *corev1.Secret
			pathTypePrefix networkingv1.PathType
			mgr            ctrl.Manager
			mgrCancel      context.CancelFunc
			mgrClient      client.Client
		)

		BeforeEach(func() {
			ctx = context.Background()
			pathTypePrefix = networkingv1.PathTypePrefix
			resourceName := fmt.Sprintf("test-ingress-%d", time.Now().UnixNano())
			namespacedName = types.NamespacedName{
				Name:      resourceName,
				Namespace: "default",
			}

			// Create Account so Monitor CRDs can be reconciled by the Monitor controller.
			// Monitors created by the Ingress controller reference this Account for UptimeRobot API credentials.
			account, secret = CreateAccount(ctx)
			ReconcileAccount(ctx, account)

			// Set up manager with field indexer for spec.sourceRef
			var err error
			mgr, err = ctrl.NewManager(cfg, ctrl.Options{
				Scheme:                 k8sClient.Scheme(),
				Metrics:                metricsserver.Options{BindAddress: "0"},
				HealthProbeBindAddress: "0",
				LeaderElection:         false,
			})
			Expect(err).NotTo(HaveOccurred())

			// Set up field indexer - this is normally done in SetupWithManager
			err = mgr.GetFieldIndexer().IndexField(ctx, &uptimerobotv1.Monitor{}, "spec.sourceRef", func(rawObj client.Object) []string {
				monitor := rawObj.(*uptimerobotv1.Monitor)
				if monitor.Spec.SourceRef == nil {
					return nil
				}
				return []string{monitor.Spec.SourceRef.Kind + "/" + monitor.Spec.SourceRef.Name}
			})
			Expect(err).NotTo(HaveOccurred())

			// Start manager in background
			var mgrCtx context.Context
			mgrCtx, mgrCancel = context.WithCancel(ctx)
			go func() {
				defer GinkgoRecover()
				_ = mgr.Start(mgrCtx)
			}()

			// Get client and event recorder before waiting (mgrClient must be set before Eventually uses it)
			eventRecorder = record.NewFakeRecorder(10)
			mgrClient = mgr.GetClient()

			// Wait for manager to be ready
			Eventually(func() error {
				return mgrClient.List(ctx, &uptimerobotv1.MonitorList{})
			}, time.Second*5, time.Millisecond*100).Should(Succeed())

			reconciler = &IngressReconciler{
				Client:   mgrClient,
				Scheme:   mgr.GetScheme(),
				Recorder: eventRecorder,
			}
		})

		AfterEach(func() {
			// Skip cleanup if mgrClient not initialized
			if mgrClient == nil {
				if mgrCancel != nil {
					mgrCancel()
				}
				return
			}

			mgrClientLocal := mgrClient
			cleanupClient := k8sClient
			if cleanupClient == nil {
				cleanupClient = mgrClientLocal
			}

			// Clean up ingress
			if ingress != nil {
				ing := &networkingv1.Ingress{}
				err := cleanupClient.Get(ctx, namespacedName, ing)
				if err == nil {
					// Ensure teardown is deterministic even if a finalizer was left behind.
					if len(ing.Finalizers) > 0 {
						ing.Finalizers = nil
						Expect(cleanupClient.Update(ctx, ing)).To(Succeed())
					}
					Expect(cleanupClient.Delete(ctx, ing)).To(Succeed())
					// Wait for deletion to complete
					Eventually(func() bool {
						err := cleanupClient.Get(ctx, namespacedName, &networkingv1.Ingress{})
						return errors.IsNotFound(err)
					}, time.Second*10, time.Millisecond*250).Should(BeTrue())
				}
			}

			// Clean up any monitors created by the ingress
			monitorList := &uptimerobotv1.MonitorList{}
			err := cleanupClient.List(ctx, monitorList, client.InNamespace("default"))
			if err == nil {
				for _, monitor := range monitorList.Items {
					if monitor.Spec.SourceRef != nil && monitor.Spec.SourceRef.Name == namespacedName.Name {
						Expect(cleanupClient.Delete(ctx, &monitor)).To(Succeed())
					}
				}
			}

			// Clean up account
			if account != nil {
				CleanupAccount(ctx, account, secret)
			}

			// Stop manager after cleanup so finalizer-driven deletes can complete.
			if mgrCancel != nil {
				mgrCancel()
			}
		})

		It("should create Monitor CRD when Ingress has uptimerobot.com/enabled=true annotation", func() {
			By("Creating an Ingress with uptimerobot.com/enabled=true annotation")
			ingress = buildTestIngress(namespacedName, pathTypePrefix, map[string]string{
				"uptimerobot.com/enabled":      "true",
				"uptimerobot.com/monitor.name": "Test Monitor",
			}, "example.com", "/api")
			Expect(mgrClient.Create(ctx, ingress)).To(Succeed())

			By("Reconciling the Ingress")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying Monitor CRD was created")
			monitor := &uptimerobotv1.Monitor{}
			Eventually(func() error {
				return mgrClient.Get(ctx, namespacedName, monitor)
			}, time.Second*5, time.Millisecond*250).Should(Succeed())

			Expect(monitor.Spec.Monitor.Name).To(Equal("Test Monitor"))
			Expect(monitor.Spec.Monitor.Status).To(Equal(uint8(1)))
			Expect(monitor.Spec.Prune).To(BeTrue())
			Expect(monitor.Spec.SourceRef).NotTo(BeNil())
			Expect(monitor.Spec.SourceRef.Kind).To(Equal("Ingress"))
			Expect(monitor.Spec.SourceRef.Name).To(Equal(namespacedName.Name))

			By("Verifying finalizer was added to Ingress")
			// mgrClient reads through the informer cache, which lags the write made
			// by Reconcile, so poll rather than asserting on a single read.
			Eventually(func() []string {
				if err := mgrClient.Get(ctx, namespacedName, ingress); err != nil {
					return nil
				}
				return ingress.Finalizers
			}, time.Second*5, time.Millisecond*250).Should(ContainElement("uptimerobot.com/finalizer"))
		})

		It("should derive Monitor URL from Ingress rules when not specified", func() {
			By("Creating an Ingress without explicit URL")
			ingress = buildTestIngress(namespacedName, pathTypePrefix, map[string]string{
				"uptimerobot.com/enabled": "true",
			}, "example.com", "/api")
			Expect(mgrClient.Create(ctx, ingress)).To(Succeed())

			By("Reconciling the Ingress")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying Monitor URL was derived from Ingress rules")
			monitor := &uptimerobotv1.Monitor{}
			Eventually(func() error {
				return mgrClient.Get(ctx, namespacedName, monitor)
			}, time.Second*5, time.Millisecond*250).Should(Succeed())

			// Default scheme should be http when no TLS
			Expect(monitor.Spec.Monitor.URL).To(Equal("http://example.com/api"))
		})

		It("should use https scheme when Ingress has TLS configuration", func() {
			By("Creating an Ingress with TLS")
			ingress = &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      namespacedName.Name,
					Namespace: namespacedName.Namespace,
					Annotations: map[string]string{
						"uptimerobot.com/enabled": "true",
					},
				},
				Spec: networkingv1.IngressSpec{
					TLS: []networkingv1.IngressTLS{
						{
							Hosts:      []string{"example.com"},
							SecretName: "tls-secret",
						},
					},
					Rules: []networkingv1.IngressRule{
						{
							Host: "example.com",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path:     "/secure",
											PathType: &pathTypePrefix,
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: "api-service",
													Port: networkingv1.ServiceBackendPort{Number: 443},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}
			Expect(mgrClient.Create(ctx, ingress)).To(Succeed())

			By("Reconciling the Ingress")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying Monitor URL uses https scheme")
			monitor := &uptimerobotv1.Monitor{}
			Eventually(func() error {
				return mgrClient.Get(ctx, namespacedName, monitor)
			}, time.Second*5, time.Millisecond*250).Should(Succeed())

			Expect(monitor.Spec.Monitor.URL).To(Equal("https://example.com/secure"))
		})

		It("should ignore root path (/) in URL construction", func() {
			By("Creating an Ingress with root path")
			ingress = &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      namespacedName.Name,
					Namespace: namespacedName.Namespace,
					Annotations: map[string]string{
						"uptimerobot.com/enabled": "true",
					},
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host: "example.com",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path:     "/",
											PathType: &pathTypePrefix,
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: "frontend-service",
													Port: networkingv1.ServiceBackendPort{Number: 80},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}
			Expect(mgrClient.Create(ctx, ingress)).To(Succeed())

			By("Reconciling the Ingress")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying Monitor URL omits root path")
			monitor := &uptimerobotv1.Monitor{}
			Eventually(func() error {
				return mgrClient.Get(ctx, namespacedName, monitor)
			}, time.Second*5, time.Millisecond*250).Should(Succeed())

			Expect(monitor.Spec.Monitor.URL).To(Equal("http://example.com"))
		})

		It("should allow custom scheme/host/path annotations to override defaults", func() {
			By("Creating an Ingress with custom scheme, host, and path annotations")
			ingress = &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      namespacedName.Name,
					Namespace: namespacedName.Namespace,
					Annotations: map[string]string{
						"uptimerobot.com/enabled":        "true",
						"uptimerobot.com/monitor.scheme": "https",
						"uptimerobot.com/monitor.host":   "custom.example.com",
						"uptimerobot.com/monitor.path":   "/custom/path",
					},
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host: "original.example.com",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path:     "/original",
											PathType: &pathTypePrefix,
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: "api-service",
													Port: networkingv1.ServiceBackendPort{Number: 80},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}
			Expect(mgrClient.Create(ctx, ingress)).To(Succeed())

			By("Reconciling the Ingress")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying Monitor URL uses custom values")
			monitor := &uptimerobotv1.Monitor{}
			Eventually(func() error {
				return mgrClient.Get(ctx, namespacedName, monitor)
			}, time.Second*5, time.Millisecond*250).Should(Succeed())

			Expect(monitor.Spec.Monitor.URL).To(Equal("https://custom.example.com/custom/path"))
		})

		It("should delete Monitor CRD when Ingress is deleted", func() {
			By("Creating and reconciling an Ingress")
			ingress = buildTestIngress(namespacedName, pathTypePrefix, map[string]string{
				"uptimerobot.com/enabled": "true",
			}, "example.com", "/api")
			Expect(mgrClient.Create(ctx, ingress)).To(Succeed())
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying Monitor was created")
			monitor := &uptimerobotv1.Monitor{}
			Eventually(func() error {
				return mgrClient.Get(ctx, namespacedName, monitor)
			}, time.Second*5, time.Millisecond*250).Should(Succeed())

			By("Deleting the Ingress")
			Expect(mgrClient.Delete(ctx, ingress)).To(Succeed())

			By("Reconciling the deletion")
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying Monitor was deleted")
			Eventually(func() bool {
				err := mgrClient.Get(ctx, namespacedName, monitor)
				return errors.IsNotFound(err)
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			By("Verifying Ingress was fully deleted")
			Eventually(func() bool {
				err := mgrClient.Get(ctx, namespacedName, ingress)
				return errors.IsNotFound(err)
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())
		})

		It("should update Monitor when Ingress annotations change", func() {
			By("Creating and reconciling an Ingress")
			ingress = buildTestIngress(namespacedName, pathTypePrefix, map[string]string{
				"uptimerobot.com/enabled":      "true",
				"uptimerobot.com/monitor.name": "Original Name",
			}, "example.com", "/api")
			Expect(mgrClient.Create(ctx, ingress)).To(Succeed())
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying initial Monitor")
			monitor := &uptimerobotv1.Monitor{}
			Eventually(func() error {
				return mgrClient.Get(ctx, namespacedName, monitor)
			}, time.Second*5, time.Millisecond*250).Should(Succeed())
			Expect(monitor.Spec.Monitor.Name).To(Equal("Original Name"))

			By("Updating Ingress annotations")
			Expect(mgrClient.Get(ctx, namespacedName, ingress)).To(Succeed())
			ingress.Annotations["uptimerobot.com/monitor.name"] = "Updated Name"
			Expect(mgrClient.Update(ctx, ingress)).To(Succeed())

			By("Reconciling the update and verifying Monitor was updated")
			Eventually(func() string {
				_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
				if err != nil {
					return ""
				}
				if err := mgrClient.Get(ctx, namespacedName, monitor); err != nil {
					return ""
				}
				return monitor.Spec.Monitor.Name
			}, time.Second*10, time.Millisecond*250).Should(Equal("Updated Name"))
		})

		It("should ignore Ingress without uptimerobot.com/ annotations", func() {
			By("Creating an Ingress without any uptimerobot.com/ annotations")
			ingress = buildTestIngress(namespacedName, pathTypePrefix, map[string]string{
				"kubernetes.io/ingress.class": "nginx",
			}, "example.com", "/api")
			Expect(mgrClient.Create(ctx, ingress)).To(Succeed())

			By("Reconciling the Ingress")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying no Monitor was created")
			monitor := &uptimerobotv1.Monitor{}
			Consistently(func() bool {
				err := mgrClient.Get(ctx, namespacedName, monitor)
				return errors.IsNotFound(err)
			}, time.Second*2, time.Millisecond*250).Should(BeTrue())

			By("Verifying no finalizer was added")
			Expect(mgrClient.Get(ctx, namespacedName, ingress)).To(Succeed())
			Expect(ingress.Finalizers).To(BeEmpty())
		})

		It("should remove Monitor and finalizer when enabled=false", func() {
			By("Creating and reconciling an Ingress with enabled=true")
			ingress = buildTestIngress(namespacedName, pathTypePrefix, map[string]string{
				"uptimerobot.com/enabled": "true",
			}, "example.com", "/api")
			Expect(mgrClient.Create(ctx, ingress)).To(Succeed())
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying Monitor was created")
			monitor := &uptimerobotv1.Monitor{}
			Eventually(func() error {
				return mgrClient.Get(ctx, namespacedName, monitor)
			}, time.Second*5, time.Millisecond*250).Should(Succeed())

			By("Updating Ingress to enabled=false")
			Expect(mgrClient.Get(ctx, namespacedName, ingress)).To(Succeed())
			ingress.Annotations["uptimerobot.com/enabled"] = "false"
			Expect(mgrClient.Update(ctx, ingress)).To(Succeed())

			By("Reconciling the update")
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying Monitor was deleted")
			Eventually(func() bool {
				err := mgrClient.Get(ctx, namespacedName, monitor)
				return errors.IsNotFound(err)
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			By("Verifying finalizer was removed")
			Expect(mgrClient.Get(ctx, namespacedName, ingress)).To(Succeed())
			Expect(ingress.Finalizers).To(BeEmpty())
		})

		It("should handle multiple Ingress rules correctly", func() {
			By("Creating an Ingress with multiple rules")
			ingress = &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      namespacedName.Name,
					Namespace: namespacedName.Namespace,
					Annotations: map[string]string{
						"uptimerobot.com/enabled": "true",
					},
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host: "first.example.com",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path:     "/api",
											PathType: &pathTypePrefix,
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: "api-service",
													Port: networkingv1.ServiceBackendPort{Number: 80},
												},
											},
										},
									},
								},
							},
						},
						{
							Host: "second.example.com",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path:     "/web",
											PathType: &pathTypePrefix,
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: "web-service",
													Port: networkingv1.ServiceBackendPort{Number: 80},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}
			Expect(mgrClient.Create(ctx, ingress)).To(Succeed())

			By("Reconciling the Ingress")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying Monitor URL uses the first rule")
			monitor := &uptimerobotv1.Monitor{}
			Eventually(func() error {
				return mgrClient.Get(ctx, namespacedName, monitor)
			}, time.Second*5, time.Millisecond*250).Should(Succeed())

			// The controller uses the first rule
			Expect(monitor.Spec.Monitor.URL).To(Equal("http://first.example.com/api"))
		})

		It("should record warning event on annotation decode error during sync", func() {
			By("Creating an Ingress with invalid annotation that will cause decode error")
			ingress = buildTestIngress(namespacedName, pathTypePrefix, map[string]string{
				"uptimerobot.com/enabled":          "true",
				"uptimerobot.com/monitor.interval": "invalid-duration", // This will cause a decode error
			}, "example.com", "/api")
			Expect(mgrClient.Create(ctx, ingress)).To(Succeed())

			By("Reconciling the Ingress")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).To(HaveOccurred())

			By("Verifying warning event was recorded")
			Eventually(eventRecorder.Events, time.Second*2, time.Millisecond*100).Should(Receive(And(ContainSubstring("Warning"), ContainSubstring("Sync"))))
		})

		It("should allow explicit monitor.url annotation to override URL derivation", func() {
			By("Creating an Ingress with explicit monitor.url annotation")
			ingress = buildTestIngress(namespacedName, pathTypePrefix, map[string]string{
				"uptimerobot.com/enabled":     "true",
				"uptimerobot.com/monitor.url": "https://custom-url.example.com/check",
			}, "ignored.example.com", "/ignored")
			Expect(mgrClient.Create(ctx, ingress)).To(Succeed())

			By("Reconciling the Ingress")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying Monitor uses explicit URL")
			monitor := &uptimerobotv1.Monitor{}
			Eventually(func() error {
				return mgrClient.Get(ctx, namespacedName, monitor)
			}, time.Second*5, time.Millisecond*250).Should(Succeed())

			// When monitor.url is explicitly set, it should be used as-is
			// The controller doesn't overwrite it when the annotation exists
			Expect(monitor.Spec.Monitor.URL).To(Equal("https://custom-url.example.com/check"))
		})

		It("should recreate Monitor when manually deleted and enabled=true", func() {
			By("Creating and reconciling an Ingress with enabled=true")
			ingress = &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      namespacedName.Name,
					Namespace: namespacedName.Namespace,
					Annotations: map[string]string{
						"uptimerobot.com/enabled": "true",
					},
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host: "example.com",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path:     "/api",
											PathType: &pathTypePrefix,
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: "api-service",
													Port: networkingv1.ServiceBackendPort{Number: 80},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}
			Expect(mgrClient.Create(ctx, ingress)).To(Succeed())

			By("Reconciling to create the Monitor")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying Monitor was created")
			monitor := &uptimerobotv1.Monitor{}
			Eventually(func() error {
				return mgrClient.Get(ctx, namespacedName, monitor)
			}, time.Second*5, time.Millisecond*250).Should(Succeed())

			originalUID := monitor.UID

			By("Manually deleting the Monitor (simulating external deletion)")
			Expect(mgrClient.Delete(ctx, monitor)).To(Succeed())

			By("Waiting for Monitor to be deleted")
			Eventually(func() bool {
				err := mgrClient.Get(ctx, namespacedName, monitor)
				return errors.IsNotFound(err)
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			By("Reconciling the Ingress again (triggered by Monitor watch)")
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying Monitor was recreated")
			newMonitor := &uptimerobotv1.Monitor{}
			Eventually(func() error {
				return mgrClient.Get(ctx, namespacedName, newMonitor)
			}, time.Second*5, time.Millisecond*250).Should(Succeed())

			// Verify it's a new Monitor (different UID)
			Expect(newMonitor.UID).NotTo(Equal(originalUID))
			Expect(newMonitor.Spec.SourceRef).NotTo(BeNil())
			Expect(newMonitor.Spec.SourceRef.Kind).To(Equal("Ingress"))
			Expect(newMonitor.Spec.SourceRef.Name).To(Equal(namespacedName.Name))
		})
	})
})

// buildTestIngress creates an Ingress with the given annotations and a single rule.
func buildTestIngress(nn types.NamespacedName, pathType networkingv1.PathType, annotations map[string]string, host, path string) *networkingv1.Ingress {
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        nn.Name,
			Namespace:   nn.Namespace,
			Annotations: annotations,
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: host,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     path,
									PathType: &pathType,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "api-service",
											Port: networkingv1.ServiceBackendPort{Number: 80},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}
