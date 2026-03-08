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
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSIValidatorAllowsValidWebhookURL(t *testing.T) {
	t.Parallel()

	scheme := newWebhookCommonScheme(t)
	account := defaultAccount()

	validator := &SlackIntegrationCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(account).Build(),
	}

	si := &SlackIntegration{
		ObjectMeta: metav1.ObjectMeta{Name: "test-si", Namespace: "default"},
		Spec: SlackIntegrationSpec{
			Integration: SlackIntegrationValues{
				WebhookURL:   "https://example-slack-webhook.internal/test",
				FriendlyName: "Test",
			},
		},
	}

	if _, err := validator.ValidateCreate(context.Background(), si); err != nil {
		t.Fatalf("expected no validation error, got: %v", err)
	}
}

func TestSIValidatorRejectsInvalidWebhookURL(t *testing.T) {
	t.Parallel()

	scheme := newWebhookCommonScheme(t)
	account := defaultAccount()

	validator := &SlackIntegrationCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(account).Build(),
	}

	si := &SlackIntegration{
		ObjectMeta: metav1.ObjectMeta{Name: "test-si", Namespace: "default"},
		Spec: SlackIntegrationSpec{
			Integration: SlackIntegrationValues{
				WebhookURL:   "not-a-url",
				FriendlyName: "Test",
			},
		},
	}

	if _, err := validator.ValidateCreate(context.Background(), si); err == nil {
		t.Fatal("expected validation error for invalid webhook URL")
	}
}

func TestSIValidatorRejectsHTTPWebhookURL(t *testing.T) {
	t.Parallel()

	scheme := newWebhookCommonScheme(t)
	account := defaultAccount()

	validator := &SlackIntegrationCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(account).Build(),
	}

	si := &SlackIntegration{
		ObjectMeta: metav1.ObjectMeta{Name: "test-si", Namespace: "default"},
		Spec: SlackIntegrationSpec{
			Integration: SlackIntegrationValues{
				WebhookURL:   "http://example-slack-webhook.internal/test",
				FriendlyName: "Test",
			},
		},
	}

	if _, err := validator.ValidateCreate(context.Background(), si); err == nil {
		t.Fatal("expected validation error for HTTP (non-HTTPS) webhook URL")
	}
}

func TestSIValidatorRejectsUnknownAccount(t *testing.T) {
	t.Parallel()

	scheme := newWebhookCommonScheme(t)

	validator := &SlackIntegrationCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
	}

	si := &SlackIntegration{
		ObjectMeta: metav1.ObjectMeta{Name: "test-si", Namespace: "default"},
		Spec: SlackIntegrationSpec{
			Account: corev1.LocalObjectReference{Name: "nonexistent"},
			Integration: SlackIntegrationValues{
				WebhookURL: "https://example-slack-webhook.internal/test",
			},
		},
	}

	if _, err := validator.ValidateCreate(context.Background(), si); err == nil {
		t.Fatal("expected validation error for unknown account")
	}
}

func TestSIValidatorRejectsMissingSecret(t *testing.T) {
	t.Parallel()

	scheme := newWebhookCommonScheme(t)
	account := defaultAccount()

	validator := &SlackIntegrationCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(account).Build(),
	}

	si := &SlackIntegration{
		ObjectMeta: metav1.ObjectMeta{Name: "test-si", Namespace: "default"},
		Spec: SlackIntegrationSpec{
			Integration: SlackIntegrationValues{
				SecretName: "nonexistent-secret",
			},
		},
	}

	if _, err := validator.ValidateCreate(context.Background(), si); err == nil {
		t.Fatal("expected validation error for missing secret")
	}
}

func TestSIValidatorAllowsExistingSecret(t *testing.T) {
	t.Parallel()

	scheme := newWebhookCommonScheme(t)
	account := defaultAccount()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "slack-secret", Namespace: "default"},
		Data: map[string][]byte{
			"webhookURL": []byte("https://example-slack-webhook.internal/test"),
		},
	}

	validator := &SlackIntegrationCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(account, secret).Build(),
	}

	si := &SlackIntegration{
		ObjectMeta: metav1.ObjectMeta{Name: "test-si", Namespace: "default"},
		Spec: SlackIntegrationSpec{
			Integration: SlackIntegrationValues{
				SecretName: "slack-secret",
			},
		},
	}

	if _, err := validator.ValidateCreate(context.Background(), si); err != nil {
		t.Fatalf("expected no error for existing secret, got: %v", err)
	}
}

func TestSIValidatorAllowsDeleteWithoutValidation(t *testing.T) {
	t.Parallel()

	scheme := newWebhookCommonScheme(t)

	validator := &SlackIntegrationCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
	}

	si := &SlackIntegration{
		ObjectMeta: metav1.ObjectMeta{Name: "test-si", Namespace: "default"},
	}

	if _, err := validator.ValidateDelete(context.Background(), si); err != nil {
		t.Fatalf("expected no error on delete, got: %v", err)
	}
}
