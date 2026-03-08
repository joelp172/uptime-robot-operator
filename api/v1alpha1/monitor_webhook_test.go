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

	"github.com/joelp172/uptime-robot-operator/internal/uptimerobot/urtypes"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newMonitorScheme(t *testing.T) *runtime.Scheme {
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

func defaultAccount() *Account {
	return &Account{
		ObjectMeta: metav1.ObjectMeta{Name: "default-account"},
		Spec: AccountSpec{
			IsDefault: true,
			ApiKeySecretRef: corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "secret"},
				Key:                  "apiKey",
			},
		},
	}
}

func TestMonitorValidatorAllowsValidCreate(t *testing.T) {
	t.Parallel()

	scheme := newMonitorScheme(t)
	account := defaultAccount()

	validator := &MonitorCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(account).Build(),
	}

	monitor := &Monitor{
		ObjectMeta: metav1.ObjectMeta{Name: "test-monitor", Namespace: "default"},
		Spec: MonitorSpec{
			Monitor: MonitorValues{
				Name: "Test",
				Type: urtypes.TypeHTTPS,
				URL:  "https://example.com",
			},
		},
	}

	if _, err := validator.ValidateCreate(context.Background(), monitor); err != nil {
		t.Fatalf("expected no validation error, got: %v", err)
	}
}

func TestMonitorValidatorRejectsUnknownAccount(t *testing.T) {
	t.Parallel()

	scheme := newMonitorScheme(t)

	validator := &MonitorCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
	}

	monitor := &Monitor{
		ObjectMeta: metav1.ObjectMeta{Name: "test-monitor", Namespace: "default"},
		Spec: MonitorSpec{
			Account: corev1.LocalObjectReference{Name: "nonexistent"},
			Monitor: MonitorValues{
				Name: "Test",
				Type: urtypes.TypeHTTPS,
				URL:  "https://example.com",
			},
		},
	}

	if _, err := validator.ValidateCreate(context.Background(), monitor); err == nil {
		t.Fatal("expected validation error for unknown account")
	}
}

func TestMonitorValidatorRejectsNoDefaultAccount(t *testing.T) {
	t.Parallel()

	scheme := newMonitorScheme(t)

	validator := &MonitorCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
	}

	monitor := &Monitor{
		ObjectMeta: metav1.ObjectMeta{Name: "test-monitor", Namespace: "default"},
		Spec: MonitorSpec{
			Monitor: MonitorValues{
				Name: "Test",
				Type: urtypes.TypeHTTPS,
				URL:  "https://example.com",
			},
		},
	}

	if _, err := validator.ValidateCreate(context.Background(), monitor); err == nil {
		t.Fatal("expected validation error when no default account exists")
	}
}

func TestMonitorValidatorRejectsUnknownContact(t *testing.T) {
	t.Parallel()

	scheme := newMonitorScheme(t)
	account := defaultAccount()

	validator := &MonitorCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(account).Build(),
	}

	monitor := &Monitor{
		ObjectMeta: metav1.ObjectMeta{Name: "test-monitor", Namespace: "default"},
		Spec: MonitorSpec{
			Contacts: []MonitorContactRef{
				{LocalObjectReference: corev1.LocalObjectReference{Name: "nonexistent-contact"}},
			},
			Monitor: MonitorValues{
				Name: "Test",
				Type: urtypes.TypeHTTPS,
				URL:  "https://example.com",
			},
		},
	}

	if _, err := validator.ValidateCreate(context.Background(), monitor); err == nil {
		t.Fatal("expected validation error for unknown contact")
	}
}

func TestMonitorValidatorAllowsNamedContact(t *testing.T) {
	t.Parallel()

	scheme := newMonitorScheme(t)
	account := defaultAccount()
	contact := &Contact{
		ObjectMeta: metav1.ObjectMeta{Name: "my-contact"},
		Spec: ContactSpec{
			Contact: ContactValues{ID: "123"},
		},
	}

	validator := &MonitorCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(account, contact).Build(),
	}

	monitor := &Monitor{
		ObjectMeta: metav1.ObjectMeta{Name: "test-monitor", Namespace: "default"},
		Spec: MonitorSpec{
			Contacts: []MonitorContactRef{
				{LocalObjectReference: corev1.LocalObjectReference{Name: "my-contact"}},
			},
			Monitor: MonitorValues{
				Name: "Test",
				Type: urtypes.TypeHTTPS,
				URL:  "https://example.com",
			},
		},
	}

	if _, err := validator.ValidateCreate(context.Background(), monitor); err != nil {
		t.Fatalf("expected no error for existing contact, got: %v", err)
	}
}

