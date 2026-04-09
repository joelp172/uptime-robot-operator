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
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
)

const testAccountKey = "test-ns/test-account"

// ---- State transitions ----

func TestCircuitBreakerInitialState(t *testing.T) {
	cb := NewCircuitBreaker(3, time.Minute)
	if got := cb.State(testAccountKey); got != CircuitClosed {
		t.Errorf("State() = %v, want CircuitClosed", got)
	}
	if !cb.Allow(testAccountKey) {
		t.Error("Allow() should return true when circuit is closed")
	}
}

func TestCircuitBreakerOpensAfterThreshold(t *testing.T) {
	const threshold = 3
	cb := NewCircuitBreaker(threshold, time.Minute)

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
	cb := NewCircuitBreaker(threshold, time.Minute)

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

	// After cooldown, State() should return HalfOpen.
	if got := cb.State(testAccountKey); got != CircuitHalfOpen {
		t.Errorf("After cooldown, State() = %v, want CircuitHalfOpen", got)
	}
	// First Allow() claims the probe slot.
	if !cb.Allow(testAccountKey) {
		t.Error("First Allow() in HalfOpen should return true (probe call)")
	}
	// Second Allow() should be blocked — probe already in flight.
	if cb.Allow(testAccountKey) {
		t.Error("Second Allow() in HalfOpen should return false (probe already in flight)")
	}

	// On success, circuit should close and Allow() works again.
	cb.RecordSuccess(testAccountKey)
	if got := cb.State(testAccountKey); got != CircuitClosed {
		t.Errorf("After RecordSuccess, State() = %v, want CircuitClosed", got)
	}
	if !cb.Allow(testAccountKey) {
		t.Error("Allow() should return true after circuit closes")
	}
}

func TestCircuitBreakerHalfOpenSingleProbe(t *testing.T) {
	const threshold = 2
	cb := NewCircuitBreaker(threshold, time.Minute)

	// Open the circuit.
	for i := 0; i < threshold; i++ {
		cb.RecordFailure(testAccountKey)
	}

	// Expire the cooldown so Allow() can promote to HalfOpen.
	cb.mu.Lock()
	cb.entries[testAccountKey].openedAt = time.Now().Add(-2 * time.Minute)
	cb.entries[testAccountKey].state = CircuitOpen
	cb.mu.Unlock()

	// First Allow() promotes to HalfOpen and claims the probe slot.
	if !cb.Allow(testAccountKey) {
		t.Fatal("First Allow() should return true (probe call)")
	}
	// Remaining concurrent callers must be blocked.
	for i := 0; i < 5; i++ {
		if cb.Allow(testAccountKey) {
			t.Errorf("Allow() call %d should return false while probe is in flight", i+2)
		}
	}
	// After RecordSuccess, circuit closes and all callers may proceed.
	cb.RecordSuccess(testAccountKey)
	for i := 0; i < 3; i++ {
		if !cb.Allow(testAccountKey) {
			t.Errorf("Allow() call %d after close should return true", i+1)
		}
	}
}

func TestCircuitBreakerReopenOnProbeFailure(t *testing.T) {
	const threshold = 3
	cb := NewCircuitBreaker(threshold, time.Minute)

	// Open the circuit.
	for i := 0; i < threshold; i++ {
		cb.RecordFailure(testAccountKey)
	}

	// Expire the cooldown → Allow() promotes to HalfOpen and claims probe.
	cb.mu.Lock()
	cb.entries[testAccountKey].openedAt = time.Now().Add(-2 * time.Minute)
	cb.entries[testAccountKey].state = CircuitOpen
	cb.mu.Unlock()

	if !cb.Allow(testAccountKey) {
		t.Fatal("Expected Allow() to return true (probe) after cooldown")
	}

	// A failure on the probe increments count and reopens the circuit.
	cb.RecordFailure(testAccountKey)
	if got := cb.State(testAccountKey); got != CircuitOpen {
		t.Errorf("After probe failure, State() = %v, want CircuitOpen", got)
	}
	if cb.Allow(testAccountKey) {
		t.Error("Allow() should return false after circuit reopens")
	}
}

