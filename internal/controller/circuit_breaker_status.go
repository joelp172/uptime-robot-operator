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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func circuitBreakerBlockDetails(accountKey string, state CircuitState, cooldown time.Duration) (reason, eventReason, msg string) {
	switch state {
	case CircuitOpen:
		return ReasonCircuitBreakerOpen, ReasonCircuitBreakerOpen,
			fmt.Sprintf("Circuit breaker open for account %s; reconciliation paused for %s", accountKey, cooldown)
	case CircuitHalfOpen:
		return ReasonCircuitBreakerBlocked, ReasonCircuitBreakerBlocked,
			fmt.Sprintf("Circuit breaker half-open for account %s; probe request already in flight, reconciliation temporarily paused (retrying in %s)", accountKey, cooldown)
	default:
		return ReasonCircuitBreakerBlocked, ReasonCircuitBreakerBlocked,
			fmt.Sprintf("Circuit breaker blocking API call for account %s in state %s; reconciliation temporarily paused (retrying in %s)", accountKey, state, cooldown)
	}
}

// applyCircuitBreakerBlockedConditions ensures Ready/Synced/Error reflect a
// blocked circuit-breaker state for the given object generation. Returns the
// event reason, the message, and changed=true only on transition into the
// blocked state (including when the spec generation advances while blocked),
// so callers emit the warning event and persist status exactly once per
// transition rather than on every requeue tick.
func applyCircuitBreakerBlockedConditions(conditions *[]metav1.Condition, generation int64, accountKey string, state CircuitState, cooldown time.Duration) (eventReason, msg string, changed bool) {
	reason, eventReason, msg := circuitBreakerBlockDetails(accountKey, state, cooldown)
	alreadyBlocked := blockedConditionMatches(*conditions, TypeSynced, metav1.ConditionFalse, reason, msg, generation) &&
		blockedConditionMatches(*conditions, TypeReady, metav1.ConditionFalse, reason, msg, generation) &&
		blockedConditionMatches(*conditions, TypeError, metav1.ConditionTrue, reason, msg, generation)
	if alreadyBlocked {
		return eventReason, msg, false
	}
	SetReadyCondition(conditions, false, reason, msg, generation)
	SetSyncedCondition(conditions, false, reason, msg, generation)
	SetErrorCondition(conditions, true, reason, msg, generation)
	return eventReason, msg, true
}

// blockedConditionMatches is like conditionMatches but also requires the
// stored ObservedGeneration to match. This ensures a spec edit during an open
// breaker is treated as a new transition so ObservedGeneration gets refreshed.
func blockedConditionMatches(conditions []metav1.Condition, conditionType string, status metav1.ConditionStatus, reason, message string, generation int64) bool {
	for _, c := range conditions {
		if c.Type == conditionType {
			return c.Status == status && c.Reason == reason && c.Message == message && c.ObservedGeneration == generation
		}
	}
	return false
}
