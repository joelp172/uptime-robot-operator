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
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestIsRetryableStatusCode(t *testing.T) {
	tests := []struct {
		statusCode int
		expected   bool
	}{
		{http.StatusOK, false},                 // 200
		{http.StatusBadRequest, false},         // 400
		{http.StatusUnauthorized, false},       // 401
		{http.StatusForbidden, false},          // 403
		{http.StatusNotFound, false},           // 404
		{http.StatusConflict, false},           // 409 - handled by caller-specific logic (e.g. duplicate adoption)
		{http.StatusTooManyRequests, true},     // 429
		{http.StatusInternalServerError, true}, // 500
		{http.StatusBadGateway, true},          // 502
		{http.StatusServiceUnavailable, true},  // 503
		{http.StatusGatewayTimeout, true},      // 504
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("StatusCode_%d", tt.statusCode), func(t *testing.T) {
			result := isRetryableStatusCode(tt.statusCode)
			if result != tt.expected {
				t.Errorf("isRetryableStatusCode(%d) = %v, want %v", tt.statusCode, result, tt.expected)
			}
		})
	}
}

// mockTimeoutError is a mock net.Error that reports as a timeout
type mockTimeoutError struct {
	error
}

func (e mockTimeoutError) Timeout() bool   { return true }
func (e mockTimeoutError) Temporary() bool { return true }

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"connection refused", fmt.Errorf("connection refused"), true},
		{"connection reset", fmt.Errorf("connection reset by peer"), true},
		{"broken pipe", fmt.Errorf("broken pipe"), true},
		{"EOF error", fmt.Errorf("unexpected EOF"), true},
		{"timeout error string", fmt.Errorf("timeout exceeded"), false},                // String matching not used for timeouts
		{"timeout error net.Error", mockTimeoutError{fmt.Errorf("i/o timeout")}, true}, // Actual net.Error with Timeout()
		{"other error", fmt.Errorf("some other error"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRetryableError(tt.err)
			if result != tt.expected {
				t.Errorf("isRetryableError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		name       string
		retryAfter string
		wantMin    time.Duration
		wantMax    time.Duration
	}{
		{"empty string", "", 0, 0},
		{"delay in seconds", "10", 10 * time.Second, 10 * time.Second},
		{"large delay capped", "120", DefaultMaxDelay, DefaultMaxDelay},
		{"HTTP date future", time.Now().Add(5 * time.Second).Format(http.TimeFormat), 4 * time.Second, 6 * time.Second},
		{"HTTP date past", time.Now().Add(-5 * time.Second).Format(http.TimeFormat), 0, 0},
		{"invalid format", "invalid", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseRetryAfter(tt.retryAfter)
			if result < tt.wantMin || result > tt.wantMax {
				t.Errorf("parseRetryAfter(%q) = %v, want between %v and %v", tt.retryAfter, result, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestCalculateBackoff(t *testing.T) {
	tests := []struct {
		name           string
		attempt        int
		baseDelay      time.Duration
		maxDelay       time.Duration
		jitterFraction float64
		wantMin        time.Duration
		wantMax        time.Duration
	}{
		{"attempt 0", 0, 1 * time.Second, 60 * time.Second, 0.1, 900 * time.Millisecond, 1100 * time.Millisecond},
		{"attempt 1", 1, 1 * time.Second, 60 * time.Second, 0.1, 1800 * time.Millisecond, 2200 * time.Millisecond},
		{"attempt 2", 2, 1 * time.Second, 60 * time.Second, 0.1, 3600 * time.Millisecond, 4400 * time.Millisecond},
		{"attempt 3", 3, 1 * time.Second, 60 * time.Second, 0.1, 7200 * time.Millisecond, 8800 * time.Millisecond},
		{"capped at max", 10, 1 * time.Second, 60 * time.Second, 0.1, 54 * time.Second, 66 * time.Second},
		{"no jitter", 1, 1 * time.Second, 60 * time.Second, 0, 2 * time.Second, 2 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateBackoff(tt.attempt, tt.baseDelay, tt.maxDelay, tt.jitterFraction)
			if result < tt.wantMin || result > tt.wantMax {
				t.Errorf("calculateBackoff(attempt=%d) = %v, want between %v and %v", tt.attempt, result, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestDoWithRetry_Success(t *testing.T) {
	client := NewClient("test-api-key")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := client.doWithRetry(context.Background(), req)
	if err != nil {
		t.Errorf("doWithRetry() error = %v, want nil", err)
	}
	if resp == nil {
		t.Fatal("doWithRetry() response is nil")
		return
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("doWithRetry() status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if err := resp.Body.Close(); err != nil {
		t.Errorf("failed to close response body: %v", err)
	}
}

func TestDoWithRetry_429WithRetryAfter(t *testing.T) {
	client := NewClient("test-api-key")
	var attemptCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attemptCount, 1)
		if count < 3 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"ThrottlerException: Too Many Requests"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	start := time.Now()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := client.doWithRetry(context.Background(), req)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("doWithRetry() error = %v, want nil", err)
	}
	if resp == nil {
		t.Fatal("doWithRetry() response is nil")
		return
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("doWithRetry() status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if err := resp.Body.Close(); err != nil {
		t.Errorf("failed to close response body: %v", err)
	}

	finalAttempts := atomic.LoadInt32(&attemptCount)
	if finalAttempts != 3 {
		t.Errorf("expected 3 attempts, got %d", finalAttempts)
	}

	// Should take at least 2 seconds (2 retries with 1s Retry-After each)
	if elapsed < 2*time.Second {
		t.Errorf("expected at least 2 seconds of retries, got %v", elapsed)
	}
}

func TestDoWithRetry_429ExponentialBackoff(t *testing.T) {
	client := NewClient("test-api-key")
	var attemptCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attemptCount, 1)
		if count < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"ThrottlerException: Too Many Requests"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	start := time.Now()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := client.doWithRetry(context.Background(), req)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("doWithRetry() error = %v, want nil", err)
	}
	if resp == nil {
		t.Fatal("doWithRetry() response is nil")
		return
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("doWithRetry() status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if err := resp.Body.Close(); err != nil {
		t.Errorf("failed to close response body: %v", err)
	}

	finalAttempts := atomic.LoadInt32(&attemptCount)
	if finalAttempts != 3 {
		t.Errorf("expected 3 attempts, got %d", finalAttempts)
	}

	// Should take at least 1s (first retry) + 2s (second retry) = 3s total
	// With jitter it may be slightly less, but should be at least 2.5s
	if elapsed < 2500*time.Millisecond {
		t.Errorf("expected at least 2.5 seconds of exponential backoff, got %v", elapsed)
	}
}

func TestDoWithRetry_MaxRetriesExceeded(t *testing.T) {
	client := NewClient("test-api-key")
	client.maxRetries = 2
	client.baseDelay = 10 * time.Millisecond
	client.maxDelay = 20 * time.Millisecond
	client.jitterFraction = 0
	var attemptCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attemptCount, 1)
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"ThrottlerException: Too Many Requests"}`))
	}))
	defer server.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := client.doWithRetry(context.Background(), req)

	if err == nil {
		t.Error("doWithRetry() error = nil, want error")
	}
	if !errors.Is(err, ErrMaxRetriesExceeded) {
		t.Errorf("doWithRetry() error = %v, want error wrapping ErrMaxRetriesExceeded", err)
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("doWithRetry() error = %v, want error containing '429'", err)
	}
	if resp != nil {
		_ = resp.Body.Close()
	}

	// Should attempt initial request + configured retries.
	finalAttempts := atomic.LoadInt32(&attemptCount)
	if finalAttempts != int32(client.maxRetries+1) {
		t.Errorf("expected %d attempts (1 initial + %d retries), got %d", client.maxRetries+1, client.maxRetries, finalAttempts)
	}
}

func TestDoWithRetry_NonRetryableError(t *testing.T) {
	client := NewClient("test-api-key")
	var attemptCount int32

	tests := []struct {
		name       string
		statusCode int
	}{
		{"400 Bad Request", http.StatusBadRequest},
		{"401 Unauthorized", http.StatusUnauthorized},
		{"403 Forbidden", http.StatusForbidden},
		{"404 Not Found", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atomic.StoreInt32(&attemptCount, 0)

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&attemptCount, 1)
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(`{"error":"client error"}`))
			}))
			t.Cleanup(server.Close)

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			resp, err := client.doWithRetry(context.Background(), req)

			if err == nil {
				t.Error("doWithRetry() error = nil, want error")
			}
			if !strings.Contains(err.Error(), strconv.Itoa(tt.statusCode)) {
				t.Errorf("doWithRetry() error = %v, want error containing '%d'", err, tt.statusCode)
			}
			if resp != nil {
				_ = resp.Body.Close()
			}

			// Should only attempt once (no retries for non-retryable errors)
			finalAttempts := atomic.LoadInt32(&attemptCount)
			if finalAttempts != 1 {
				t.Errorf("expected 1 attempt (no retries), got %d", finalAttempts)
			}
		})
	}
}

func TestDoWithRetry_ContextCancellation(t *testing.T) {
	client := NewClient("test-api-key")
	var attemptCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attemptCount, 1)
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"ThrottlerException: Too Many Requests"}`))
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel context after first retry
	go func() {
		time.Sleep(1500 * time.Millisecond)
		cancel()
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	_, retryErr := client.doWithRetry(ctx, req)

	if retryErr == nil {
		t.Error("doWithRetry() error = nil, want context cancellation error")
	}
	if !strings.Contains(retryErr.Error(), "context canceled") {
		t.Errorf("doWithRetry() error = %v, want context cancellation error", retryErr)
	}

	// Should have attempted at least once but not all retries
	finalAttempts := atomic.LoadInt32(&attemptCount)
	if finalAttempts < 1 || finalAttempts > DefaultMaxRetries+1 {
		t.Errorf("expected 1 to %d attempts due to cancellation, got %d", DefaultMaxRetries+1, finalAttempts)
	}
}

func TestDoWithRetry_429RespectsRetryAfterHeader(t *testing.T) {
	client := NewClient("test-api-key")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate UptimeRobot rate limit response with Retry-After header
		// Note: X-RateLimit-* headers are informational only and not used by retry logic
		w.Header().Set("X-RateLimit-Limit", "10")
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(60*time.Second).Unix(), 10))
		w.Header().Set("Retry-After", "2")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"ThrottlerException: Too Many Requests"}`))
	}))
	defer server.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, retryErr := client.doWithRetry(ctx, req)
	elapsed := time.Since(start)

	if retryErr == nil {
		t.Fatal("expected deadline error, got nil")
	}
	if !strings.Contains(retryErr.Error(), context.DeadlineExceeded.Error()) {
		t.Errorf("expected deadline exceeded error, got %v", retryErr)
	}

	// Should respect Retry-After header (2 seconds)
	if elapsed < 2*time.Second {
		t.Errorf("expected at least 2 seconds delay (Retry-After), got %v", elapsed)
	}
}

func TestDoWithRetry_POSTRequestWithBody(t *testing.T) {
	client := NewClient("test-api-key")
	var attemptCount int32
	var receivedBodies []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attemptCount, 1)

		// Read and store the request body
		body, _ := io.ReadAll(r.Body)
		receivedBodies = append(receivedBodies, string(body))

		if count < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"ThrottlerException: Too Many Requests"}`))
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"status":"created"}`))
	}))
	defer server.Close()

	// Create a POST request with a body using newRequest (which sets GetBody)
	testBody := map[string]string{"test": "data", "foo": "bar"}
	req, err := client.newRequest(context.Background(), http.MethodPost, "test-endpoint", testBody)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	// Update URL to point to test server
	req.URL.Scheme = "http"
	req.URL.Host = strings.TrimPrefix(server.URL, "http://")

	resp, err := client.doWithRetry(context.Background(), req)
	if err != nil {
		t.Errorf("doWithRetry() error = %v, want nil", err)
	}
	if resp == nil {
		t.Fatal("doWithRetry() response is nil")
		return
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("doWithRetry() status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}
	if err := resp.Body.Close(); err != nil {
		t.Errorf("failed to close response body: %v", err)
	}

	finalAttempts := atomic.LoadInt32(&attemptCount)
	if finalAttempts != 3 {
		t.Errorf("expected 3 attempts, got %d", finalAttempts)
	}

	// Verify that the body was sent on all retry attempts
	expectedBody := `{"foo":"bar","test":"data"}`
	for i, body := range receivedBodies {
		if body != expectedBody {
			t.Errorf("attempt %d: expected body %q, got %q", i+1, expectedBody, body)
		}
	}
}

func TestDoWithRetry_PATCHRequestWithBody(t *testing.T) {
	client := NewClient("test-api-key")
	var attemptCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attemptCount, 1)

		// Verify method and body presence
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH method, got %s", r.Method)
		}

		if count < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"Service Unavailable"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"updated"}`))
	}))
	defer server.Close()

	// Create a PATCH request with a body
	testBody := map[string]interface{}{"updated": true, "value": 42}
	req, err := client.newRequest(context.Background(), http.MethodPatch, "test-endpoint", testBody)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	// Update URL to point to test server
	req.URL.Scheme = "http"
	req.URL.Host = strings.TrimPrefix(server.URL, "http://")

	resp, err := client.doWithRetry(context.Background(), req)
	if err != nil {
		t.Errorf("doWithRetry() error = %v, want nil", err)
	}
	if resp == nil {
		t.Fatal("doWithRetry() response is nil")
		return
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("doWithRetry() status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if err := resp.Body.Close(); err != nil {
		t.Errorf("failed to close response body: %v", err)
	}

	finalAttempts := atomic.LoadInt32(&attemptCount)
	if finalAttempts != 2 {
		t.Errorf("expected 2 attempts, got %d", finalAttempts)
	}
}

