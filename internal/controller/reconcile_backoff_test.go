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

package controller

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/joelp172/uptime-robot-operator/internal/uptimerobot"
)

// mockTimeoutError implements net.Error with Timeout() returning true
type mockTimeoutError struct {
	msg string
}

func (e *mockTimeoutError) Error() string   { return e.msg }
func (e *mockTimeoutError) Timeout() bool   { return true }
func (e *mockTimeoutError) Temporary() bool { return true }

var _ net.Error = (*mockTimeoutError)(nil)

func TestIsTransientError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "500 Internal Server Error",
			err:      errors.New("error code from Uptime Robot API: 500 Internal Server Error - server error"),
			expected: true,
		},
		{
			name:     "502 Bad Gateway",
			err:      errors.New("error code from Uptime Robot API: 502 Bad Gateway"),
			expected: true,
		},
		{
			name:     "503 Service Unavailable",
			err:      errors.New("error code from Uptime Robot API: 503 Service Unavailable"),
			expected: true,
		},
		{
			name:     "504 Gateway Timeout",
			err:      errors.New("error code from Uptime Robot API: 504 Gateway Timeout"),
			expected: true,
		},
		{
			name:     "429 Too Many Requests",
			err:      errors.New("error code from Uptime Robot API: 429 Too Many Requests"),
			expected: true,
		},
		{
			name:     "network timeout error",
			err:      &mockTimeoutError{msg: "context deadline exceeded"},
			expected: true,
		},
		{
			name:     "connection refused",
			err:      errors.New("dial tcp: connection refused"),
			expected: true,
		},
		{
			name:     "connection reset",
			err:      errors.New("read tcp: connection reset by peer"),
			expected: true,
		},
		{
			name:     "broken pipe",
			err:      errors.New("write tcp: broken pipe"),
			expected: true,
		},
		{
			name:     "EOF error",
			err:      errors.New("EOF"),
			expected: true,
		},
		{
			name:     "unexpected EOF",
			err:      errors.New("unexpected EOF"),
			expected: true,
		},
		{
			name:     "max retries exceeded",
			err:      fmt.Errorf("%w: original error", uptimerobot.ErrMaxRetriesExceeded),
			expected: true,
		},
		{
			name:     "400 Bad Request (not transient)",
			err:      errors.New("error code from Uptime Robot API: 400 Bad Request"),
			expected: false,
		},
		{
			name:     "401 Unauthorized (not transient)",
			err:      errors.New("error code from Uptime Robot API: 401 Unauthorized"),
			expected: false,
		},
		{
			name:     "404 Not Found (not transient)",
			err:      errors.New("error code from Uptime Robot API: 404 Not Found"),
			expected: false,
		},
		{
			name:     "generic error (not transient)",
			err:      errors.New("some random error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsTransientError(tt.err)
			if result != tt.expected {
				t.Errorf("IsTransientError() = %v, want %v for error: %v", result, tt.expected, tt.err)
			}
		})
	}
}

func TestIsPermanentError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "400 Bad Request",
			err:      errors.New("error code from Uptime Robot API: 400 Bad Request"),
			expected: true,
		},
		{
			name:     "401 Unauthorized",
			err:      errors.New("error code from Uptime Robot API: 401 Unauthorized"),
			expected: true,
		},
		{
			name:     "403 Forbidden",
			err:      errors.New("error code from Uptime Robot API: 403 Forbidden"),
			expected: true,
		},
		{
			name:     "404 Not Found",
			err:      errors.New("error code from Uptime Robot API: 404 Not Found - resource not found"),
			expected: true,
		},
		{
			name:     "uptimerobot.ErrMonitorNotFound",
			err:      uptimerobot.ErrMonitorNotFound,
			expected: true,
		},
		{
			name:     "uptimerobot.ErrContactNotFound",
			err:      uptimerobot.ErrContactNotFound,
			expected: true,
		},
		{
			name:     "ErrContactMissingID",
			err:      ErrContactMissingID,
			expected: true,
		},
		{
			name:     "ErrSecretMissingKey",
			err:      ErrSecretMissingKey,
			expected: true,
		},
		{
			name:     "ErrKeyNotFound",
			err:      ErrKeyNotFound,
			expected: true,
		},
		{
			name:     "ErrEmptyKey",
			err:      ErrEmptyKey,
			expected: true,
		},
		{
			name:     "500 Internal Server Error (not permanent)",
			err:      errors.New("error code from Uptime Robot API: 500 Internal Server Error"),
			expected: false,
		},
		{
			name:     "503 Service Unavailable (not permanent)",
			err:      errors.New("error code from Uptime Robot API: 503 Service Unavailable"),
			expected: false,
		},
		{
			name:     "network timeout (not permanent)",
			err:      &mockTimeoutError{msg: "timeout"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsPermanentError(tt.err)
			if result != tt.expected {
				t.Errorf("IsPermanentError() = %v, want %v for error: %v", result, tt.expected, tt.err)
			}
		})
	}
}

