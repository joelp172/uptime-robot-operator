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
	t.Setenv("UPTIME_ROBOT_RATE_LIMIT", "5")
	client := NewClient("test-api-key")
	if client.limiter == nil {
		t.Fatal("expected limiter to be non-nil")
	}
	if client.limiter.Limit() != rate.Limit(5) {
		t.Errorf("expected rate limit 5, got %v", client.limiter.Limit())
	}
}

// TestNewClient_InvalidEnvVarRateLimit verifies that an invalid
// UPTIME_ROBOT_RATE_LIMIT value falls back to the default.
func TestNewClient_InvalidEnvVarRateLimit(t *testing.T) {
	t.Setenv("UPTIME_ROBOT_RATE_LIMIT", "not-a-number")
	client := NewClient("test-api-key")
	if client.limiter == nil {
		t.Fatal("expected limiter to be non-nil")
	}
	if client.limiter.Limit() != rate.Limit(DefaultRateLimit) {
		t.Errorf("expected default rate limit %v, got %v", DefaultRateLimit, client.limiter.Limit())
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
	client := NewClient("test-api-key")
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
	// Use a small tolerance to avoid flakiness on slow systems.
	const minElapsed = 1900 * time.Millisecond
	if elapsed < minElapsed {
		t.Errorf("expected at least %v for 3 requests with 1 req/s limiter, got %v", minElapsed, elapsed)
	}
	if atomic.LoadInt32(&callCount) != numRequests {
		t.Errorf("expected %d calls, got %d", numRequests, atomic.LoadInt32(&callCount))
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
	client := NewClient("test-api-key")
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