func TestNewClient_HasHTTPClientWithTimeout(t *testing.T) {
	resetClientConfig()
	t.Cleanup(resetClientConfig)

	client := NewClient("test-api-key")

	if client.httpClient == nil {
		t.Fatal("NewClient() httpClient is nil, want non-nil *http.Client")
	}
	if client.httpClient.Timeout == 0 {
		t.Errorf("NewClient() httpClient.Timeout = 0, want non-zero timeout")
	}
	if client.httpClient.Timeout != DefaultHTTPTimeout {
		t.Errorf("NewClient() httpClient.Timeout = %v, want %v", client.httpClient.Timeout, DefaultHTTPTimeout)
	}
}

// TestNewClient_EnvVarHTTPTimeout verifies that UPTIME_ROBOT_HTTP_TIMEOUT
// overrides the default timeout.
func TestNewClient_EnvVarHTTPTimeout(t *testing.T) {
	resetClientConfig()
	t.Cleanup(resetClientConfig)
	t.Setenv("UPTIME_ROBOT_HTTP_TIMEOUT", "10s")

	client := NewClient("test-api-key-timeout10")
	if client.httpClient == nil {
		t.Fatal("NewClient() httpClient is nil, want non-nil *http.Client")
	}
	if client.httpClient.Timeout != 10*time.Second {
		t.Errorf("NewClient() httpClient.Timeout = %v, want %v", client.httpClient.Timeout, 10*time.Second)
	}
}

