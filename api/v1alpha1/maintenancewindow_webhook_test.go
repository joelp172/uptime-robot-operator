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
	"testing"
	"time"

	"github.com/joelp172/uptime-robot-operator/internal/uptimerobot/urtypes"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newMWScheme(t *testing.T) *runtime.Scheme {
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

func TestMWValidatorAllowsValidCreate(t *testing.T) {
	t.Parallel()

	scheme := newMWScheme(t)
	account := defaultAccount()

	validator := &MaintenanceWindowCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(account).Build(),
	}

	mw := &MaintenanceWindow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-mw", Namespace: "default"},
		Spec: MaintenanceWindowSpec{
			Name:      "Test Window",
			Interval:  "daily",
			StartTime: "02:00:00",
			Duration:  metav1.Duration{Duration: 30 * time.Minute},
		},
	}

	if _, err := validator.ValidateCreate(context.Background(), mw); err != nil {
		t.Fatalf("expected no validation error, got: %v", err)
	}
}

func TestMWValidatorRejectsUnknownAccount(t *testing.T) {
	t.Parallel()

	scheme := newMWScheme(t)

	validator := &MaintenanceWindowCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
	}

	mw := &MaintenanceWindow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-mw", Namespace: "default"},
		Spec: MaintenanceWindowSpec{
			Account:   corev1.LocalObjectReference{Name: "nonexistent"},
			Name:      "Test",
			Interval:  "daily",
			StartTime: "02:00:00",
			Duration:  metav1.Duration{Duration: 30 * time.Minute},
		},
	}

	if _, err := validator.ValidateCreate(context.Background(), mw); err == nil {
		t.Fatal("expected validation error for unknown account")
	}
}

func TestMWValidatorRejectsUnknownMonitorRef(t *testing.T) {
	t.Parallel()

	scheme := newMWScheme(t)
	account := defaultAccount()

	validator := &MaintenanceWindowCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(account).Build(),
	}

	mw := &MaintenanceWindow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-mw", Namespace: "default"},
		Spec: MaintenanceWindowSpec{
			Name:     "Test",
			Interval: "daily",
			MonitorRefs: []corev1.LocalObjectReference{
				{Name: "nonexistent-monitor"},
			},
			StartTime: "02:00:00",
			Duration:  metav1.Duration{Duration: 30 * time.Minute},
		},
	}

	if _, err := validator.ValidateCreate(context.Background(), mw); err == nil {
		t.Fatal("expected validation error for unknown monitor reference")
	}
}

func TestMWValidatorAllowsExistingMonitorRef(t *testing.T) {
	t.Parallel()

	scheme := newMWScheme(t)
	account := defaultAccount()
	monitor := &Monitor{
		ObjectMeta: metav1.ObjectMeta{Name: "my-monitor", Namespace: "default"},
		Spec: MonitorSpec{
			Monitor: MonitorValues{Name: "Test", Type: urtypes.TypeHTTPS, URL: "https://example.com"},
		},
	}

	validator := &MaintenanceWindowCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(account, monitor).Build(),
	}

	mw := &MaintenanceWindow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-mw", Namespace: "default"},
		Spec: MaintenanceWindowSpec{
			Name:     "Test",
			Interval: "daily",
			MonitorRefs: []corev1.LocalObjectReference{
				{Name: "my-monitor"},
			},
			StartTime: "02:00:00",
			Duration:  metav1.Duration{Duration: 30 * time.Minute},
		},
	}

	if _, err := validator.ValidateCreate(context.Background(), mw); err != nil {
		t.Fatalf("expected no error for existing monitor ref, got: %v", err)
	}
}

func TestMWValidatorRejectsPastStartDate(t *testing.T) {
	t.Parallel()

	scheme := newMWScheme(t)
	account := defaultAccount()

	validator := &MaintenanceWindowCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(account).Build(),
	}

	pastDate := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")

	mw := &MaintenanceWindow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-mw", Namespace: "default"},
		Spec: MaintenanceWindowSpec{
			Name:      "Test",
			Interval:  "once",
			StartDate: pastDate,
			StartTime: "02:00:00",
			Duration:  metav1.Duration{Duration: 30 * time.Minute},
		},
	}

	if _, err := validator.ValidateCreate(context.Background(), mw); err == nil {
		t.Fatal("expected validation error for past startDate")
	}
}

func TestMWValidatorAllowsFutureStartDate(t *testing.T) {
	t.Parallel()

	scheme := newMWScheme(t)
	account := defaultAccount()

	validator := &MaintenanceWindowCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(account).Build(),
	}

	futureDate := time.Now().UTC().AddDate(0, 0, 7).Format("2006-01-02")

	mw := &MaintenanceWindow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-mw", Namespace: "default"},
		Spec: MaintenanceWindowSpec{
			Name:      "Test",
			Interval:  "once",
			StartDate: futureDate,
			StartTime: "02:00:00",
			Duration:  metav1.Duration{Duration: 30 * time.Minute},
		},
	}

	if _, err := validator.ValidateCreate(context.Background(), mw); err != nil {
		t.Fatalf("expected no error for future startDate, got: %v", err)
	}
}

func TestMWValidatorAllowsTodayStartDate(t *testing.T) {
	t.Parallel()

	scheme := newMWScheme(t)
	account := defaultAccount()

	validator := &MaintenanceWindowCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(account).Build(),
	}

	today := time.Now().UTC().Format("2006-01-02")

	mw := &MaintenanceWindow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-mw", Namespace: "default"},
		Spec: MaintenanceWindowSpec{
			Name:      "Test",
			Interval:  "once",
			StartDate: today,
			StartTime: "02:00:00",
			Duration:  metav1.Duration{Duration: 30 * time.Minute},
		},
	}

	if _, err := validator.ValidateCreate(context.Background(), mw); err != nil {
		t.Fatalf("expected no error for today's startDate, got: %v", err)
	}
}

func TestMWValidatorIgnoresStartDateForNonOnceInterval(t *testing.T) {
	t.Parallel()

	scheme := newMWScheme(t)
	account := defaultAccount()

	validator := &MaintenanceWindowCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(account).Build(),
	}

	// Past date is fine when interval != "once"
	pastDate := fmt.Sprintf("%d-01-01", time.Now().Year()-1)

	mw := &MaintenanceWindow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-mw", Namespace: "default"},
		Spec: MaintenanceWindowSpec{
			Name:      "Test",
			Interval:  "daily",
			StartDate: pastDate,
			StartTime: "02:00:00",
			Duration:  metav1.Duration{Duration: 30 * time.Minute},
		},
	}

	if _, err := validator.ValidateCreate(context.Background(), mw); err != nil {
		t.Fatalf("expected no error when interval is daily (startDate not validated), got: %v", err)
	}
}
