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
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// TestNewClient_DefaultRateLimit verifies that NewClient sets a limiter with the
// correct default rate.
func TestNewClient_DefaultRateLimit(t *testing.T) {
	resetClientConfig()
	resetGlobalLimiters()
	t.Cleanup(func() { resetGlobalLimiters(); resetClientConfig() })
	client := NewClient("test-api-key")
	if client.limiter == nil {
		t.Fatal("expected limiter to be non-nil")
	}
	if client.limiter.Limit() != rate.Limit(DefaultRateLimit) {
		t.Errorf("expected rate limit %v, got %v", DefaultRateLimit, client.limiter.Limit())
	}
}

// TestNewClient_EnvVarRateLimit verifies that UPTIME_ROBOT_RATE_LIMIT overrides
// the default rate.
func TestNewClient_EnvVarRateLimit(t *testing.T) {
	resetClientConfig()
	resetGlobalLimiters()
	t.Cleanup(func() { resetGlobalLimiters(); resetClientConfig() })
	t.Setenv("UPTIME_ROBOT_RATE_LIMIT", "5")
	client := NewClient("test-api-key-rate5")
	if client.limiter == nil {
		t.Fatal("expected limiter to be non-nil")
	}
	if client.limiter.Limit() != rate.Limit(5) {
		t.Errorf("expected rate limit 5, got %v", client.limiter.Limit())
	}
}

// TestNewClient_InvalidEnvVarRateLimit verifies that invalid
// UPTIME_ROBOT_RATE_LIMIT values fall back to the default.
func TestNewClient_InvalidEnvVarRateLimit(t *testing.T) {
	t.Cleanup(func() { resetGlobalLimiters(); resetClientConfig() })
	tests := []struct {
		name  string
		value string
	}{
		{"non-numeric", "not-a-number"},
		{"zero", "0"},
		{"negative", "-1"},
		{"float", "5.5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetClientConfig()
			resetGlobalLimiters()
			t.Setenv("UPTIME_ROBOT_RATE_LIMIT", tt.value)
			client := NewClient("test-api-key-invalid-" + tt.name)
			if client.limiter == nil {
				t.Fatal("expected limiter to be non-nil")
			}
			if client.limiter.Limit() != rate.Limit(DefaultRateLimit) {
				t.Errorf("expected default rate limit %v, got %v", DefaultRateLimit, client.limiter.Limit())
			}
		})
	}
}

// TestNewClient_SharesLimiterPerAPIKey verifies that multiple NewClient calls
// with the same API key and rate share a single limiter instance.
func TestNewClient_SharesLimiterPerAPIKey(t *testing.T) {
	resetClientConfig()
	resetGlobalLimiters()
	t.Cleanup(func() { resetGlobalLimiters(); resetClientConfig() })
	const key = "test-api-key-shared-limiter"
	c1 := NewClient(key)
	c2 := NewClient(key)
	if c1.limiter != c2.limiter {
		t.Error("expected clients with the same API key to share a limiter instance")
	}

	c3 := NewClient("test-api-key-different")
	if c1.limiter == c3.limiter {
		t.Error("expected clients with different API keys to have separate limiters")
	}
}

// TestDoWithRetry_RateLimiterThrottles verifies that the rate limiter delays
// rapid sequential requests.
func TestDoWithRetry_RateLimiterThrottles(t *testing.T) {
	var callCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create a client with a very slow rate (1 request per second) and burst 1.
	client := NewClient("test-api-key-throttle")
	client.limiter = rate.NewLimiter(rate.Every(time.Second), 1)
	// Use fast retries so the test doesn't wait on backoff.
	client.baseDelay = time.Millisecond
	client.maxDelay = time.Millisecond

	const numRequests = 3
	start := time.Now()

	for i := 0; i < numRequests; i++ {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		_, err = client.doWithRetry(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	elapsed := time.Since(start)

	// With burst=1 and rate=1/s, 3 requests should take at least ~2 seconds.
	// Use a generous tolerance to avoid flakiness on slow CI runners.
	const minElapsed = 1500 * time.Millisecond
	if elapsed < minElapsed {
		t.Errorf("expected at least %v for 3 requests with 1 req/s limiter, got %v", minElapsed, elapsed)
	}
	if atomic.LoadInt32(&callCount) != numRequests {
		t.Errorf("expected %d calls, got %d", numRequests, atomic.LoadInt32(&callCount))
	}
}

// TestDoWithRetry_RateLimiterAppliesToRetries verifies that the rate limiter
// is applied to every retry attempt, not just the initial request.
func TestDoWithRetry_RateLimiterAppliesToRetries(t *testing.T) {
	var attemptCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attemptCount, 1)
		if count == 1 {
			// First attempt returns 429 to trigger a retry.
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"rate limited"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// burst=1: initial attempt consumes the only token; the retry must wait ~1s.
	client := NewClient("test-api-key-retry-rl")
	client.limiter = rate.NewLimiter(rate.Every(time.Second), 1)
	client.baseDelay = time.Millisecond // fast backoff so we don't wait on it
	client.maxDelay = time.Millisecond

	start := time.Now()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	_, err = client.doWithRetry(context.Background(), req)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The first attempt consumes the burst token; the retry must wait ~1s for a new token.
	const minElapsed = 700 * time.Millisecond
	if elapsed < minElapsed {
		t.Errorf("expected retry to be rate-limited (>= %v), got %v", minElapsed, elapsed)
	}
	if atomic.LoadInt32(&attemptCount) != 2 {
		t.Errorf("expected 2 calls, got %d", atomic.LoadInt32(&attemptCount))
	}
}

// TestDoWithRetry_RateLimiterContextCancellation verifies that the rate limiter
// respects context cancellation.
func TestDoWithRetry_RateLimiterContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create a client with a very slow rate so the second request must wait.
	client := NewClient("test-api-key-cancel")
	client.limiter = rate.NewLimiter(rate.Every(10*time.Second), 1)

	// The first request should be granted immediately (burst=1).
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	_, err = client.doWithRetry(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error on first request: %v", err)
	}

	// The second request must wait 10 seconds — cancel it quickly.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	req2, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	start := time.Now()
	_, err = client.doWithRetry(ctx, req2)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected context cancellation error, got nil")
	}
	// Should fail quickly (well under the 10s wait).
	if elapsed > time.Second {
		t.Errorf("expected fast cancellation, but waited %v", elapsed)
	}
}

// TestDoWithRetry_NilLimiter verifies that a Client with a nil limiter
// (e.g., constructed directly without NewClient) does not panic.
func TestDoWithRetry_NilLimiter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Directly constructed Client with nil limiter.
	client := Client{}
	client.baseDelay = time.Millisecond
	client.maxDelay = time.Millisecond

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	_, err = client.doWithRetry(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error with nil limiter: %v", err)
	}
}
