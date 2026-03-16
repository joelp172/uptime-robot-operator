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
	"testing"
	"time"
)

const testAccountKey = "test-ns/test-account"

// newTestCircuitBreaker returns a CircuitBreaker with a short cooldown suitable for tests.
func newTestCircuitBreaker(threshold int, cooldown time.Duration) *CircuitBreaker {
	return NewCircuitBreaker(threshold, cooldown)
}

// ---- State transitions ----

func TestCircuitBreakerInitialState(t *testing.T) {
	cb := newTestCircuitBreaker(3, time.Minute)
	if got := cb.State(testAccountKey); got != CircuitClosed {
		t.Errorf("State() = %v, want CircuitClosed", got)
	}
	if !cb.Allow(testAccountKey) {
		t.Error("Allow() should return true when circuit is closed")
	}
}

func TestCircuitBreakerOpensAfterThreshold(t *testing.T) {
	const threshold = 3
	cb := newTestCircuitBreaker(threshold, time.Minute)

	// Record (threshold-1) failures — circuit must remain closed.
	for i := 0; i < threshold-1; i++ {
		cb.RecordFailure(testAccountKey)
		if cb.State(testAccountKey) != CircuitClosed {
			t.Errorf("After %d failures, circuit should still be closed", i+1)
		}
	}

	// One more failure should open the circuit.
	opened := cb.RecordFailure(testAccountKey)
	if !opened {
		t.Error("RecordFailure() should return true when circuit transitions to Open")
	}
	if got := cb.State(testAccountKey); got != CircuitOpen {
		t.Errorf("After %d failures, State() = %v, want CircuitOpen", threshold, got)
	}
	if cb.Allow(testAccountKey) {
		t.Error("Allow() should return false when circuit is open")
	}
	// Subsequent RecordFailure should not re-trigger the opened event.
	secondOpened := cb.RecordFailure(testAccountKey)
	if secondOpened {
		t.Error("RecordFailure() should return false when circuit is already Open")
	}
}

func TestCircuitBreakerClosesAfterSuccess(t *testing.T) {
	const threshold = 3
	cb := newTestCircuitBreaker(threshold, time.Minute)

	// Open the circuit.
	for i := 0; i < threshold; i++ {
		cb.RecordFailure(testAccountKey)
	}
	if cb.State(testAccountKey) != CircuitOpen {
		t.Fatal("Circuit should be open after threshold failures")
	}

	// Simulate the cooldown elapsing by reaching into internal state.
	cb.mu.Lock()
	cb.entries[testAccountKey].openedAt = time.Now().Add(-2 * time.Minute)
	cb.entries[testAccountKey].state = CircuitOpen
	cb.mu.Unlock()

	// After cooldown, State() should return HalfOpen, Allow() should be true.
	if got := cb.State(testAccountKey); got != CircuitHalfOpen {
		t.Errorf("After cooldown, State() = %v, want CircuitHalfOpen", got)
	}
	if !cb.Allow(testAccountKey) {
		t.Error("Allow() should return true in HalfOpen state (probe call)")
	}

	// On success, circuit should close.
	cb.RecordSuccess(testAccountKey)
	if got := cb.State(testAccountKey); got != CircuitClosed {
		t.Errorf("After RecordSuccess, State() = %v, want CircuitClosed", got)
	}
}

func TestCircuitBreakerReopenOnProbeFailure(t *testing.T) {
	const threshold = 3
	cb := newTestCircuitBreaker(threshold, time.Minute)

	// Open the circuit.
	for i := 0; i < threshold; i++ {
		cb.RecordFailure(testAccountKey)
	}

	// Expire the cooldown → HalfOpen.
	cb.mu.Lock()
	cb.entries[testAccountKey].openedAt = time.Now().Add(-2 * time.Minute)
	cb.entries[testAccountKey].state = CircuitOpen
	cb.mu.Unlock()

	if cb.State(testAccountKey) != CircuitHalfOpen {
		t.Fatal("Expected HalfOpen after cooldown")
	}

	// A failure while HalfOpen increments count and reopens.
	cb.RecordFailure(testAccountKey)
	if got := cb.State(testAccountKey); got != CircuitOpen {
		t.Errorf("After probe failure, State() = %v, want CircuitOpen", got)
	}
}

