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
	"net"
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

// ErrMaxRetriesExceeded is returned when max retry attempts are exhausted
var ErrMaxRetriesExceeded = errors.New("max retry attempts exceeded")

// isRetryableStatusCode checks if an HTTP status code should trigger a retry
func isRetryableStatusCode(statusCode int) bool {
	switch statusCode {
	case http.StatusTooManyRequests, // 429
		http.StatusInternalServerError, // 500
		http.StatusBadGateway,          // 502
		http.StatusServiceUnavailable,  // 503
		http.StatusGatewayTimeout:      // 504
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

	// Check for standard Go network errors
	var netErr net.Error
	if errors.As(err, &netErr) {
		// Network timeout errors are retryable
		if netErr.Timeout() {
			return true
		}
	}

	// Check for specific error types
	errMsg := err.Error()

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

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Clone the request for retry
		// Note: We need to handle request body separately since Clone doesn't copy it
		reqClone := req.Clone(ctx)
		if req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, fmt.Errorf("failed to get request body for retry: %w", err)
			}
			reqClone.Body = body
		}

		resp, err := http.DefaultClient.Do(reqClone)

		// Success case
		if err == nil && resp.StatusCode < 400 {
			return resp, nil
		}

		// Build error for status codes >= 400
		if err == nil && resp.StatusCode >= 400 {
			body, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if readErr != nil {
				lastErr = fmt.Errorf("%w: %s - (failed to read body: %v)", ErrStatus, resp.Status, readErr)
			} else {
				lastErr = fmt.Errorf("%w: %s - %s", ErrStatus, resp.Status, string(body))
			}

			// Check if we should retry based on status code
			shouldRetry := isRetryableStatusCode(resp.StatusCode)

			// Don't retry if not retryable or if we've exhausted attempts
			if !shouldRetry || attempt >= maxRetries {
				return nil, lastErr
			}

			// Calculate backoff delay
			var delay time.Duration

			// Check for Retry-After header (especially for 429 responses)
			if resp.StatusCode == http.StatusTooManyRequests {
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
				return nil, ctx.Err()
			case <-time.After(delay):
				// Continue to next retry attempt
			}
		} else {
			// Network error case
			lastErr = err

			// Check if we should retry based on error type
			shouldRetry := isRetryableError(err)

			// Don't retry if not retryable or if we've exhausted attempts
			if !shouldRetry || attempt >= maxRetries {
				return nil, lastErr
			}

			// Calculate backoff delay
			delay := calculateBackoff(attempt, baseDelay, maxDelay, jitterFraction)

			// Wait before retry, respecting context cancellation
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
				// Continue to next retry attempt
			}
		}
	}

	// Should not reach here, but return last error if we do
	if lastErr != nil {
		return nil, fmt.Errorf("%w: %v", ErrMaxRetriesExceeded, lastErr)
	}
	return nil, ErrMaxRetriesExceeded
}
