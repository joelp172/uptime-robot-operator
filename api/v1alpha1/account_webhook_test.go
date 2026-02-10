package v1alpha1

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestAccountValidatorRejectsSecondDefault(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := AddToScheme(scheme); err != nil {
		t.Fatalf("failed to build scheme: %v", err)
	}

	existing := &Account{
		ObjectMeta: metav1.ObjectMeta{Name: "default-a"},
		Spec: AccountSpec{
			IsDefault: true,
			ApiKeySecretRef: corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "secret-a"},
				Key:                  "apiKey",
			},
		},
	}

	validator := &AccountCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build(),
	}

	candidate := &Account{
		ObjectMeta: metav1.ObjectMeta{Name: "default-b"},
		Spec: AccountSpec{
			IsDefault: true,
			ApiKeySecretRef: corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "secret-b"},
				Key:                  "apiKey",
			},
		},
	}

	if _, err := validator.ValidateCreate(context.Background(), candidate); err == nil {
		t.Fatalf("expected validation error for second default account")
	}
}

func TestAccountValidatorAllowsSingleDefault(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := AddToScheme(scheme); err != nil {
		t.Fatalf("failed to build scheme: %v", err)
	}

	validator := &AccountCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
	}

	candidate := &Account{
		ObjectMeta: metav1.ObjectMeta{Name: "default-a"},
		Spec: AccountSpec{
			IsDefault: true,
			ApiKeySecretRef: corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "secret-a"},
				Key:                  "apiKey",
			},
		},
	}

	if _, err := validator.ValidateCreate(context.Background(), candidate); err != nil {
		t.Fatalf("expected no validation error, got: %v", err)
	}
}

func TestAccountValidatorAllowsUpdateOfCurrentDefault(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := AddToScheme(scheme); err != nil {
		t.Fatalf("failed to build scheme: %v", err)
	}

	existing := &Account{
		ObjectMeta: metav1.ObjectMeta{Name: "default-a"},
		Spec: AccountSpec{
			IsDefault: true,
			ApiKeySecretRef: corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "secret-a"},
				Key:                  "apiKey",
			},
		},
	}

	validator := &AccountCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build(),
	}

	oldObj := existing.DeepCopy()
	newObj := existing.DeepCopy()
	newObj.Spec.ApiKeySecretRef.Name = "new-secret"
	newObj.Spec.ApiKeySecretRef.Key = "api-key"

	if _, err := validator.ValidateUpdate(context.Background(), oldObj, newObj); err != nil {
		t.Fatalf("expected no validation error on updating existing default: %v", err)
	}
}

func TestAccountValidatorRejectsUpdateToDefaultWhenAnotherExists(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := AddToScheme(scheme); err != nil {
		t.Fatalf("failed to build scheme: %v", err)
	}

	existingDefault := &Account{
		ObjectMeta: metav1.ObjectMeta{Name: "default-a"},
		Spec: AccountSpec{
			IsDefault: true,
			ApiKeySecretRef: corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "secret-a"},
				Key:                  "apiKey",
			},
		},
	}
	other := &Account{
		ObjectMeta: metav1.ObjectMeta{Name: "other"},
		Spec: AccountSpec{
			IsDefault: false,
			ApiKeySecretRef: corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "secret-b"},
				Key:                  "apiKey",
			},
		},
	}

	validator := &AccountCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingDefault, other).Build(),
	}

	oldObj := other.DeepCopy()
	newObj := other.DeepCopy()
	newObj.Spec.IsDefault = true

	if _, err := validator.ValidateUpdate(context.Background(), oldObj, newObj); err == nil {
		t.Fatalf("expected validation error when updating second account to default")
	}
}
