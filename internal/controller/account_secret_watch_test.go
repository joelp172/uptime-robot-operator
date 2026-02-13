package controller

import (
	"context"
	"testing"

	uptimerobotv1 "github.com/joelp172/uptime-robot-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestMapSecretToAccountsReferencedSecretReturnsRequest(t *testing.T) {
	r := newAccountReconcilerForWatchTests(t,
		&uptimerobotv1.Account{
			ObjectMeta: metav1.ObjectMeta{Name: "account-a"},
			Spec: uptimerobotv1.AccountSpec{
				ApiKeySecretRef: corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "secret-a"},
					Key:                  "apiKey",
				},
			},
		},
		&uptimerobotv1.Account{
			ObjectMeta: metav1.ObjectMeta{Name: "account-b"},
			Spec: uptimerobotv1.AccountSpec{
				ApiKeySecretRef: corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "secret-b"},
					Key:                  "apiKey",
				},
			},
		},
	)

	reqs := r.mapSecretToAccounts(context.Background(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret-a",
			Namespace: ClusterResourceNamespace,
		},
	})

	if len(reqs) != 1 {
		t.Fatalf("expected 1 reconcile request, got %d", len(reqs))
	}
	if reqs[0].Name != "account-a" {
		t.Fatalf("expected reconcile request for account-a, got %s", reqs[0].Name)
	}
}

func TestMapSecretToAccountsUnrelatedSecretReturnsNoRequests(t *testing.T) {
	r := newAccountReconcilerForWatchTests(t,
		&uptimerobotv1.Account{
			ObjectMeta: metav1.ObjectMeta{Name: "account-a"},
			Spec: uptimerobotv1.AccountSpec{
				ApiKeySecretRef: corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "secret-a"},
					Key:                  "apiKey",
				},
			},
		},
	)

	reqs := r.mapSecretToAccounts(context.Background(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "unrelated-secret",
			Namespace: ClusterResourceNamespace,
		},
	})

	if len(reqs) != 0 {
		t.Fatalf("expected 0 reconcile requests, got %d", len(reqs))
	}
}

func TestMapSecretToAccountsDifferentNamespaceReturnsNoRequests(t *testing.T) {
	r := newAccountReconcilerForWatchTests(t,
		&uptimerobotv1.Account{
			ObjectMeta: metav1.ObjectMeta{Name: "account-a"},
			Spec: uptimerobotv1.AccountSpec{
				ApiKeySecretRef: corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "secret-a"},
					Key:                  "apiKey",
				},
			},
		},
	)

	reqs := r.mapSecretToAccounts(context.Background(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret-a",
			Namespace: "other-namespace",
		},
	})

	if len(reqs) != 0 {
		t.Fatalf("expected 0 reconcile requests, got %d", len(reqs))
	}
}

func TestMapSecretToAccountsSharedSecretReturnsAllReferencingAccounts(t *testing.T) {
	r := newAccountReconcilerForWatchTests(t,
		&uptimerobotv1.Account{
			ObjectMeta: metav1.ObjectMeta{Name: "account-a"},
			Spec: uptimerobotv1.AccountSpec{
				ApiKeySecretRef: corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "shared-secret"},
					Key:                  "apiKey",
				},
			},
		},
		&uptimerobotv1.Account{
			ObjectMeta: metav1.ObjectMeta{Name: "account-b"},
			Spec: uptimerobotv1.AccountSpec{
				ApiKeySecretRef: corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "shared-secret"},
					Key:                  "apiKey",
				},
			},
		},
	)

	reqs := r.mapSecretToAccounts(context.Background(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "shared-secret",
			Namespace: ClusterResourceNamespace,
		},
	})

	if len(reqs) != 2 {
		t.Fatalf("expected 2 reconcile requests, got %d", len(reqs))
	}
}

func newAccountReconcilerForWatchTests(t *testing.T, accounts ...*uptimerobotv1.Account) *AccountReconciler {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1 scheme: %v", err)
	}
	if err := uptimerobotv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add uptimerobotv1 scheme: %v", err)
	}

	objects := make([]runtime.Object, 0, len(accounts))
	for _, account := range accounts {
		objects = append(objects, account)
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objects...).Build()
	return &AccountReconciler{
		Client: c,
		Scheme: scheme,
	}
}
