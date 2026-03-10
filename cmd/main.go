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

package main

import (
	"context"
	"crypto/tls"
	"flag"
	"net/http"
	"os"
	"path/filepath"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	uptimerobotv1 "github.com/joelp172/uptime-robot-operator/api/v1alpha1"
	"github.com/joelp172/uptime-robot-operator/internal/controller"
	"github.com/joelp172/uptime-robot-operator/internal/metrics"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

// disabledWebhookServer disables webhook serving when webhooks are not enabled.
type disabledWebhookServer struct{}

func (disabledWebhookServer) NeedLeaderElection() bool { return false }

func (disabledWebhookServer) Register(_ string, _ http.Handler) {}

func (disabledWebhookServer) Start(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (disabledWebhookServer) StartedChecker() healthz.Checker {
	return func(_ *http.Request) error { return nil }
}

func (disabledWebhookServer) WebhookMux() *http.ServeMux {
	return http.NewServeMux()
}

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(uptimerobotv1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

// nolint:gocyclo
func main() {
	var metricsAddr string
	var webhookCertPath, webhookCertName, webhookCertKey string
	var enableWebhooks bool
	var enableLeaderElection bool
	var probeAddr string
	var enableHTTP2 bool
	var tlsOpts []func(*tls.Config)
	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 to enable metrics over HTTP, or leave as 0 to disable the metrics service.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&webhookCertPath, "webhook-cert-path", "", "The directory that contains the webhook certificate.")
	flag.StringVar(&webhookCertName, "webhook-cert-name", "tls.crt", "The name of the webhook certificate file.")
	flag.StringVar(&webhookCertKey, "webhook-cert-key", "tls.key", "The name of the webhook key file.")
	flag.BoolVar(&enableWebhooks, "enable-webhooks", false, "Enable validating webhooks.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	flag.StringVar(&controller.ClusterResourceNamespace, "cluster-resource-namespace", controller.ClusterResourceNamespace,
		"Namespace to store resources owned by cluster scoped resources",
	)
	flag.StringVar(&controller.IngressAnnotationPrefix, "ingress-annotation-prefix", controller.IngressAnnotationPrefix,
		"Ingress annotation prefix",
	)
	opts := zap.Options{
		Development: false,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Register custom Prometheus metrics
	metrics.RegisterMetrics()
	setupLog.Info("registered custom Prometheus metrics")

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	// Create watcher for webhook certificates
	var webhookCertWatcher *certwatcher.CertWatcher
	webhookServer := webhook.Server(disabledWebhookServer{})

	if enableWebhooks {
		// Initial webhook TLS options
		webhookTLSOpts := tlsOpts

		if len(webhookCertPath) > 0 {
			setupLog.Info("Initializing webhook certificate watcher using provided certificates",
				"webhook-cert-path", webhookCertPath, "webhook-cert-name", webhookCertName, "webhook-cert-key", webhookCertKey)

			var err error
			webhookCertWatcher, err = certwatcher.New(
				filepath.Join(webhookCertPath, webhookCertName),
				filepath.Join(webhookCertPath, webhookCertKey),
			)
			if err != nil {
				setupLog.Error(err, "Failed to initialize webhook certificate watcher")
				os.Exit(1)
			}

			webhookTLSOpts = append(webhookTLSOpts, func(config *tls.Config) {
				config.GetCertificate = webhookCertWatcher.GetCertificate
			})
		}

		webhookServer = webhook.NewServer(webhook.Options{
			TLSOpts: webhookTLSOpts,
		})
	}

	// Metrics endpoint is enabled in 'config/default/kustomization.yaml'. The Metrics options configure the server.
	// More info:
	// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.2/pkg/metrics/server
	// - https://book.kubebuilder.io/reference/metrics.html
	metricsServerOptions := metricsserver.Options{
		BindAddress: metricsAddr,
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "9926c7f2.uptimerobot.com",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controller.MonitorReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("monitor-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Monitor")
		os.Exit(1)
	}
	if err = (&controller.ContactReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("contact-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Contact")
		os.Exit(1)
	}
	if err = (&controller.AccountReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("account-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Account")
		os.Exit(1)
	}
	if err = (&controller.IngressReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("uptime-robot-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Ingress")
		os.Exit(1)
	}
	if err = (&controller.MaintenanceWindowReconciler{
		Client:    mgr.GetClient(),
		APIReader: mgr.GetAPIReader(),
		Scheme:    mgr.GetScheme(),
		Recorder:  mgr.GetEventRecorderFor("maintenancewindow-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MaintenanceWindow")
		os.Exit(1)
	}
	if err = (&controller.MonitorGroupReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("monitorgroup-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MonitorGroup")
		os.Exit(1)
	}
	if err = (&controller.SlackIntegrationReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("slackintegration-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SlackIntegration")
		os.Exit(1)
	}
	if enableWebhooks {
		if err = (&uptimerobotv1.Account{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "Account")
			os.Exit(1)
		}
		if err = (&uptimerobotv1.Contact{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "Contact")
			os.Exit(1)
		}
		if err = (&uptimerobotv1.Monitor{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "Monitor")
			os.Exit(1)
		}
		if err = (&uptimerobotv1.MaintenanceWindow{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "MaintenanceWindow")
			os.Exit(1)
		}
		if err = (&uptimerobotv1.MonitorGroup{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "MonitorGroup")
			os.Exit(1)
		}
		if err = (&uptimerobotv1.SlackIntegration{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "SlackIntegration")
			os.Exit(1)
		}
	} else {
		setupLog.Info("webhooks are disabled: webhook server will not listen")
	}
	//+kubebuilder:scaffold:builder

	if webhookCertWatcher != nil {
		setupLog.Info("Adding webhook certificate watcher to manager")
		if err := mgr.Add(webhookCertWatcher); err != nil {
			setupLog.Error(err, "unable to add webhook certificate watcher to manager")
			os.Exit(1)
		}
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
