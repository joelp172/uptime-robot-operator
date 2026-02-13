package controller

import (
	"context"
	"errors"
	"testing"

	uptimerobotv1 "github.com/joelp172/uptime-robot-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetApiKeyTrimsWhitespace(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1 scheme: %v", err)
	}
	if err := uptimerobotv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add uptimerobotv1 scheme: %v", err)
	}

	account := &uptimerobotv1.Account{
		ObjectMeta: metav1.ObjectMeta{Name: "acct"},
		Spec: uptimerobotv1.AccountSpec{
			ApiKeySecretRef: corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "api-key-secret"},
				Key:                  "apiKey",
			},
		},
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-key-secret",
			Namespace: ClusterResourceNamespace,
		},
		Data: map[string][]byte{
			"apiKey": []byte("  key-with-newline\n"),
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
	got, err := GetApiKey(context.Background(), c, account)
	if err != nil {
		t.Fatalf("GetApiKey returned error: %v", err)
	}
	if got != "key-with-newline" {
		t.Fatalf("unexpected api key: %q", got)
	}
}

func TestGetApiKeyRejectsEmptyAfterTrim(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1 scheme: %v", err)
	}
	if err := uptimerobotv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add uptimerobotv1 scheme: %v", err)
	}

	account := &uptimerobotv1.Account{
		ObjectMeta: metav1.ObjectMeta{Name: "acct"},
		Spec: uptimerobotv1.AccountSpec{
			ApiKeySecretRef: corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "api-key-secret"},
				Key:                  "apiKey",
			},
		},
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-key-secret",
			Namespace: ClusterResourceNamespace,
		},
		Data: map[string][]byte{
			"apiKey": []byte(" \n\t "),
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
	_, err := GetApiKey(context.Background(), c, account)
	if err == nil {
		t.Fatal("expected GetApiKey to return an error for empty key")
	}
	if !errors.Is(err, ErrEmptyKey) {
		t.Fatalf("expected ErrEmptyKey, got: %v", err)
	}
}
