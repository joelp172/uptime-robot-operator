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

package uptimerobot

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultMaxRetries is the default maximum number of retry attempts
	DefaultMaxRetries = 5
	// DefaultBaseDelay is the initial delay for exponential backoff (1 second)
	DefaultBaseDelay = 1 * time.Second
	// DefaultMaxDelay is the maximum delay between retries (60 seconds)
	DefaultMaxDelay = 60 * time.Second
	// DefaultJitterFraction is the fraction of delay to use for jitter (0.1 = 10%)
	DefaultJitterFraction = 0.1
)

var (
	// ErrMaxRetriesExceeded is returned when max retry attempts are exhausted
	ErrMaxRetriesExceeded = errors.New("max retry attempts exceeded")
)

// isRetryableStatusCode checks if an HTTP status code should trigger a retry
func isRetryableStatusCode(statusCode int) bool {
	switch statusCode {
	case http.StatusTooManyRequests, // 429
		http.StatusInternalServerError, // 500
		http.StatusBadGateway,          // 502
		http.StatusServiceUnavailable,  // 503
		http.StatusGatewayTimeout:      // 504
		return true
	case http.StatusConflict: // 409 - conflicts may be transient
		return true
	default:
		return false
	}
}

// isRetryableError checks if an error should trigger a retry
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check for status errors
	if errors.Is(err, ErrStatus) {
		errMsg := err.Error()
		// Extract status code from error message
		// Format: "error code from Uptime Robot API: 429 Too Many Requests - ..."
		if strings.Contains(errMsg, "429") ||
			strings.Contains(errMsg, "500") ||
			strings.Contains(errMsg, "502") ||
			strings.Contains(errMsg, "503") ||
			strings.Contains(errMsg, "504") ||
			strings.Contains(errMsg, "409") {
			return true
		}
		// Non-retryable client errors
		if strings.Contains(errMsg, "400") ||
			strings.Contains(errMsg, "401") ||
			strings.Contains(errMsg, "403") ||
			strings.Contains(errMsg, "404") {
			return false
		}
	}

	// Network errors are generally retryable
	if strings.Contains(err.Error(), "connection") ||
		strings.Contains(err.Error(), "timeout") ||
		strings.Contains(err.Error(), "EOF") {
		return true
	}

	return false
}

// parseRetryAfter extracts the retry delay from Retry-After header
// Supports both delay-seconds format and HTTP-date format
func parseRetryAfter(retryAfter string) time.Duration {
	if retryAfter == "" {
		return 0
	}

	// Try parsing as seconds (delay-seconds format)
	if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds > 0 {
		delay := time.Duration(seconds) * time.Second
		if delay > DefaultMaxDelay {
			return DefaultMaxDelay
		}
		return delay
	}

	// Try parsing as HTTP-date format
	if t, err := http.ParseTime(retryAfter); err == nil {
		delay := time.Until(t)
		if delay < 0 {
			return 0
		}
		if delay > DefaultMaxDelay {
			return DefaultMaxDelay
		}
		return delay
	}

	return 0
}

// calculateBackoff calculates exponential backoff delay with jitter
func calculateBackoff(attempt int, baseDelay, maxDelay time.Duration, jitterFraction float64) time.Duration {
	// Exponential backoff: baseDelay * 2^attempt
	exp := math.Pow(2, float64(attempt))
	delay := time.Duration(float64(baseDelay) * exp)

	// Cap at max delay
	if delay > maxDelay {
		delay = maxDelay
	}

	// Add jitter to prevent thundering herd
	// Jitter is a random value between -jitterFraction and +jitterFraction of the delay
	if jitterFraction > 0 {
		jitterRange := float64(delay) * jitterFraction
		jitter := (rand.Float64()*2 - 1) * jitterRange // Random value in [-jitterRange, +jitterRange]
		delay = time.Duration(float64(delay) + jitter)
	}

	// Ensure delay is positive
	if delay < 0 {
		delay = 0
	}

	return delay
}

// doWithRetry wraps an HTTP request with retry logic
func (c Client) doWithRetry(ctx context.Context, req *http.Request) (*http.Response, error) {
	maxRetries := DefaultMaxRetries
	baseDelay := DefaultBaseDelay
	maxDelay := DefaultMaxDelay
	jitterFraction := DefaultJitterFraction

	var lastErr error
	var lastResp *http.Response

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Clone the request for retry (in case body needs to be re-read)
		reqClone := req.Clone(ctx)

		resp, err := http.DefaultClient.Do(reqClone)

		// Success case
		if err == nil && resp.StatusCode < 400 {
			return resp, nil
		}

		// Store error/response for potential retry decision
		lastErr = err
		lastResp = resp

		// Build error for status codes >= 400
		if err == nil && resp.StatusCode >= 400 {
			defer func() { _ = resp.Body.Close() }()
			body, _ := io.ReadAll(resp.Body)
			lastErr = fmt.Errorf("%w: %s - %s", ErrStatus, resp.Status, string(body))
		}

		// Check if we should retry
		shouldRetry := false
		if err != nil {
			shouldRetry = isRetryableError(err)
		} else if resp != nil {
			shouldRetry = isRetryableStatusCode(resp.StatusCode)
		}

		// Don't retry if not retryable or if we've exhausted attempts
		if !shouldRetry || attempt >= maxRetries {
			return lastResp, lastErr
		}

		// Calculate backoff delay
		var delay time.Duration

		// Check for Retry-After header (especially for 429 responses)
		if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
			if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
				if parsed := parseRetryAfter(retryAfter); parsed > 0 {
					delay = parsed
				}
			}
		}

		// Use exponential backoff if no Retry-After header or invalid
		if delay == 0 {
			delay = calculateBackoff(attempt, baseDelay, maxDelay, jitterFraction)
		}

		// Wait before retry, respecting context cancellation
		select {
		case <-ctx.Done():
			return lastResp, ctx.Err()
		case <-time.After(delay):
			// Continue to next retry attempt
		}
	}

	// Should not reach here, but return last error if we do
	if lastErr != nil {
		return lastResp, fmt.Errorf("%w: %v", ErrMaxRetriesExceeded, lastErr)
	}
	return lastResp, ErrMaxRetriesExceeded
}
