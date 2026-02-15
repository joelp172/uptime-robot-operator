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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

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

// ClassifyError is exported for testing purposes.
// It wraps the internal classifyError function to allow unit tests to verify error classification logic.
func ClassifyError(err error) string {
	return classifyError(err)
}
