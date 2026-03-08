package controller

import (
	"testing"

	uptimerobotv1 "github.com/joelp172/uptime-robot-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestMonitorToIngressRequests(t *testing.T) {
	tests := []struct {
		name    string
		obj     client.Object
		wantLen int
		wantNN  types.NamespacedName
	}{
		{
			name: "monitor sourced from ingress maps to one request",
			obj: &uptimerobotv1.Monitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "monitor-a",
					Namespace: "default",
				},
				Spec: uptimerobotv1.MonitorSpec{
					SourceRef: &corev1.TypedLocalObjectReference{
						Kind: "Ingress",
						Name: "ing-a",
					},
				},
			},
			wantLen: 1,
			wantNN: types.NamespacedName{
				Namespace: "default",
				Name:      "ing-a",
			},
		},
		{
			name: "monitor without source ref maps to no request",
			obj: &uptimerobotv1.Monitor{
				ObjectMeta: metav1.ObjectMeta{Name: "monitor-a", Namespace: "default"},
			},
			wantLen: 0,
		},
		{
			name: "non-ingress source ref maps to no request",
			obj: &uptimerobotv1.Monitor{
				ObjectMeta: metav1.ObjectMeta{Name: "monitor-a", Namespace: "default"},
				Spec: uptimerobotv1.MonitorSpec{
					SourceRef: &corev1.TypedLocalObjectReference{
						Kind: "Deployment",
						Name: "dep-a",
					},
				},
			},
			wantLen: 0,
		},
		{
			name: "ingress source ref with empty name maps to no request",
			obj: &uptimerobotv1.Monitor{
				ObjectMeta: metav1.ObjectMeta{Name: "monitor-a", Namespace: "default"},
				Spec: uptimerobotv1.MonitorSpec{
					SourceRef: &corev1.TypedLocalObjectReference{
						Kind: "Ingress",
						Name: "",
					},
				},
			},
			wantLen: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := monitorToIngressRequests(tc.obj)
			if len(got) != tc.wantLen {
				t.Fatalf("len(monitorToIngressRequests)=%d want=%d", len(got), tc.wantLen)
			}
			if tc.wantLen == 1 && got[0].NamespacedName != tc.wantNN {
				t.Fatalf("request=%v want=%v", got[0].NamespacedName, tc.wantNN)
			}
		})
	}
}

func TestIsIngressSourcedMonitor(t *testing.T) {
	tests := []struct {
		name string
		obj  client.Object
		want bool
	}{
		{
			name: "ingress sourced monitor returns true",
			obj: &uptimerobotv1.Monitor{
				Spec: uptimerobotv1.MonitorSpec{
					SourceRef: &corev1.TypedLocalObjectReference{Kind: "Ingress", Name: "ing-a"},
				},
			},
			want: true,
		},
		{
			name: "monitor without source ref returns false",
			obj:  &uptimerobotv1.Monitor{},
			want: false,
		},
		{
			name: "non-ingress sourced monitor returns false",
			obj: &uptimerobotv1.Monitor{
				Spec: uptimerobotv1.MonitorSpec{
					SourceRef: &corev1.TypedLocalObjectReference{Kind: "Service", Name: "svc-a"},
				},
			},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isIngressSourcedMonitor(tc.obj); got != tc.want {
				t.Fatalf("isIngressSourcedMonitor()=%v want=%v", got, tc.want)
			}
		})
	}
}