// TestNewClient_InvalidEnvVarHTTPTimeout verifies that invalid
// UPTIME_ROBOT_HTTP_TIMEOUT values fall back to the default.
func TestNewClient_InvalidEnvVarHTTPTimeout(t *testing.T) {
	t.Cleanup(resetClientConfig)
	tests := []struct {
		name  string
		value string
	}{
		{"non-duration", "abc"},
		{"zero", "0s"},
		{"negative", "-5s"},
		{"empty-looking", " "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetClientConfig()
			t.Setenv("UPTIME_ROBOT_HTTP_TIMEOUT", tt.value)
			client := NewClient("test-api-key-invalid-" + tt.name)
			if client.httpClient == nil {
				t.Fatal("NewClient() httpClient is nil, want non-nil *http.Client")
			}
			if client.httpClient.Timeout != DefaultHTTPTimeout {
				t.Errorf("NewClient() httpClient.Timeout = %v, want default %v", client.httpClient.Timeout, DefaultHTTPTimeout)
			}
		})
	}
}

func TestDoWithRetry_HTTPClientTimeout(t *testing.T) {
	// Create a server that hangs until the test ends, simulating a slow/hung upstream.
	serverDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-serverDone // Block until the test signals completion.
	}))
	defer func() {
		close(serverDone)
		server.Close()
	}()

	// Use a very short timeout so the test completes quickly.
	client := NewClient("test-api-key")
	client.httpClient = &http.Client{Timeout: 50 * time.Millisecond}
	// Keep retries and delays minimal to avoid long test runtime.
	client.maxRetries = 1
	client.baseDelay = 10 * time.Millisecond
	client.maxDelay = 10 * time.Millisecond
	client.jitterFraction = 0

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	start := time.Now()
	_, reqErr := client.doWithRetry(context.Background(), req)
	elapsed := time.Since(start)

	if reqErr == nil {
		t.Fatal("doWithRetry() error = nil, want timeout error")
	}

	// The timeout wraps ErrMaxRetriesExceeded (timeout is retryable); verify the
	// error message contains a timeout indication.
	if !strings.Contains(reqErr.Error(), "timeout") && !strings.Contains(reqErr.Error(), "deadline exceeded") {
		t.Errorf("doWithRetry() error = %v, want timeout or deadline exceeded error", reqErr)
	}

	// Should have returned well under 1 second (client timeout is 50ms, 1 retry + 10ms backoff).
	if elapsed > 1*time.Second {
		t.Errorf("doWithRetry() took %v, expected to timeout quickly", elapsed)
	}
}

