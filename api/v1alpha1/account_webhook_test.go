package v1alpha1

import (
	"context"
	"testing"

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
		Spec:       AccountSpec{IsDefault: true},
	}

	validator := &AccountCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build(),
	}

	candidate := &Account{
		ObjectMeta: metav1.ObjectMeta{Name: "default-b"},
		Spec:       AccountSpec{IsDefault: true},
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
		Spec:       AccountSpec{IsDefault: true},
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
		Spec:       AccountSpec{IsDefault: true},
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
