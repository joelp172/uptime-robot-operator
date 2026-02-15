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
	"math"
	"math/rand/v2"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/joelp172/uptime-robot-operator/internal/uptimerobot"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	// BackoffInitialDelay is the initial delay for exponential backoff (5 seconds)
	BackoffInitialDelay = 5 * time.Second
	// BackoffMaxDelay is the maximum delay between retries (5 minutes)
	BackoffMaxDelay = 5 * time.Minute
	// BackoffJitterFraction is the fraction of delay to use for jitter (0.15 = 15%)
	BackoffJitterFraction = 0.15

	// AnnotationRetryCount tracks the number of retry attempts for a resource
	AnnotationRetryCount = "uptimerobot.com/retry-count"
)

// IsTransientError checks if an error should trigger exponential backoff retry.
// Transient errors include:
// - 5xx server errors (500, 502, 503, 504)
// - 429 Too Many Requests
// - Network timeouts
// - Connection errors (refused, reset, broken pipe)
// - EOF errors during request/response
func IsTransientError(err error) bool {
	if err == nil {
		return false
	}

	// Check for ErrMaxRetriesExceeded - this wraps the underlying error
	// If we've already exhausted retries at the API client level, the error is transient
	if errors.Is(err, uptimerobot.ErrMaxRetriesExceeded) {
		return true
	}

	// Check for standard Go network errors
	var netErr net.Error
	if errors.As(err, &netErr) {
		// Network timeout errors are retryable
		if netErr.Timeout() {
			return true
		}
	}

	errMsg := err.Error()

	// Check for HTTP status codes in error message
	// The uptimerobot client wraps errors with status codes
	if strings.Contains(errMsg, "429") || // Too Many Requests
		strings.Contains(errMsg, "500") || // Internal Server Error
		strings.Contains(errMsg, "502") || // Bad Gateway
		strings.Contains(errMsg, "503") || // Service Unavailable
		strings.Contains(errMsg, "504") { // Gateway Timeout
		return true
	}

	// Connection errors are generally retryable
	if strings.Contains(errMsg, "connection refused") ||
		strings.Contains(errMsg, "connection reset") ||
		strings.Contains(errMsg, "broken pipe") {
		return true
	}

	// EOF errors during request/response are retryable
	if strings.Contains(errMsg, "EOF") || strings.Contains(errMsg, "unexpected EOF") {
		return true
	}

	return false
}

// IsPermanentError checks if an error is permanent and should not be retried.
// Permanent errors include:
// - 400 Bad Request (validation errors)
// - 401 Unauthorized (invalid API key)
// - 403 Forbidden (insufficient permissions)
// - 404 Not Found (when not during adoption/lookup)
// - Validation errors (missing fields, invalid values)
func IsPermanentError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := err.Error()

	// Check for HTTP status codes in error message
	if strings.Contains(errMsg, "400") || // Bad Request
		strings.Contains(errMsg, "401") || // Unauthorized
		strings.Contains(errMsg, "403") || // Forbidden
		strings.Contains(errMsg, "404") { // Not Found
		return true
	}

	// Check for specific uptimerobot errors
	if uptimerobot.IsNotFound(err) {
		return true
	}

	// Validation errors
	if errors.Is(err, ErrContactMissingID) ||
		errors.Is(err, ErrSecretMissingKey) ||
		errors.Is(err, ErrKeyNotFound) ||
		errors.Is(err, ErrEmptyKey) {
		return true
	}

	return false
}

// CalculateRequeueDelay calculates exponential backoff delay with jitter.
// The delay follows the pattern: baseDelay * 2^attempt
// For example with baseDelay=5s: 5s, 10s, 20s, 40s, 80s, capped at 5m
// Jitter of ±15% is added to prevent thundering herd across resources.
func CalculateRequeueDelay(attempt int) time.Duration {
	// Exponential backoff: baseDelay * 2^attempt
	exp := math.Pow(2, float64(attempt))
	delay := time.Duration(float64(BackoffInitialDelay) * exp)

	// Cap at max delay
	if delay > BackoffMaxDelay {
		delay = BackoffMaxDelay
	}

	// Add jitter to prevent thundering herd
	// Jitter is a random value between -jitterFraction and +jitterFraction of the delay
	jitterRange := float64(delay) * BackoffJitterFraction
	jitter := (rand.Float64()*2 - 1) * jitterRange // Random value in [-jitterRange, +jitterRange]
	delay = time.Duration(float64(delay) + jitter)

	// Ensure delay is positive and at least 1 second
	if delay < time.Second {
		delay = time.Second
	}

	return delay
}

// GetRetryCount extracts the retry count from resource annotations.
// Returns 0 if the annotation is not present or invalid.
func GetRetryCount(annotations map[string]string) int {
	if annotations == nil {
		return 0
	}
	countStr, exists := annotations[AnnotationRetryCount]
	if !exists {
		return 0
	}
	count, err := strconv.Atoi(countStr)
	if err != nil || count < 0 {
		return 0
	}
	return count
}

// IncrementRetryCount increments the retry count annotation.
// If annotations map is nil, it creates a new one.
func IncrementRetryCount(annotations map[string]string) map[string]string {
	if annotations == nil {
		annotations = make(map[string]string)
	}
	count := GetRetryCount(annotations)
	annotations[AnnotationRetryCount] = fmt.Sprintf("%d", count+1)
	return annotations
}

// ResetRetryCount removes the retry count annotation.
func ResetRetryCount(annotations map[string]string) {
	if annotations != nil {
		delete(annotations, AnnotationRetryCount)
	}
}

// HandleReconcileError processes an error from reconciliation and returns
// the appropriate ctrl.Result and error to return from Reconcile().
// For transient errors, it returns RequeueAfter with exponential backoff.
// For permanent errors, it returns the error directly without requeue.
// For nil errors (success), it resets retry count and returns no error.
func HandleReconcileError(err error, retryCount int) (ctrl.Result, error) {
	if err == nil {
		// Success - no retry needed
		return ctrl.Result{}, nil
	}

	if IsPermanentError(err) {
		// Permanent error - don't retry, return error immediately
		// This causes controller-runtime to requeue with rate limiting
		return ctrl.Result{}, err
	}

	if IsTransientError(err) {
		// Transient error - use exponential backoff
		delay := CalculateRequeueDelay(retryCount)
		return ctrl.Result{RequeueAfter: delay}, nil
	}

	// Unknown error type - treat as transient but log for investigation
	// Use a moderate delay
	delay := CalculateRequeueDelay(retryCount)
	return ctrl.Result{RequeueAfter: delay}, nil
}
