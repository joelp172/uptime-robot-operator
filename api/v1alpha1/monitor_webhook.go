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

package v1alpha1

import (
	"context"
	"fmt"
	"net/url"

	"github.com/joelp172/uptime-robot-operator/internal/uptimerobot/urtypes"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

//+kubebuilder:webhook:path=/validate-uptimerobot-com-v1alpha1-monitor,mutating=false,failurePolicy=fail,sideEffects=None,groups=uptimerobot.com,resources=monitors,verbs=create;update,versions=v1alpha1,name=vmonitor.uptimerobot.com,admissionReviewVersions=v1

func (r *Monitor) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, r).
		WithValidator(&MonitorCustomValidator{
			Client: mgr.GetClient(),
		}).
		Complete()
}

// MonitorCustomValidator validates Monitor admission requests.
// +kubebuilder:object:generate=false
type MonitorCustomValidator struct {
	Client client.Reader
}

var _ admission.Validator[*Monitor] = &MonitorCustomValidator{}

func (v *MonitorCustomValidator) ValidateCreate(ctx context.Context, monitor *Monitor) (admission.Warnings, error) {
	return nil, v.validate(ctx, monitor, nil)
}

func (v *MonitorCustomValidator) ValidateUpdate(ctx context.Context, old, monitor *Monitor) (admission.Warnings, error) {
	return nil, v.validate(ctx, monitor, old)
}

func (v *MonitorCustomValidator) ValidateDelete(_ context.Context, _ *Monitor) (admission.Warnings, error) {
	return nil, nil
}

func (v *MonitorCustomValidator) validate(ctx context.Context, monitor *Monitor, old *Monitor) error {
	var errs field.ErrorList

	// Validate account reference.
	if ferr := validateAccountRef(ctx, v.Client, monitor.Spec.Account.Name,
		field.NewPath("spec", "account", "name"), "Monitor"); ferr != nil {
		errs = append(errs, ferr)
	}

	// Validate named contact references (cluster-scoped).
	for i, ref := range monitor.Spec.Contacts {
		if ref.Name == "" {
			continue
		}
		contact := &Contact{}
		if err := v.Client.Get(ctx, client.ObjectKey{Name: ref.Name}, contact); err != nil {
			if apierrors.IsNotFound(err) {
				errs = append(errs, field.Invalid(
					field.NewPath("spec", "contacts").Index(i).Child("name"),
					ref.Name,
					fmt.Sprintf("Contact %q not found", ref.Name),
				))
			} else {
				errs = append(errs, field.InternalError(
					field.NewPath("spec", "contacts").Index(i).Child("name"),
					fmt.Errorf("looking up Contact %q: %w", ref.Name, err),
				))
			}
		}
	}

	// Prevent monitor type changes on update.
	if old != nil && old.Spec.Monitor.Type != 0 && monitor.Spec.Monitor.Type != 0 &&
		old.Spec.Monitor.Type != monitor.Spec.Monitor.Type {
		errs = append(errs, field.Forbidden(
			field.NewPath("spec", "monitor", "type"),
			fmt.Sprintf("monitor type is immutable: cannot change from %q to %q; delete and recreate the Monitor",
				old.Spec.Monitor.Type, monitor.Spec.Monitor.Type),
		))
	}

	// Validate URL format for types that require a URL.
	if monitor.Spec.Monitor.URL != "" {
		monType := monitor.Spec.Monitor.Type
		if monType == urtypes.TypeHTTPS || monType == urtypes.TypeKeyword {
			parsed, parseErr := url.ParseRequestURI(monitor.Spec.Monitor.URL)
			if parseErr != nil {
				errs = append(errs, field.Invalid(
					field.NewPath("spec", "monitor", "url"),
					monitor.Spec.Monitor.URL,
					fmt.Sprintf("invalid URL: %v", parseErr),
				))
			} else if parsed.Scheme != "https" {
				errs = append(errs, field.Invalid(
					field.NewPath("spec", "monitor", "url"),
					monitor.Spec.Monitor.URL,
					"URL must use HTTPS for HTTPS/Keyword monitor types",
				))
			} else if parsed.Host == "" {
				errs = append(errs, field.Invalid(
					field.NewPath("spec", "monitor", "url"),
					monitor.Spec.Monitor.URL,
					"URL must include a host for HTTPS/Keyword monitor types",
				))
			}
		}
	}

	if len(errs) > 0 {
		return invalidErr("Monitor", monitor.Name, errs)
	}
	return nil
}
