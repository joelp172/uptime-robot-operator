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

func newMGScheme(t *testing.T) *runtime.Scheme {
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

func TestMGValidatorAllowsValidCreate(t *testing.T) {
	t.Parallel()

	scheme := newMGScheme(t)
	account := defaultAccount()

	validator := &MonitorGroupCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(account).Build(),
	}

	mg := &MonitorGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "test-mg", Namespace: "default"},
		Spec: MonitorGroupSpec{
			FriendlyName: "Test Group",
		},
	}

	if _, err := validator.ValidateCreate(context.Background(), mg); err != nil {
		t.Fatalf("expected no validation error, got: %v", err)
	}
}

func TestMGValidatorRejectsUnknownAccount(t *testing.T) {
	t.Parallel()

	scheme := newMGScheme(t)

	validator := &MonitorGroupCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
	}

	mg := &MonitorGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "test-mg", Namespace: "default"},
		Spec: MonitorGroupSpec{
			Account:      corev1.LocalObjectReference{Name: "nonexistent"},
			FriendlyName: "Test Group",
		},
	}

	if _, err := validator.ValidateCreate(context.Background(), mg); err == nil {
		t.Fatal("expected validation error for unknown account")
	}
}

func TestMGValidatorRejectsUnknownMonitorRef(t *testing.T) {
	t.Parallel()

	scheme := newMGScheme(t)
	account := defaultAccount()

	validator := &MonitorGroupCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(account).Build(),
	}

	mg := &MonitorGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "test-mg", Namespace: "default"},
		Spec: MonitorGroupSpec{
			FriendlyName: "Test Group",
			Monitors: []corev1.LocalObjectReference{
				{Name: "nonexistent-monitor"},
			},
		},
	}

	if _, err := validator.ValidateCreate(context.Background(), mg); err == nil {
		t.Fatal("expected validation error for unknown monitor reference")
	}
}

func TestMGValidatorAllowsExistingMonitorRef(t *testing.T) {
	t.Parallel()

	scheme := newMGScheme(t)
	account := defaultAccount()
	monitor := &Monitor{
		ObjectMeta: metav1.ObjectMeta{Name: "my-monitor", Namespace: "default"},
		Spec: MonitorSpec{
			Monitor: MonitorValues{Name: "Test", Type: urtypes.TypeHTTPS, URL: "https://example.com"},
		},
	}

	validator := &MonitorGroupCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(account, monitor).Build(),
	}

	mg := &MonitorGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "test-mg", Namespace: "default"},
		Spec: MonitorGroupSpec{
			FriendlyName: "Test Group",
			Monitors: []corev1.LocalObjectReference{
				{Name: "my-monitor"},
			},
		},
	}

	if _, err := validator.ValidateCreate(context.Background(), mg); err != nil {
		t.Fatalf("expected no error for existing monitor ref, got: %v", err)
	}
}

func TestMGValidatorAllowsDeleteWithoutValidation(t *testing.T) {
	t.Parallel()

	scheme := newMGScheme(t)

	validator := &MonitorGroupCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
	}

	mg := &MonitorGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "test-mg", Namespace: "default"},
	}

	if _, err := validator.ValidateDelete(context.Background(), mg); err != nil {
		t.Fatalf("expected no error on delete, got: %v", err)
	}
}

func TestMGValidatorAppliesValidationOnUpdate(t *testing.T) {
	t.Parallel()

	scheme := newMGScheme(t)
	account := defaultAccount()

	validator := &MonitorGroupCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(account).Build(),
	}

	oldMG := &MonitorGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "test-mg", Namespace: "default"},
		Spec: MonitorGroupSpec{
			FriendlyName: "Test Group",
		},
	}

	newMG := oldMG.DeepCopy()
	newMG.Spec.Monitors = []corev1.LocalObjectReference{{Name: "nonexistent-monitor"}}

	if _, err := validator.ValidateUpdate(context.Background(), oldMG, newMG); err == nil {
		t.Fatal("expected validation error for unknown monitor reference on update")
	}

	if _, err := validator.ValidateUpdate(context.Background(), oldMG, oldMG.DeepCopy()); err != nil {
		t.Fatalf("expected no validation error for unchanged update, got: %v", err)
	}
}