// TestDoWithRetry_500InternalServerError verifies that a 500 response triggers
// exponential-backoff retries and eventually succeeds.
func TestDoWithRetry_500InternalServerError(t *testing.T) {
	client := NewClient("test-api-key")
	client.baseDelay = 10 * time.Millisecond
	client.maxDelay = 50 * time.Millisecond
	client.jitterFraction = 0
	var attemptCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attemptCount, 1)
		if count < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"Internal Server Error"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, retryErr := client.doWithRetry(context.Background(), req)

	if retryErr != nil {
		t.Errorf("doWithRetry() error = %v, want nil", retryErr)
	}
	if resp == nil {
		t.Fatal("doWithRetry() response is nil")
		return
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("doWithRetry() status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	_ = resp.Body.Close()

	if got := atomic.LoadInt32(&attemptCount); got != 3 {
		t.Errorf("expected 3 attempts, got %d", got)
	}
}

// TestDoWithRetry_502BadGateway verifies that a 502 response triggers retries.
func TestDoWithRetry_502BadGateway(t *testing.T) {
	client := NewClient("test-api-key")
	client.baseDelay = 10 * time.Millisecond
	client.maxDelay = 50 * time.Millisecond
	client.jitterFraction = 0
	var attemptCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attemptCount, 1)
		if count < 3 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":"Bad Gateway"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, retryErr := client.doWithRetry(context.Background(), req)

	if retryErr != nil {
		t.Errorf("doWithRetry() error = %v, want nil", retryErr)
	}
	if resp == nil {
		t.Fatal("doWithRetry() response is nil")
		return
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("doWithRetry() status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	_ = resp.Body.Close()

	if got := atomic.LoadInt32(&attemptCount); got != 3 {
		t.Errorf("expected 3 attempts, got %d", got)
	}
}

// TestDoWithRetry_ConnectionRefused verifies that connection-refused errors are
// treated as retryable and that ErrMaxRetriesExceeded is returned once retries
// are exhausted.
func TestDoWithRetry_ConnectionRefused(t *testing.T) {
	// Start a server just to get a valid host:port, then close it immediately
	// so that subsequent connection attempts get "connection refused".
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	closedURL := srv.URL
	srv.Close()

	client := NewClient("test-api-key")
	client.maxRetries = 2
	client.baseDelay = 5 * time.Millisecond
	client.maxDelay = 10 * time.Millisecond
	client.jitterFraction = 0

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, closedURL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	start := time.Now()
	_, retryErr := client.doWithRetry(context.Background(), req)
	elapsed := time.Since(start)

	if retryErr == nil {
		t.Fatal("doWithRetry() expected error for connection refused, got nil")
	}
	if !errors.Is(retryErr, ErrMaxRetriesExceeded) {
		t.Errorf("doWithRetry() error = %v, want ErrMaxRetriesExceeded", retryErr)
	}

	// With maxRetries=2 and baseDelay=5ms the minimum elapsed time is
	// ~5ms (attempt 0->1) + ~10ms (attempt 1->2) = ~15ms.
	if elapsed < 10*time.Millisecond {
		t.Errorf("expected backoff delay for connection-refused retries, got %v", elapsed)
	}
}

// TestDoWithRetry_ConcurrentRequests verifies that concurrent doWithRetry calls
// from multiple goroutines do not interfere with each other and all succeed.
func TestDoWithRetry_ConcurrentRequests(t *testing.T) {
	var callCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	// Use a fresh client with a permissive rate limiter so rate limiting is not
	// the bottleneck in this test.
	client := NewClient("test-api-key-concurrent")
	client.baseDelay = time.Millisecond
	client.maxDelay = time.Millisecond

	const numGoroutines = 20
	var wg sync.WaitGroup
	errs := make([]error, numGoroutines)

	for i := range numGoroutines {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
			if err != nil {
				errs[idx] = fmt.Errorf("create request: %w", err)
				return
			}
			resp, err := client.doWithRetry(context.Background(), req)
			if err != nil {
				errs[idx] = err
				return
			}
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
		}(i)
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: unexpected error: %v", i, err)
		}
	}

	if got := atomic.LoadInt32(&callCount); got != numGoroutines {
		t.Errorf("expected %d calls, got %d", numGoroutines, got)
	}
}
