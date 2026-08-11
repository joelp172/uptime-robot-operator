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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

//+kubebuilder:webhook:path=/validate-uptimerobot-com-v1alpha1-monitorgroup,mutating=false,failurePolicy=fail,sideEffects=None,groups=uptimerobot.com,resources=monitorgroups,verbs=create;update,versions=v1alpha1,name=vmonitorgroup.uptimerobot.com,admissionReviewVersions=v1

func (r *MonitorGroup) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, r).
		WithValidator(&MonitorGroupCustomValidator{
			Client: mgr.GetClient(),
		}).
		Complete()
}

// MonitorGroupCustomValidator validates MonitorGroup admission requests.
// +kubebuilder:object:generate=false
type MonitorGroupCustomValidator struct {
	Client client.Reader
}

var _ admission.Validator[*MonitorGroup] = &MonitorGroupCustomValidator{}

func (v *MonitorGroupCustomValidator) ValidateCreate(ctx context.Context, mg *MonitorGroup) (admission.Warnings, error) {
	return nil, v.validate(ctx, mg)
}

func (v *MonitorGroupCustomValidator) ValidateUpdate(ctx context.Context, _, mg *MonitorGroup) (admission.Warnings, error) {
	return nil, v.validate(ctx, mg)
}

func (v *MonitorGroupCustomValidator) ValidateDelete(_ context.Context, _ *MonitorGroup) (admission.Warnings, error) {
	return nil, nil
}

func (v *MonitorGroupCustomValidator) validate(ctx context.Context, mg *MonitorGroup) error {
	var errs field.ErrorList

	// Validate account reference.
	if ferr := validateAccountRef(ctx, v.Client, mg.Spec.Account.Name,
		field.NewPath("spec", "account", "name"), "MonitorGroup"); ferr != nil {
		errs = append(errs, ferr)
	}

	// Validate monitor references exist in the same namespace.
	for i, ref := range mg.Spec.Monitors {
		if ref.Name == "" {
			continue
		}
		monitor := &Monitor{}
		if err := v.Client.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: mg.Namespace}, monitor); err != nil {
			if apierrors.IsNotFound(err) {
				errs = append(errs, field.Invalid(
					field.NewPath("spec", "monitors").Index(i).Child("name"),
					ref.Name,
					fmt.Sprintf("Monitor %q not found in namespace %q", ref.Name, mg.Namespace),
				))
			} else {
				errs = append(errs, field.InternalError(
					field.NewPath("spec", "monitors").Index(i).Child("name"),
					fmt.Errorf("looking up Monitor %q: %w", ref.Name, err),
				))
			}
		}
	}

	if len(errs) > 0 {
		return invalidErr("MonitorGroup", mg.Name, errs)
	}
	return nil
}
