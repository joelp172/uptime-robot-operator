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

package metrics

import (
	"context"
	"errors"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
)

// ReconcileWithMetrics wraps a reconcile function with metrics collection
func ReconcileWithMetrics(controllerName string, reconcileFunc func(ctx context.Context, req ctrl.Request) (ctrl.Result, error)) func(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return func(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
		startTime := time.Now()

		result, err := reconcileFunc(ctx, req)

		duration := time.Since(startTime).Seconds()
		ReconciliationDuration.WithLabelValues(controllerName).Observe(duration)

		if err != nil {
			errorType := classifyError(err)
			ReconciliationErrorsTotal.WithLabelValues(controllerName, errorType).Inc()
		}

		return result, err
	}
}

// classifyError classifies errors into categories for metrics
func classifyError(err error) string {
	if err == nil {
		return "none"
	}

	// Kubernetes API errors
	if apierrors.IsNotFound(err) {
		return "not_found"
	}
	if apierrors.IsConflict(err) {
		return "conflict"
	}
	if apierrors.IsAlreadyExists(err) {
		return "already_exists"
	}
	if apierrors.IsInvalid(err) {
		return "invalid"
	}
	if apierrors.IsUnauthorized(err) {
		return "unauthorized"
	}
	if apierrors.IsForbidden(err) {
		return "forbidden"
	}
	if apierrors.IsTimeout(err) {
		return "timeout"
	}
	if apierrors.IsServerTimeout(err) {
		return "server_timeout"
	}
	if apierrors.IsServiceUnavailable(err) {
		return "service_unavailable"
	}
	if apierrors.IsTooManyRequests(err) {
		return "too_many_requests"
	}

	// Context errors
	if errors.Is(err, context.Canceled) {
		return "context_canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "context_deadline_exceeded"
	}

	// Generic errors
	return "unknown"
}