func TestCalculateRequeueDelay(t *testing.T) {
	tests := []struct {
		name        string
		attempt     int
		wantMin     time.Duration
		wantMax     time.Duration
		description string
	}{
		{
			name:        "attempt 0",
			attempt:     0,
			wantMin:     4*time.Second + 250*time.Millisecond, // 5s - 15%
			wantMax:     5*time.Second + 750*time.Millisecond, // 5s + 15%
			description: "5s * 2^0 = 5s",
		},
		{
			name:        "attempt 1",
			attempt:     1,
			wantMin:     8*time.Second + 500*time.Millisecond,  // 10s - 15%
			wantMax:     11*time.Second + 500*time.Millisecond, // 10s + 15%
			description: "5s * 2^1 = 10s",
		},
		{
			name:        "attempt 2",
			attempt:     2,
			wantMin:     17 * time.Second, // 20s - 15%
			wantMax:     23 * time.Second, // 20s + 15%
			description: "5s * 2^2 = 20s",
		},
		{
			name:        "attempt 3",
			attempt:     3,
			wantMin:     34 * time.Second, // 40s - 15%
			wantMax:     46 * time.Second, // 40s + 15%
			description: "5s * 2^3 = 40s",
		},
		{
			name:        "attempt 4",
			attempt:     4,
			wantMin:     68 * time.Second, // 80s - 15%
			wantMax:     92 * time.Second, // 80s + 15%
			description: "5s * 2^4 = 80s",
		},
		{
			name:        "attempt 5",
			attempt:     5,
			wantMin:     136 * time.Second, // 160s - 15%
			wantMax:     184 * time.Second, // 160s + 15%
			description: "5s * 2^5 = 160s",
		},
		{
			name:        "attempt 6 (capped at 5m)",
			attempt:     6,
			wantMin:     4*time.Minute + 15*time.Second, // 5m - 15%
			wantMax:     5*time.Minute + 45*time.Second, // 5m + 15%
			description: "5s * 2^6 = 320s, capped at 5m",
		},
		{
			name:        "attempt 10 (capped at 5m)",
			attempt:     10,
			wantMin:     4*time.Minute + 15*time.Second, // 5m - 15%
			wantMax:     5*time.Minute + 45*time.Second, // 5m + 15%
			description: "capped at max delay of 5m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run multiple times to account for jitter randomness
			for i := 0; i < 10; i++ {
				delay := CalculateRequeueDelay(tt.attempt)
				if delay < tt.wantMin || delay > tt.wantMax {
					t.Errorf("CalculateRequeueDelay(%d) = %v, want between %v and %v (%s)",
						tt.attempt, delay, tt.wantMin, tt.wantMax, tt.description)
				}
			}
		})
	}
}

func TestCalculateRequeueDelayJitter(t *testing.T) {
	// Test that jitter actually produces different values
	attempt := 2
	delays := make(map[time.Duration]bool)

	// Run 100 times to ensure we get different values due to jitter
	for i := 0; i < 100; i++ {
		delay := CalculateRequeueDelay(attempt)
		delays[delay] = true
	}

	// We should have many different delay values due to jitter
	// With 15% jitter on 20s (±3s) and nanosecond precision, we expect high variance
	// Using threshold of 50 to be robust across different systems while still validating jitter works
	if len(delays) < 50 {
		t.Errorf("Expected jitter to produce varied delays, got only %d unique values out of 100 runs (expected at least 50)", len(delays))
	}
}

