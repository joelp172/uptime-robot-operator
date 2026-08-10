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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

//+kubebuilder:webhook:path=/validate-uptimerobot-com-v1alpha1-slackintegration,mutating=false,failurePolicy=fail,sideEffects=None,groups=uptimerobot.com,resources=slackintegrations,verbs=create;update,versions=v1alpha1,name=vslackintegration.uptimerobot.com,admissionReviewVersions=v1

func (r *SlackIntegration) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, r).
		WithValidator(&SlackIntegrationCustomValidator{
			Client: mgr.GetClient(),
		}).
		Complete()
}

// SlackIntegrationCustomValidator validates SlackIntegration admission requests.
// +kubebuilder:object:generate=false
type SlackIntegrationCustomValidator struct {
	Client client.Reader
}

var _ admission.Validator[*SlackIntegration] = &SlackIntegrationCustomValidator{}

func (v *SlackIntegrationCustomValidator) ValidateCreate(ctx context.Context, si *SlackIntegration) (admission.Warnings, error) {
	return nil, v.validate(ctx, si)
}

func (v *SlackIntegrationCustomValidator) ValidateUpdate(ctx context.Context, _, si *SlackIntegration) (admission.Warnings, error) {
	return nil, v.validate(ctx, si)
}

func (v *SlackIntegrationCustomValidator) ValidateDelete(_ context.Context, _ *SlackIntegration) (admission.Warnings, error) {
	return nil, nil
}

func (v *SlackIntegrationCustomValidator) validate(ctx context.Context, si *SlackIntegration) error {
	var errs field.ErrorList

	// Validate account reference.
	if ferr := validateAccountRef(ctx, v.Client, si.Spec.Account.Name,
		field.NewPath("spec", "account", "name"), "SlackIntegration"); ferr != nil {
		errs = append(errs, ferr)
	}

	// Validate webhook URL format when provided directly.
	if si.Spec.Integration.WebhookURL != "" {
		parsed, err := url.ParseRequestURI(si.Spec.Integration.WebhookURL)
		if err != nil {
			errs = append(errs, field.Invalid(
				field.NewPath("spec", "integration", "webhookURL"),
				si.Spec.Integration.WebhookURL,
				fmt.Sprintf("invalid webhook URL: %v", err),
			))
		} else if parsed.Scheme != "https" {
			errs = append(errs, field.Invalid(
				field.NewPath("spec", "integration", "webhookURL"),
				si.Spec.Integration.WebhookURL,
				"webhook URL must use HTTPS",
			))
		}
	}

	// Validate Secret reference exists when secretName is provided.
	if si.Spec.Integration.SecretName != "" {
		secret := &corev1.Secret{}
		if err := v.Client.Get(ctx, client.ObjectKey{Name: si.Spec.Integration.SecretName, Namespace: si.Namespace}, secret); err != nil {
			if apierrors.IsNotFound(err) {
				errs = append(errs, field.Invalid(
					field.NewPath("spec", "integration", "secretName"),
					si.Spec.Integration.SecretName,
					fmt.Sprintf("Secret %q not found in namespace %q", si.Spec.Integration.SecretName, si.Namespace),
				))
			} else {
				errs = append(errs, field.InternalError(
					field.NewPath("spec", "integration", "secretName"),
					fmt.Errorf("looking up Secret %q: %w", si.Spec.Integration.SecretName, err),
				))
			}
		}
	}

	if len(errs) > 0 {
		return invalidErr("SlackIntegration", si.Name, errs)
	}
	return nil
}
