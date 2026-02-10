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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

//+kubebuilder:webhook:path=/validate-uptimerobot-com-v1alpha1-account,mutating=false,failurePolicy=fail,sideEffects=None,groups=uptimerobot.com,resources=accounts,verbs=create;update,versions=v1alpha1,name=vaccount.uptimerobot.com,admissionReviewVersions=v1

func (r *Account) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		WithValidator(&AccountCustomValidator{
			Client: mgr.GetClient(),
		}).
		Complete()
}

// AccountCustomValidator validates Account admission requests.
// +kubebuilder:object:generate=false
type AccountCustomValidator struct {
	Client client.Reader
}

var _ webhook.CustomValidator = &AccountCustomValidator{}

func (v *AccountCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	account, ok := obj.(*Account)
	if !ok {
		return nil, fmt.Errorf("expected Account but got %T", obj)
	}

	return nil, v.validateUniqueDefault(ctx, account)
}

func (v *AccountCustomValidator) ValidateUpdate(ctx context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	account, ok := newObj.(*Account)
	if !ok {
		return nil, fmt.Errorf("expected Account but got %T", newObj)
	}

	return nil, v.validateUniqueDefault(ctx, account)
}

func (v *AccountCustomValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (v *AccountCustomValidator) validateUniqueDefault(ctx context.Context, account *Account) error {
	if !account.Spec.IsDefault {
		return nil
	}

	list := &AccountList{}
	if err := v.Client.List(ctx, list); err != nil {
		return fmt.Errorf("listing accounts for default validation: %w", err)
	}

	defaultCount := 0
	for _, existing := range list.Items {
		if !existing.Spec.IsDefault {
			continue
		}
		if existing.Name == account.Name {
			continue
		}
		defaultCount++
	}

	if defaultCount == 0 {
		return nil
	}

	return apierrors.NewInvalid(
		schema.GroupKind{Group: GroupVersion.Group, Kind: "Account"},
		account.Name,
		field.ErrorList{
			field.Forbidden(
				field.NewPath("spec", "isDefault"),
				"at most one Account can have spec.isDefault=true",
			),
		},
	)
}
