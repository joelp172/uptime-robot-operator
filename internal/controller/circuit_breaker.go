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
	"sync"
	"time"

	"github.com/joelp172/uptime-robot-operator/internal/metrics"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CircuitState represents the operational state of a circuit breaker.
type CircuitState int

const (
	// CircuitClosed is the normal operating state — API calls proceed as usual.
	// Explicit values are load-bearing: they map directly to the
	// uptimerobot_circuit_breaker_state Prometheus gauge.
	CircuitClosed CircuitState = 0
	// CircuitOpen means too many consecutive failures occurred; API calls are skipped.
	CircuitOpen CircuitState = 1
	// CircuitHalfOpen is the probe state — one call is allowed to test recovery.
	CircuitHalfOpen CircuitState = 2
)

// String returns a human-readable label for the circuit state.
func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

const (
	// DefaultFailureThreshold is the number of consecutive API failures that open the circuit.
	DefaultFailureThreshold = 5
	// DefaultCooldownPeriod is the time to wait after the circuit opens before probing.
	DefaultCooldownPeriod = 5 * time.Minute
)

// circuitEntry holds the per-account state for a circuit breaker.
type circuitEntry struct {
	state            CircuitState
	consecutiveFails int
	openedAt         time.Time
	// probeInFlight is set to true when a single probe call is allowed through
	// in HalfOpen state. It prevents multiple concurrent callers from all
	// slipping through and spiking traffic during recovery.
	probeInFlight bool
}

// CircuitBreaker implements per-account API protection.
// It tracks consecutive API failures keyed by account (namespace/name).
// After reaching failureThreshold consecutive failures the circuit opens and
// API calls are skipped. After cooldownPeriod the circuit transitions to
// HalfOpen so a single probe call can test whether the API has recovered.
// Thread-safe.
type CircuitBreaker struct {
	mu               sync.RWMutex
	entries          map[string]*circuitEntry
	failureThreshold int
	cooldownPeriod   time.Duration
}

// NewCircuitBreaker returns a CircuitBreaker with the given thresholds.
// Invalid inputs are clamped to defaults: failureThreshold must be >= 1,
// cooldownPeriod must be > 0.
func NewCircuitBreaker(failureThreshold int, cooldownPeriod time.Duration) *CircuitBreaker {
	if failureThreshold < 1 {
		failureThreshold = DefaultFailureThreshold
	}
	if cooldownPeriod <= 0 {
		cooldownPeriod = DefaultCooldownPeriod
	}
	return &CircuitBreaker{
		entries:          make(map[string]*circuitEntry),
		failureThreshold: failureThreshold,
		cooldownPeriod:   cooldownPeriod,
	}
}

// DefaultCircuitBreaker is the package-level circuit breaker shared by all controllers.
var DefaultCircuitBreaker = NewCircuitBreaker(DefaultFailureThreshold, DefaultCooldownPeriod)

// entry returns the circuitEntry for key, creating it (Closed) if absent.
// Must be called while holding the write lock (cb.mu.Lock).
func (cb *CircuitBreaker) entry(key string) *circuitEntry {
	if e, ok := cb.entries[key]; ok {
		return e
	}
	e := &circuitEntry{state: CircuitClosed}
	cb.entries[key] = e
	return e
}

// promoteIfCooldownElapsed transitions Open → HalfOpen if the cooldown has
// elapsed. Must be called while holding cb.mu (write lock).
func (cb *CircuitBreaker) promoteIfCooldownElapsed(e *circuitEntry, accountKey string) {
	if e.state == CircuitOpen && time.Since(e.openedAt) >= cb.cooldownPeriod {
		e.state = CircuitHalfOpen
		e.probeInFlight = false
		metrics.CircuitBreakerState.WithLabelValues(accountKey).Set(float64(CircuitHalfOpen))
	}
}

// State returns the current CircuitState for accountKey.
// NOTE: this may transition Open → HalfOpen after the cooldown elapses.
// Allow() performs the same promotion.
func (cb *CircuitBreaker) State(accountKey string) CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	e := cb.entry(accountKey)
	cb.promoteIfCooldownElapsed(e, accountKey)
	return e.state
}