func TestMonitorValidatorAllowsEmptyContactRef(t *testing.T) {
	t.Parallel()

	scheme := newMonitorScheme(t)
	account := defaultAccount()

	validator := &MonitorCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(account).Build(),
	}

	monitor := &Monitor{
		ObjectMeta: metav1.ObjectMeta{Name: "test-monitor", Namespace: "default"},
		Spec: MonitorSpec{
			// Empty contact ref (uses default contact) — name is ""
			Contacts: []MonitorContactRef{
				{LocalObjectReference: corev1.LocalObjectReference{Name: ""}},
			},
			Monitor: MonitorValues{
				Name: "Test",
				Type: urtypes.TypeHTTPS,
				URL:  "https://example.com",
			},
		},
	}

	if _, err := validator.ValidateCreate(context.Background(), monitor); err != nil {
		t.Fatalf("expected no error for empty contact ref (default), got: %v", err)
	}
}

func TestMonitorValidatorRejectsTypeChange(t *testing.T) {
	t.Parallel()

	scheme := newMonitorScheme(t)
	account := defaultAccount()

	validator := &MonitorCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(account).Build(),
	}

	oldMonitor := &Monitor{
		ObjectMeta: metav1.ObjectMeta{Name: "test-monitor", Namespace: "default"},
		Spec: MonitorSpec{
			Monitor: MonitorValues{
				Name: "Test",
				Type: urtypes.TypeHTTPS,
				URL:  "https://example.com",
			},
		},
	}
	newMonitor := oldMonitor.DeepCopy()
	newMonitor.Spec.Monitor.Type = urtypes.TypePing

	if _, err := validator.ValidateUpdate(context.Background(), oldMonitor, newMonitor); err == nil {
		t.Fatal("expected validation error for type change")
	}
}

func TestMonitorValidatorAllowsSameTypeUpdate(t *testing.T) {
	t.Parallel()

	scheme := newMonitorScheme(t)
	account := defaultAccount()

	validator := &MonitorCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(account).Build(),
	}

	oldMonitor := &Monitor{
		ObjectMeta: metav1.ObjectMeta{Name: "test-monitor", Namespace: "default"},
		Spec: MonitorSpec{
			Monitor: MonitorValues{
				Name: "Test",
				Type: urtypes.TypeHTTPS,
				URL:  "https://example.com",
			},
		},
	}
	newMonitor := oldMonitor.DeepCopy()
	newMonitor.Spec.Monitor.URL = "https://example.org"

	if _, err := validator.ValidateUpdate(context.Background(), oldMonitor, newMonitor); err != nil {
		t.Fatalf("expected no error for same-type update, got: %v", err)
	}
}

func TestMonitorValidatorRejectsInvalidURL(t *testing.T) {
	t.Parallel()

	scheme := newMonitorScheme(t)
	account := defaultAccount()

	validator := &MonitorCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(account).Build(),
	}

	monitor := &Monitor{
		ObjectMeta: metav1.ObjectMeta{Name: "test-monitor", Namespace: "default"},
		Spec: MonitorSpec{
			Monitor: MonitorValues{
				Name: "Test",
				Type: urtypes.TypeHTTPS,
				URL:  "not-a-valid-url",
			},
		},
	}

	if _, err := validator.ValidateCreate(context.Background(), monitor); err == nil {
		t.Fatal("expected validation error for invalid URL")
	}
}

func TestMonitorValidatorAllowsDeleteWithoutValidation(t *testing.T) {
	t.Parallel()

	scheme := newMonitorScheme(t)

	validator := &MonitorCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
	}

	monitor := &Monitor{
		ObjectMeta: metav1.ObjectMeta{Name: "test-monitor", Namespace: "default"},
	}

	if _, err := validator.ValidateDelete(context.Background(), monitor); err != nil {
		t.Fatalf("expected no error on delete, got: %v", err)
	}
}
