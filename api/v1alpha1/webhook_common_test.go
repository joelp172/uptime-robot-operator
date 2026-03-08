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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newWebhookCommonScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := AddToScheme(scheme); err != nil {
		t.Fatalf("failed to build scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add core scheme: %v", err)
	}
	return scheme
}

func testAccount(name string, isDefault bool) *Account {
	return &Account{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: AccountSpec{
			IsDefault: isDefault,
			ApiKeySecretRef: corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "secret"},
				Key:                  "apiKey",
			},
		},
	}
}

func fakeReaderForAccounts(t *testing.T, accounts ...client.Object) client.Reader {
	t.Helper()
	return fake.NewClientBuilder().
		WithScheme(newWebhookCommonScheme(t)).
		WithObjects(accounts...).
		Build()
}

func TestValidateAccountRefAllowsSingleDefaultWhenNameEmpty(t *testing.T) {
	t.Parallel()

	err := validateAccountRef(
		context.Background(),
		fakeReaderForAccounts(t, testAccount("default-account", true)),
		"",
		field.NewPath("spec", "account", "name"),
		"Monitor",
	)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidateAccountRefRejectsMissingDefaultWhenNameEmpty(t *testing.T) {
	t.Parallel()

	err := validateAccountRef(
		context.Background(),
		fakeReaderForAccounts(t, testAccount("non-default", false)),
		"",
		field.NewPath("spec", "account", "name"),
		"Monitor",
	)
	if err == nil {
		t.Fatal("expected error when no default account exists")
	}
	if err.Type != field.ErrorTypeRequired {
		t.Fatalf("expected Required error type, got: %s", err.Type)
	}
}

func TestValidateAccountRefRejectsMultipleDefaultsWhenNameEmpty(t *testing.T) {
	t.Parallel()

	err := validateAccountRef(
		context.Background(),
		fakeReaderForAccounts(t,
			testAccount("default-1", true),
			testAccount("default-2", true),
		),
		"",
		field.NewPath("spec", "account", "name"),
		"Monitor",
	)
	if err == nil {
		t.Fatal("expected error when multiple default accounts exist")
	}
	if err.Type != field.ErrorTypeInvalid {
		t.Fatalf("expected Invalid error type, got: %s", err.Type)
	}
}

func TestValidateAccountRefAllowsExistingNamedAccount(t *testing.T) {
	t.Parallel()

	err := validateAccountRef(
		context.Background(),
		fakeReaderForAccounts(t, testAccount("account-a", false)),
		"account-a",
		field.NewPath("spec", "account", "name"),
		"Monitor",
	)
	if err != nil {
		t.Fatalf("expected no error for existing named account, got: %v", err)
	}
}

func TestValidateAccountRefRejectsUnknownNamedAccount(t *testing.T) {
	t.Parallel()

	err := validateAccountRef(
		context.Background(),
		fakeReaderForAccounts(t),
		"missing-account",
		field.NewPath("spec", "account", "name"),
		"Monitor",
	)
	if err == nil {
		t.Fatal("expected error for unknown named account")
	}
	if err.Type != field.ErrorTypeInvalid {
		t.Fatalf("expected Invalid error type, got: %s", err.Type)
	}
}