// Allow returns true when a reconciliation should proceed with an API call.
// Returns false when the circuit is not allowing traffic (Open, or HalfOpen
// with a probe already in flight).
// In HalfOpen exactly one probe call is allowed through; all subsequent callers
// are blocked until RecordSuccess or RecordFailure is called.
func (cb *CircuitBreaker) Allow(accountKey string) bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	e := cb.entry(accountKey)
	cb.promoteIfCooldownElapsed(e, accountKey)

	switch e.state {
	case CircuitClosed:
		return true
	case CircuitHalfOpen:
		// Claim the probe slot atomically: only one caller gets through.
		if !e.probeInFlight {
			e.probeInFlight = true
			return true
		}
		return false
	default: // CircuitOpen
		return false
	}
}

// RecordSuccess closes the circuit and resets the failure counter.
func (cb *CircuitBreaker) RecordSuccess(accountKey string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	e := cb.entry(accountKey)
	e.consecutiveFails = 0
	e.state = CircuitClosed
	e.probeInFlight = false
	metrics.CircuitBreakerState.WithLabelValues(accountKey).Set(float64(CircuitClosed))
}

// RecordFailure increments the consecutive failure counter.
// If the circuit is HalfOpen (probe failed), it immediately reopens.
// If the counter reaches failureThreshold from Closed, the circuit opens.
// Returns true if the circuit transitioned to Open on this call.
func (cb *CircuitBreaker) RecordFailure(accountKey string) bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	e := cb.entry(accountKey)
	e.consecutiveFails++

	// A probe failure in HalfOpen always reopens the circuit immediately,
	// regardless of the failure count. This ensures the cooldown timer
	// resets and we don't re-probe right away.
	if e.state == CircuitHalfOpen {
		e.state = CircuitOpen
		e.openedAt = time.Now()
		e.probeInFlight = false
		metrics.CircuitBreakerState.WithLabelValues(accountKey).Set(float64(CircuitOpen))
		return true
	}

	if e.consecutiveFails >= cb.failureThreshold && e.state != CircuitOpen {
		e.state = CircuitOpen
		e.openedAt = time.Now()
		e.probeInFlight = false
		metrics.CircuitBreakerState.WithLabelValues(accountKey).Set(float64(CircuitOpen))
		return true
	}
	return false
}

// Remove deletes the circuit entry for accountKey, freeing memory for
// accounts that no longer exist.
func (cb *CircuitBreaker) Remove(accountKey string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	delete(cb.entries, accountKey)
	metrics.CircuitBreakerState.DeleteLabelValues(accountKey)
}

// CooldownPeriod returns the configured cooldown duration.
// cooldownPeriod is immutable after construction so no lock is needed.
func (cb *CircuitBreaker) CooldownPeriod() time.Duration {
	return cb.cooldownPeriod
}

// ConsecutiveFailures returns the current consecutive failure count for an account key.
func (cb *CircuitBreaker) ConsecutiveFailures(accountKey string) int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	e, ok := cb.entries[accountKey]
	if !ok {
		return 0
	}
	return e.consecutiveFails
}

// onTransientAPIFailure records a transient API failure for the account's circuit breaker
// using DefaultCircuitBreaker, and emits a CircuitBreakerOpened Kubernetes event if the
// circuit just transitioned to Open. It is a no-op when err is not a transient error.
// Controllers that make UptimeRobot API calls share this helper to avoid duplicating the
// IsTransientError → RecordFailure → event-emission pattern.
func onTransientAPIFailure(accountKey string, err error, recorder record.EventRecorder, obj client.Object) {
	if !IsTransientError(err) {
		return
	}
	if justOpened := DefaultCircuitBreaker.RecordFailure(accountKey); justOpened && recorder != nil {
		recorder.Event(obj, "Warning", "CircuitBreakerOpened",
			"UptimeRobot API circuit breaker opened after repeated failures")
	}
}
