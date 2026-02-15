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
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: "none",
		},
		{
			name:     "not found error",
			err:      apierrors.NewNotFound(schema.GroupResource{}, "test"),
			expected: "not_found",
		},
		{
			name:     "conflict error",
			err:      apierrors.NewConflict(schema.GroupResource{}, "test", errors.New("conflict")),
			expected: "conflict",
		},
		{
			name:     "already exists error",
			err:      apierrors.NewAlreadyExists(schema.GroupResource{}, "test"),
			expected: "already_exists",
		},
		{
			name:     "timeout error",
			err:      apierrors.NewTimeoutError("timeout", 5),
			expected: "timeout",
		},
		{
			name:     "context canceled",
			err:      context.Canceled,
			expected: "context_canceled",
		},
		{
			name:     "context deadline exceeded",
			err:      context.DeadlineExceeded,
			expected: "context_deadline_exceeded",
		},
		{
			name:     "unknown error",
			err:      errors.New("some other error"),
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyError(tt.err)
			if result != tt.expected {
				t.Errorf("classifyError() = %v, want %v", result, tt.expected)
			}
		})
	}
}