func TestCircuitBreakerSuccessResetsCounter(t *testing.T) {
	const threshold = 5
	cb := NewCircuitBreaker(threshold, time.Minute)

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
	cb := NewCircuitBreaker(threshold, time.Minute)
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
	cb := NewCircuitBreaker(threshold, time.Minute)
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

// ---- openedAt refresh on probe failure ----

func TestCircuitBreakerProbeFailureRefreshesOpenedAt(t *testing.T) {
	const threshold = 2
	cb := NewCircuitBreaker(threshold, time.Minute)

	// Open the circuit.
	for i := 0; i < threshold; i++ {
		cb.RecordFailure(testAccountKey)
	}

	// Expire the cooldown so Allow() promotes to HalfOpen.
	cb.mu.Lock()
	cb.entries[testAccountKey].openedAt = time.Now().Add(-2 * time.Minute)
	cb.mu.Unlock()

	// Claim probe slot.
	if !cb.Allow(testAccountKey) {
		t.Fatal("Expected Allow() to return true (probe) after cooldown")
	}

	before := time.Now()
	cb.RecordFailure(testAccountKey) // probe fails → reopen
	after := time.Now()

	// Verify circuit is open again.
	cb.mu.RLock()
	e := cb.entries[testAccountKey]
	openedAt := e.openedAt
	state := e.state
	cb.mu.RUnlock()

	if state != CircuitOpen {
		t.Errorf("State after probe failure = %v, want CircuitOpen", state)
	}
	if openedAt.Before(before) || openedAt.After(after) {
		t.Errorf("openedAt = %v, want between %v and %v (should be refreshed)", openedAt, before, after)
	}

	// Because openedAt was refreshed, State() should still return Open (not HalfOpen).
	if got := cb.State(testAccountKey); got != CircuitOpen {
		t.Errorf("State() immediately after probe failure = %v, want CircuitOpen (cooldown not elapsed)", got)
	}
}

// ---- HalfOpen probe failure after counter reset ----

func TestCircuitBreakerHalfOpenProbeFailureAfterCounterReset(t *testing.T) {
	const threshold = 3
	cb := NewCircuitBreaker(threshold, time.Minute)

	// Open the circuit.
	for i := 0; i < threshold; i++ {
		cb.RecordFailure(testAccountKey)
	}

	// Expire cooldown, promote to HalfOpen via Allow, probe succeeds → circuit closes.
	cb.mu.Lock()
	cb.entries[testAccountKey].openedAt = time.Now().Add(-2 * time.Minute)
	cb.mu.Unlock()
	cb.Allow(testAccountKey)
	cb.RecordSuccess(testAccountKey)

	// Now counter is 0. Open circuit again.
	for i := 0; i < threshold; i++ {
		cb.RecordFailure(testAccountKey)
	}

	// Expire cooldown and claim probe.
	cb.mu.Lock()
	cb.entries[testAccountKey].openedAt = time.Now().Add(-2 * time.Minute)
	cb.mu.Unlock()
	if !cb.Allow(testAccountKey) {
		t.Fatal("Expected Allow() to return true (probe)")
	}

	// Reset the counter to simulate a scenario where RecordSuccess was called
	// by another reconciliation between open and probe.
	cb.mu.Lock()
	cb.entries[testAccountKey].consecutiveFails = 0
	cb.mu.Unlock()

	// Probe failure should still immediately reopen the circuit (HalfOpen → Open).
	opened := cb.RecordFailure(testAccountKey)
	if !opened {
		t.Error("RecordFailure() in HalfOpen should return true (circuit reopened) even with low counter")
	}
	if got := cb.State(testAccountKey); got != CircuitOpen {
		t.Errorf("After HalfOpen probe failure with reset counter, State() = %v, want CircuitOpen", got)
	}
}

// ---- CircuitState String() ----

func TestCircuitStateString(t *testing.T) {
	tests := []struct {
		state CircuitState
		want  string
	}{
		{CircuitClosed, "closed"},
		{CircuitOpen, "open"},
		{CircuitHalfOpen, "half-open"},
		{CircuitState(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("CircuitState(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}

// ---- Constructor validation ----

func TestNewCircuitBreakerValidation(t *testing.T) {
	// Zero threshold should be clamped to default.
	cb := NewCircuitBreaker(0, time.Minute)
	if cb.failureThreshold != DefaultFailureThreshold {
		t.Errorf("failureThreshold = %d, want %d (default)", cb.failureThreshold, DefaultFailureThreshold)
	}

	// Negative threshold should be clamped to default.
	cb = NewCircuitBreaker(-1, time.Minute)
	if cb.failureThreshold != DefaultFailureThreshold {
		t.Errorf("failureThreshold = %d, want %d (default)", cb.failureThreshold, DefaultFailureThreshold)
	}

	// Zero cooldown should be clamped to default.
	cb = NewCircuitBreaker(5, 0)
	if cb.cooldownPeriod != DefaultCooldownPeriod {
		t.Errorf("cooldownPeriod = %v, want %v (default)", cb.cooldownPeriod, DefaultCooldownPeriod)
	}

	// Negative cooldown should be clamped to default.
	cb = NewCircuitBreaker(5, -time.Second)
	if cb.cooldownPeriod != DefaultCooldownPeriod {
		t.Errorf("cooldownPeriod = %v, want %v (default)", cb.cooldownPeriod, DefaultCooldownPeriod)
	}

	// Valid inputs should be preserved.
	cb = NewCircuitBreaker(10, 2*time.Minute)
	if cb.failureThreshold != 10 || cb.cooldownPeriod != 2*time.Minute {
		t.Errorf("Valid inputs not preserved: threshold=%d, cooldown=%v", cb.failureThreshold, cb.cooldownPeriod)
	}
}

// ---- Remove ----

func TestCircuitBreakerRemove(t *testing.T) {
	cb := NewCircuitBreaker(3, time.Minute)
	cb.RecordFailure(testAccountKey)

	if cb.ConsecutiveFailures(testAccountKey) != 1 {
		t.Fatal("Expected 1 failure after RecordFailure")
	}

	cb.Remove(testAccountKey)

	if cb.ConsecutiveFailures(testAccountKey) != 0 {
		t.Error("After Remove, ConsecutiveFailures should return 0")
	}
	if cb.State(testAccountKey) != CircuitClosed {
		t.Error("After Remove, State should return CircuitClosed")
	}
}

// ---- onTransientAPIFailure ----

func testObj() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}
}

func TestOnTransientAPIFailure_NonTransientError(t *testing.T) {
	// Save and restore the package-level DefaultCircuitBreaker.
	orig := DefaultCircuitBreaker
	defer func() { DefaultCircuitBreaker = orig }()
	DefaultCircuitBreaker = NewCircuitBreaker(3, time.Minute)

	recorder := record.NewFakeRecorder(10)
	// A non-transient error (400 Bad Request) should be a no-op.
	onTransientAPIFailure(testAccountKey, fmt.Errorf("bad request"), recorder, testObj())

	if DefaultCircuitBreaker.ConsecutiveFailures(testAccountKey) != 0 {
		t.Error("Non-transient error should not increment failure counter")
	}
	select {
	case <-recorder.Events:
		t.Error("Non-transient error should not emit an event")
	default:
		// expected
	}
}

func TestOnTransientAPIFailure_TransientBelowThreshold(t *testing.T) {
	orig := DefaultCircuitBreaker
	defer func() { DefaultCircuitBreaker = orig }()
	DefaultCircuitBreaker = NewCircuitBreaker(5, time.Minute)

	recorder := record.NewFakeRecorder(10)
	// A transient error (503) that doesn't trip the threshold should record failure but no event.
	onTransientAPIFailure(testAccountKey, fmt.Errorf("status code: 503"), recorder, testObj())

	if DefaultCircuitBreaker.ConsecutiveFailures(testAccountKey) != 1 {
		t.Errorf("ConsecutiveFailures = %d, want 1", DefaultCircuitBreaker.ConsecutiveFailures(testAccountKey))
	}
	select {
	case <-recorder.Events:
		t.Error("Below-threshold transient error should not emit a CircuitBreakerOpened event")
	default:
		// expected
	}
}

func TestOnTransientAPIFailure_TransientTripsThreshold(t *testing.T) {
	orig := DefaultCircuitBreaker
	defer func() { DefaultCircuitBreaker = orig }()
	DefaultCircuitBreaker = NewCircuitBreaker(2, time.Minute)

	recorder := record.NewFakeRecorder(10)
	obj := testObj()

	// First transient failure: below threshold.
	onTransientAPIFailure(testAccountKey, fmt.Errorf("status code: 503"), recorder, obj)
	select {
	case <-recorder.Events:
		t.Error("First failure should not emit event")
	default:
	}

	// Second transient failure: trips threshold → circuit opens → event emitted.
	onTransientAPIFailure(testAccountKey, fmt.Errorf("status code: 503"), recorder, obj)
	select {
	case evt := <-recorder.Events:
		if evt == "" {
			t.Error("Expected CircuitBreakerOpened event")
		}
	default:
		t.Error("Expected CircuitBreakerOpened event when circuit opens")
	}

	if DefaultCircuitBreaker.State(testAccountKey) != CircuitOpen {
		t.Error("Circuit should be open after threshold failures")
	}
}

func TestOnTransientAPIFailure_NilRecorder(t *testing.T) {
	orig := DefaultCircuitBreaker
	defer func() { DefaultCircuitBreaker = orig }()
	DefaultCircuitBreaker = NewCircuitBreaker(1, time.Minute)

	// Should not panic with nil recorder.
	onTransientAPIFailure(testAccountKey, fmt.Errorf("status code: 503"), nil, testObj())

	if DefaultCircuitBreaker.State(testAccountKey) != CircuitOpen {
		t.Error("Circuit should be open even with nil recorder")
	}
}
