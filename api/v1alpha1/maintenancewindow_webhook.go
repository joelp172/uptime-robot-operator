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
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

//+kubebuilder:webhook:path=/validate-uptimerobot-com-v1alpha1-maintenancewindow,mutating=false,failurePolicy=fail,sideEffects=None,groups=uptimerobot.com,resources=maintenancewindows,verbs=create;update,versions=v1alpha1,name=vmaintenancewindow.uptimerobot.com,admissionReviewVersions=v1

func (r *MaintenanceWindow) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, r).
		WithValidator(&MaintenanceWindowCustomValidator{
			Client: mgr.GetClient(),
		}).
		Complete()
}

// MaintenanceWindowCustomValidator validates MaintenanceWindow admission requests.
// +kubebuilder:object:generate=false
type MaintenanceWindowCustomValidator struct {
	Client client.Reader
}

var _ admission.Validator[*MaintenanceWindow] = &MaintenanceWindowCustomValidator{}

func (v *MaintenanceWindowCustomValidator) ValidateCreate(ctx context.Context, mw *MaintenanceWindow) (admission.Warnings, error) {
	return nil, v.validate(ctx, mw)
}

func (v *MaintenanceWindowCustomValidator) ValidateUpdate(ctx context.Context, _, mw *MaintenanceWindow) (admission.Warnings, error) {
	return nil, v.validate(ctx, mw)
}

func (v *MaintenanceWindowCustomValidator) ValidateDelete(_ context.Context, _ *MaintenanceWindow) (admission.Warnings, error) {
	return nil, nil
}

func (v *MaintenanceWindowCustomValidator) validate(ctx context.Context, mw *MaintenanceWindow) error {
	var errs field.ErrorList

	// Validate account reference.
	if ferr := validateAccountRef(ctx, v.Client, mw.Spec.Account.Name,
		field.NewPath("spec", "account", "name"), "MaintenanceWindow"); ferr != nil {
		errs = append(errs, ferr)
	}

	// Validate monitor references exist in the same namespace.
	for i, ref := range mw.Spec.MonitorRefs {
		if ref.Name == "" {
			continue
		}
		monitor := &Monitor{}
		if err := v.Client.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: mw.Namespace}, monitor); err != nil {
			if apierrors.IsNotFound(err) {
				errs = append(errs, field.Invalid(
					field.NewPath("spec", "monitorRefs").Index(i).Child("name"),
					ref.Name,
					fmt.Sprintf("Monitor %q not found in namespace %q", ref.Name, mw.Namespace),
				))
			} else {
				errs = append(errs, field.InternalError(
					field.NewPath("spec", "monitorRefs").Index(i).Child("name"),
					fmt.Errorf("looking up Monitor %q: %w", ref.Name, err),
				))
			}
		}
	}

	// Validate startDate is in the future for "once" interval.
	if mw.Spec.Interval == "once" && mw.Spec.StartDate != "" {
		parsed, err := time.Parse("2006-01-02", mw.Spec.StartDate)
		if err != nil {
			errs = append(errs, field.Invalid(
				field.NewPath("spec", "startDate"),
				mw.Spec.StartDate,
				fmt.Sprintf("invalid date format (expected YYYY-MM-DD): %v", err),
			))
		} else {
			// Compare only date portions using UTC.
			today := time.Now().UTC().Truncate(24 * time.Hour)
			if parsed.Before(today) {
				errs = append(errs, field.Invalid(
					field.NewPath("spec", "startDate"),
					mw.Spec.StartDate,
					"startDate must be today or in the future for a once-only maintenance window",
				))
			}
		}
	}

	if len(errs) > 0 {
		return invalidErr("MaintenanceWindow", mw.Name, errs)
	}
	return nil
}
