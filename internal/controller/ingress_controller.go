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
	"net/url"
	"strconv"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	uptimerobotv1 "github.com/joelp172/uptime-robot-operator/api/v1"
	"github.com/joelp172/uptime-robot-operator/internal/util"
	"github.com/knadh/koanf/maps"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var IngressAnnotationPrefix = "uptimerobot.com/"

// IngressReconciler reconciles a Ingress object
type IngressReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.2/pkg/reconcile
func (r *IngressReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	ingress := &networkingv1.Ingress{}
	if err := r.Get(ctx, req.NamespacedName, ingress); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	list, err := r.findMonitors(ctx, ingress)
	if err != nil {
		return ctrl.Result{}, err
	}

	const myFinalizerName = "uptimerobot.com/finalizer"
	if !ingress.DeletionTimestamp.IsZero() {
		// Object is being deleted
		if controllerutil.ContainsFinalizer(ingress, myFinalizerName) {
			for _, monitor := range list.Items {
				if err := r.Delete(ctx, &monitor); err != nil {
					return ctrl.Result{}, err
				}
			}

			controllerutil.RemoveFinalizer(ingress, myFinalizerName)
			if err := r.Update(ctx, ingress); err != nil {
				return ctrl.Result{}, err
			}
		}

		return ctrl.Result{}, nil
	}

	annotations := r.getMatchingAnnotations(ingress)

	var enabled bool
	if val, ok := annotations["enabled"]; ok {
		if enabled, err = strconv.ParseBool(val); err != nil {
			return ctrl.Result{}, err
		}
	}

	var create bool
	if !enabled {
		if controllerutil.ContainsFinalizer(ingress, myFinalizerName) {
			// Delete existing Monitor
			for _, monitor := range list.Items {
				if err := r.Delete(ctx, &monitor); err != nil {
					return ctrl.Result{}, err
				}
			}

			controllerutil.RemoveFinalizer(ingress, myFinalizerName)
			if err := r.Update(ctx, ingress); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	} else if len(list.Items) == 0 {
		// Create new Monitor
		create = true
		list.Items = append(list.Items, uptimerobotv1.Monitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ingress.Name,
				Namespace: req.Namespace,
			},
			Spec: uptimerobotv1.MonitorSpec{
				SourceRef: &corev1.TypedLocalObjectReference{
					Kind: ingress.Kind,
					Name: ingress.Name,
				},
			},
		})
	}

	for _, monitor := range list.Items {
		if err := r.updateValues(ingress, &monitor, annotations); err != nil {
			r.Recorder.Event(ingress, "Warning", "Sync", err.Error())
			return ctrl.Result{}, err
		}

		if create {
			if err := r.Create(ctx, &monitor); err != nil {
				return ctrl.Result{}, err
			}
		} else {
			if err := r.Update(ctx, &monitor); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	if !controllerutil.ContainsFinalizer(ingress, myFinalizerName) {
		controllerutil.AddFinalizer(ingress, myFinalizerName)
		if err := r.Update(ctx, ingress); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *IngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1.Ingress{}, builder.WithPredicates(
			predicate.Or(predicate.GenerationChangedPredicate{}, predicate.AnnotationChangedPredicate{}),
		)).
		Named("ingress").
		Complete(r)
}

func (r *IngressReconciler) findMonitors(ctx context.Context, ingress *networkingv1.Ingress) (*uptimerobotv1.MonitorList, error) {
	list := &uptimerobotv1.MonitorList{}
	err := r.Client.List(ctx, list, &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.sourceRef", ingress.Kind+"/"+ingress.Name),
	})
	if err != nil {
		return list, err
	}
	return list, nil
}

func (r *IngressReconciler) countMatchingAnnotations(ingress *networkingv1.Ingress) uint {
	var count uint
	for k := range ingress.Annotations {
		if strings.HasPrefix(k, IngressAnnotationPrefix) {
			count++
		}
	}
	return count
}

func (r *IngressReconciler) getMatchingAnnotations(ingress *networkingv1.Ingress) map[string]string {
	count := r.countMatchingAnnotations(ingress)
	if count == 0 {
		return nil
	}

	annotations := make(map[string]string, count)
	for k, v := range ingress.Annotations {
		if strings.HasPrefix(k, IngressAnnotationPrefix) {
			annotations[strings.TrimPrefix(k, IngressAnnotationPrefix)] = v
		}
	}
	return annotations
}

func (r *IngressReconciler) updateValues(ingress *networkingv1.Ingress, monitor *uptimerobotv1.Monitor, annotations map[string]string) error {
	monitor.Spec.Monitor.Name = ingress.Name
	if _, ok := annotations["monitor.url"]; !ok {
		if len(ingress.Spec.Rules) != 0 {
			var u url.URL
			if u.Scheme, ok = annotations["monitor.scheme"]; !ok {
				if len(ingress.Spec.TLS) == 0 {
					u.Scheme = "http"
				} else {
					u.Scheme = "https"
				}
			}
			rule := ingress.Spec.Rules[0]
			if u.Host, ok = annotations["monitor.host"]; !ok {
				u.Host = rule.Host
			}
			if u.Path, ok = annotations["monitor.path"]; !ok && len(rule.HTTP.Paths) != 0 {
				if path := rule.HTTP.Paths[0].Path; path != "/" {
					u.Path = path
				}
			}
			monitor.Spec.Monitor.URL = u.String()
		}
	}
	delete(annotations, "enabled")
	delete(annotations, "monitor.url")
	delete(annotations, "monitor.scheme")
	delete(annotations, "monitor.host")
	delete(annotations, "monitor.path")

	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			util.DecodeHookMetav1Duration,
			mapstructure.TextUnmarshallerHookFunc(),
		),
		ErrorUnused:      true,
		WeaklyTypedInput: true,
		Result:           &monitor.Spec,
	})
	if err != nil {
		return err
	}

	expanded := make(map[string]any, len(annotations))
	for k, v := range annotations {
		expanded[k] = v
	}
	expanded = maps.Unflatten(expanded, ".")
	return dec.Decode(expanded)
}