func TestGetRetryCount(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    int
	}{
		{
			name:        "nil annotations",
			annotations: nil,
			expected:    0,
		},
		{
			name:        "empty annotations",
			annotations: map[string]string{},
			expected:    0,
		},
		{
			name: "no retry count annotation",
			annotations: map[string]string{
				"other-annotation": "value",
			},
			expected: 0,
		},
		{
			name: "retry count 0",
			annotations: map[string]string{
				AnnotationRetryCount: "0",
			},
			expected: 0,
		},
		{
			name: "retry count 1",
			annotations: map[string]string{
				AnnotationRetryCount: "1",
			},
			expected: 1,
		},
		{
			name: "retry count 5",
			annotations: map[string]string{
				AnnotationRetryCount: "5",
			},
			expected: 5,
		},
		{
			name: "invalid retry count (non-numeric)",
			annotations: map[string]string{
				AnnotationRetryCount: "invalid",
			},
			expected: 0,
		},
		{
			name: "invalid retry count (negative)",
			annotations: map[string]string{
				AnnotationRetryCount: "-1",
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetRetryCount(tt.annotations)
			if result != tt.expected {
				t.Errorf("GetRetryCount() = %d, want %d", result, tt.expected)
			}
		})
	}
}

func TestIncrementRetryCount(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    string
	}{
		{
			name:        "nil annotations",
			annotations: nil,
			expected:    "1",
		},
		{
			name:        "empty annotations",
			annotations: map[string]string{},
			expected:    "1",
		},
		{
			name: "no retry count",
			annotations: map[string]string{
				"other": "value",
			},
			expected: "1",
		},
		{
			name: "existing retry count 0",
			annotations: map[string]string{
				AnnotationRetryCount: "0",
			},
			expected: "1",
		},
		{
			name: "existing retry count 3",
			annotations: map[string]string{
				AnnotationRetryCount: "3",
			},
			expected: "4",
		},
		{
			name: "invalid count (non-numeric)",
			annotations: map[string]string{
				AnnotationRetryCount: "invalid",
			},
			expected: "1", // Should reset to 1
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a copy to avoid modifying test data
			var annotationsCopy map[string]string
			if tt.annotations != nil {
				annotationsCopy = make(map[string]string)
				for k, v := range tt.annotations {
					annotationsCopy[k] = v
				}
			}

			result := IncrementRetryCount(annotationsCopy)
			if result[AnnotationRetryCount] != tt.expected {
				t.Errorf("IncrementRetryCount() annotation = %s, want %s", result[AnnotationRetryCount], tt.expected)
			}
		})
	}
}

func TestResetRetryCount(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		shouldExist bool
	}{
		{
			name:        "nil annotations",
			annotations: nil,
			shouldExist: false,
		},
		{
			name:        "empty annotations",
			annotations: map[string]string{},
			shouldExist: false,
		},
		{
			name: "with retry count",
			annotations: map[string]string{
				AnnotationRetryCount: "5",
				"other":              "value",
			},
			shouldExist: false,
		},
		{
			name: "without retry count",
			annotations: map[string]string{
				"other": "value",
			},
			shouldExist: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a copy to avoid modifying test data
			var annotationsCopy map[string]string
			if tt.annotations != nil {
				annotationsCopy = make(map[string]string)
				for k, v := range tt.annotations {
					annotationsCopy[k] = v
				}
			}

			ResetRetryCount(annotationsCopy)
			_, exists := annotationsCopy[AnnotationRetryCount]
			if exists != tt.shouldExist {
				t.Errorf("After ResetRetryCount(), annotation exists = %v, want %v", exists, tt.shouldExist)
			}

			// Verify other annotations are preserved
			if annotationsCopy != nil && tt.annotations != nil {
				for k, v := range tt.annotations {
					if k == AnnotationRetryCount {
						continue
					}
					if annotationsCopy[k] != v {
						t.Errorf("Other annotation %s = %s, want %s", k, annotationsCopy[k], v)
					}
				}
			}
		})
	}
}

