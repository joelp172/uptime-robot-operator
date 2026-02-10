package v1alpha1

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestContactValidatorRejectsSecondDefault(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := AddToScheme(scheme); err != nil {
		t.Fatalf("failed to build scheme: %v", err)
	}

	existing := &Contact{
		ObjectMeta: metav1.ObjectMeta{Name: "default-a"},
		Spec: ContactSpec{
			IsDefault: true,
			Contact:   ContactValues{ID: "1"},
		},
	}

	validator := &ContactCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build(),
	}

	candidate := &Contact{
		ObjectMeta: metav1.ObjectMeta{Name: "default-b"},
		Spec: ContactSpec{
			IsDefault: true,
			Contact:   ContactValues{ID: "2"},
		},
	}

	if _, err := validator.ValidateCreate(context.Background(), candidate); err == nil {
		t.Fatalf("expected validation error for second default contact")
	}
}

func TestContactValidatorAllowsSingleDefault(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := AddToScheme(scheme); err != nil {
		t.Fatalf("failed to build scheme: %v", err)
	}

	validator := &ContactCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
	}

	candidate := &Contact{
		ObjectMeta: metav1.ObjectMeta{Name: "default-a"},
		Spec: ContactSpec{
			IsDefault: true,
			Contact:   ContactValues{ID: "1"},
		},
	}

	if _, err := validator.ValidateCreate(context.Background(), candidate); err != nil {
		t.Fatalf("expected no validation error, got: %v", err)
	}
}

func TestContactValidatorAllowsUpdateOfCurrentDefault(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := AddToScheme(scheme); err != nil {
		t.Fatalf("failed to build scheme: %v", err)
	}

	existing := &Contact{
		ObjectMeta: metav1.ObjectMeta{Name: "default-a"},
		Spec: ContactSpec{
			IsDefault: true,
			Contact:   ContactValues{ID: "1"},
		},
	}

	validator := &ContactCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build(),
	}

	oldObj := existing.DeepCopy()
	newObj := existing.DeepCopy()
	newObj.Spec.Contact = ContactValues{Name: "new-name"}

	if _, err := validator.ValidateUpdate(context.Background(), oldObj, newObj); err != nil {
		t.Fatalf("expected no validation error on updating existing default: %v", err)
	}
}

func TestContactValidatorRejectsUpdateToDefaultWhenAnotherExists(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := AddToScheme(scheme); err != nil {
		t.Fatalf("failed to build scheme: %v", err)
	}

	existingDefault := &Contact{
		ObjectMeta: metav1.ObjectMeta{Name: "default-a"},
		Spec: ContactSpec{
			IsDefault: true,
			Contact:   ContactValues{ID: "1"},
		},
	}
	other := &Contact{
		ObjectMeta: metav1.ObjectMeta{Name: "other"},
		Spec: ContactSpec{
			IsDefault: false,
			Contact:   ContactValues{ID: "2"},
		},
	}

	validator := &ContactCustomValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingDefault, other).Build(),
	}

	oldObj := other.DeepCopy()
	newObj := other.DeepCopy()
	newObj.Spec.IsDefault = true

	if _, err := validator.ValidateUpdate(context.Background(), oldObj, newObj); err == nil {
		t.Fatalf("expected validation error when updating second contact to default")
	}
}