func TestCircuitBreakerSuccessResetsCounter(t *testing.T) {
	const threshold = 5
	cb := newTestCircuitBreaker(threshold, time.Minute)

	for i := 0; i < threshold-1; i++ {
		cb.RecordFailure(testAccountKey)
	}
	cb.RecordSuccess(testAccountKey)

	if got := cb.ConsecutiveFailures(testAccountKey); got != 0 {
		t.Errorf("ConsecutiveFailures after RecordSuccess = %d, want 0", got)
	}
	if got := cb.State(testAccountKey); got != CircuitClosed {
		t.Errorf("State after RecordSuccess = %v, want CircuitClosed", got)
	}
}

func TestCircuitBreakerPerAccountIsolation(t *testing.T) {
	const threshold = 2
	cb := newTestCircuitBreaker(threshold, time.Minute)
	key1 := "ns/account-a"
	key2 := "ns/account-b"

	// Open circuit for key1.
	for i := 0; i < threshold; i++ {
		cb.RecordFailure(key1)
	}

	if cb.State(key1) != CircuitOpen {
		t.Error("key1 circuit should be open")
	}
	if cb.State(key2) != CircuitClosed {
		t.Error("key2 circuit should be closed (independent of key1)")
	}
	if !cb.Allow(key2) {
		t.Error("Allow(key2) should return true when key2 circuit is closed")
	}
}

func TestCircuitBreakerConcurrentAccess(t *testing.T) {
	const threshold = 100
	cb := newTestCircuitBreaker(threshold, time.Minute)
	done := make(chan struct{})

	// Concurrent writers.
	for i := 0; i < 50; i++ {
		go func() {
			cb.RecordFailure(testAccountKey)
			cb.RecordSuccess(testAccountKey)
			done <- struct{}{}
		}()
	}

	// Concurrent readers.
	for i := 0; i < 50; i++ {
		go func() {
			_ = cb.State(testAccountKey)
			_ = cb.Allow(testAccountKey)
			_ = cb.ConsecutiveFailures(testAccountKey)
			done <- struct{}{}
		}()
	}

	for i := 0; i < 100; i++ {
		<-done
	}
}

// ---- AddSyncJitter ----

func TestAddSyncJitterRange(t *testing.T) {
	base := 24 * time.Hour
	min := time.Duration(float64(base) * (1 - SyncJitterFraction))
	max := time.Duration(float64(base) * (1 + SyncJitterFraction))

	for i := 0; i < 100; i++ {
		got := AddSyncJitter(base)
		if got < min || got > max {
			t.Errorf("AddSyncJitter(%v) = %v, want [%v, %v]", base, got, min, max)
		}
	}
}

func TestAddSyncJitterProducesVariedValues(t *testing.T) {
	base := time.Hour
	seen := make(map[time.Duration]bool)
	for i := 0; i < 100; i++ {
		seen[AddSyncJitter(base)] = true
	}
	if len(seen) < 50 {
		t.Errorf("Expected many unique jitter values, got only %d out of 100 runs", len(seen))
	}
}

func TestAddSyncJitterZeroDuration(t *testing.T) {
	if got := AddSyncJitter(0); got != 0 {
		t.Errorf("AddSyncJitter(0) = %v, want 0", got)
	}
}

func TestAddSyncJitterNegativeDuration(t *testing.T) {
	if got := AddSyncJitter(-time.Hour); got != -time.Hour {
		t.Errorf("AddSyncJitter(-1h) = %v, want -1h (passthrough)", got)
	}
}