func TestHandleReconcileError(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		retryCount     int
		expectRequeue  bool
		expectError    bool
		expectDelayMin time.Duration
		expectDelayMax time.Duration
	}{
		{
			name:          "nil error (success)",
			err:           nil,
			retryCount:    0,
			expectRequeue: false,
			expectError:   false,
		},
		{
			name:          "permanent error",
			err:           errors.New("error code from Uptime Robot API: 400 Bad Request"),
			retryCount:    0,
			expectRequeue: false,
			expectError:   true,
		},
		{
			name:           "transient error - first attempt",
			err:            errors.New("error code from Uptime Robot API: 503 Service Unavailable"),
			retryCount:     0,
			expectRequeue:  true,
			expectError:    false,
			expectDelayMin: 4*time.Second + 250*time.Millisecond, // 5s - 15%
			expectDelayMax: 5*time.Second + 750*time.Millisecond, // 5s + 15%
		},
		{
			name:           "transient error - third attempt",
			err:            errors.New("error code from Uptime Robot API: 500 Internal Server Error"),
			retryCount:     3,
			expectRequeue:  true,
			expectError:    false,
			expectDelayMin: 34 * time.Second, // 40s - 15%
			expectDelayMax: 46 * time.Second, // 40s + 15%
		},
		{
			name:           "unknown error (treated as transient)",
			err:            errors.New("some unknown error"),
			retryCount:     1,
			expectRequeue:  true,
			expectError:    false,
			expectDelayMin: 8*time.Second + 500*time.Millisecond,  // 10s - 15%
			expectDelayMax: 11*time.Second + 500*time.Millisecond, // 10s + 15%
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := HandleReconcileError(tt.err, tt.retryCount)

			// Check if error was returned
			hasError := err != nil
			if hasError != tt.expectError {
				t.Errorf("HandleReconcileError() returned error = %v, want error = %v", hasError, tt.expectError)
			}

			// Check if requeue was requested
			hasRequeue := result.RequeueAfter > 0
			if hasRequeue != tt.expectRequeue {
				t.Errorf("HandleReconcileError() requeue = %v, want requeue = %v", hasRequeue, tt.expectRequeue)
			}

			// Check delay range if requeue is expected
			if tt.expectRequeue {
				if result.RequeueAfter < tt.expectDelayMin || result.RequeueAfter > tt.expectDelayMax {
					t.Errorf("HandleReconcileError() delay = %v, want between %v and %v",
						result.RequeueAfter, tt.expectDelayMin, tt.expectDelayMax)
				}
			}
		})
	}
}

func TestErrorClassificationConsistency(t *testing.T) {
	// Ensure that an error cannot be both transient and permanent
	testErrors := []error{
		errors.New("error code from Uptime Robot API: 400 Bad Request"),
		errors.New("error code from Uptime Robot API: 401 Unauthorized"),
		errors.New("error code from Uptime Robot API: 403 Forbidden"),
		errors.New("error code from Uptime Robot API: 404 Not Found"),
		errors.New("error code from Uptime Robot API: 429 Too Many Requests"),
		errors.New("error code from Uptime Robot API: 500 Internal Server Error"),
		errors.New("error code from Uptime Robot API: 502 Bad Gateway"),
		errors.New("error code from Uptime Robot API: 503 Service Unavailable"),
		errors.New("error code from Uptime Robot API: 504 Gateway Timeout"),
		&mockTimeoutError{msg: "timeout"},
		uptimerobot.ErrMonitorNotFound,
		ErrContactMissingID,
		errors.New("connection refused"),
	}

	for _, err := range testErrors {
		isTransient := IsTransientError(err)
		isPermanent := IsPermanentError(err)

		if isTransient && isPermanent {
			t.Errorf("Error classified as both transient and permanent: %v", err)
		}

		// Every error should be classified as either transient or permanent or neither
		// This is fine - some errors might be unknown
		errMsg := err.Error()
		if !isTransient && !isPermanent {
			// This is acceptable for truly unknown errors
			if !strings.Contains(errMsg, "400") &&
				!strings.Contains(errMsg, "401") &&
				!strings.Contains(errMsg, "403") &&
				!strings.Contains(errMsg, "404") &&
				!strings.Contains(errMsg, "429") &&
				!strings.Contains(errMsg, "500") &&
				!strings.Contains(errMsg, "502") &&
				!strings.Contains(errMsg, "503") &&
				!strings.Contains(errMsg, "504") &&
				!strings.Contains(errMsg, "timeout") &&
				!strings.Contains(errMsg, "connection") &&
				!strings.Contains(errMsg, "EOF") &&
				!strings.Contains(errMsg, "not found") &&
				!strings.Contains(errMsg, "missing") {
				// This is a truly unknown error, which is fine
				continue
			}
			t.Errorf("Known error not classified: %v (transient=%v, permanent=%v)", err, isTransient, isPermanent)
		}
	}
}
