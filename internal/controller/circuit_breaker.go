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
)

// CircuitState represents the operational state of a circuit breaker.
type CircuitState int

const (
	// CircuitClosed is the normal operating state — API calls proceed as usual.
	CircuitClosed CircuitState = iota
	// CircuitOpen means too many consecutive failures occurred; API calls are skipped.
	CircuitOpen
	// CircuitHalfOpen is the probe state — one call is allowed to test recovery.
	CircuitHalfOpen

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
func NewCircuitBreaker(failureThreshold int, cooldownPeriod time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		entries:          make(map[string]*circuitEntry),
		failureThreshold: failureThreshold,
		cooldownPeriod:   cooldownPeriod,
	}
}

// DefaultCircuitBreaker is the package-level circuit breaker shared by all controllers.
var DefaultCircuitBreaker = NewCircuitBreaker(DefaultFailureThreshold, DefaultCooldownPeriod)

// entry returns the circuitEntry for key, creating it (Closed) if absent.
// Must be called with at least a read lock; upgrades are handled by the caller.
func (cb *CircuitBreaker) entry(key string) *circuitEntry {
	if e, ok := cb.entries[key]; ok {
		return e
	}
	e := &circuitEntry{state: CircuitClosed}
	cb.entries[key] = e
	return e
}

// State returns the current CircuitState for accountKey.
func (cb *CircuitBreaker) State(accountKey string) CircuitState {
	cb.mu.RLock()
	e, ok := cb.entries[accountKey]
	if !ok {
		cb.mu.RUnlock()
		return CircuitClosed
	}
	state := e.state
	openedAt := e.openedAt
	cb.mu.RUnlock()

	// Promote Open → HalfOpen once the cooldown has elapsed.
	if state == CircuitOpen && time.Since(openedAt) >= cb.cooldownPeriod {
		cb.mu.Lock()
		// Re-check inside write lock.
		if e.state == CircuitOpen && time.Since(e.openedAt) >= cb.cooldownPeriod {
			e.state = CircuitHalfOpen
			metrics.CircuitBreakerState.WithLabelValues(accountKey).Set(float64(CircuitHalfOpen))
		}
		state = e.state
		cb.mu.Unlock()
	}

	return state
}

// Allow returns true when a reconciliation should proceed with an API call.
// Returns false when the circuit is Open (blocking calls to protect the API).
// HalfOpen allows exactly one probe call through.
func (cb *CircuitBreaker) Allow(accountKey string) bool {
	state := cb.State(accountKey)
	return state == CircuitClosed || state == CircuitHalfOpen
}

// RecordSuccess closes the circuit and resets the failure counter.
func (cb *CircuitBreaker) RecordSuccess(accountKey string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	e := cb.entry(accountKey)
	e.consecutiveFails = 0
	e.state = CircuitClosed
	metrics.CircuitBreakerState.WithLabelValues(accountKey).Set(float64(CircuitClosed))
}

// RecordFailure increments the consecutive failure counter.
// If the counter reaches failureThreshold the circuit opens.
// Returns true if the circuit transitioned to Open on this call.
func (cb *CircuitBreaker) RecordFailure(accountKey string) bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	e := cb.entry(accountKey)
	e.consecutiveFails++
	if e.consecutiveFails >= cb.failureThreshold && e.state != CircuitOpen {
		e.state = CircuitOpen
		e.openedAt = time.Now()
		metrics.CircuitBreakerState.WithLabelValues(accountKey).Set(float64(CircuitOpen))
		return true
	}
	return false
}

// CooldownPeriod returns the configured cooldown duration.
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
