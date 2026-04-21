package controller

import (
	"fmt"
	"time"
)

func circuitBreakerBlockDetails(accountKey string, state CircuitState, cooldown time.Duration) (reason, eventReason, msg string) {
	switch state {
	case CircuitOpen:
		return ReasonCircuitBreakerOpen, "CircuitBreakerOpen",
			fmt.Sprintf("Circuit breaker open for account %s; reconciliation paused for %s", accountKey, cooldown)
	case CircuitHalfOpen:
		return ReasonCircuitBreakerBlocked, "CircuitBreakerBlocked",
			fmt.Sprintf("Circuit breaker half-open for account %s; probe request already in flight, reconciliation temporarily paused (retrying in %s)", accountKey, cooldown)
	default:
		return ReasonCircuitBreakerBlocked, "CircuitBreakerBlocked",
			fmt.Sprintf("Circuit breaker blocking API call for account %s in state %s; reconciliation temporarily paused (retrying in %s)", accountKey, state, cooldown)
	}
}
