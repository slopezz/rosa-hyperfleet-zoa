package executor

import (
	"fmt"
	"sync"
	"time"

	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/metrics"
)

const (
	circuitBreakerThreshold = 3
	circuitBreakerWindow    = 30 * time.Second
	circuitBreakerOpenFor   = 60 * time.Second
)

type circuitState int

const (
	circuitClosed circuitState = iota
	circuitOpen
	circuitHalfOpen
)

type circuitBreaker struct {
	mu            sync.Mutex
	cluster       string
	state         circuitState
	failures      []time.Time
	openedAt      time.Time
	lastOperation string
}

func newCircuitBreaker(cluster string) *circuitBreaker {
	return &circuitBreaker{
		cluster:  cluster,
		state:    circuitClosed,
		failures: make([]time.Time, 0, circuitBreakerThreshold),
	}
}

func (cb *circuitBreaker) allow(operation string) error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case circuitOpen:
		if time.Since(cb.openedAt) >= circuitBreakerOpenFor {
			cb.setStateLocked(circuitHalfOpen)
			return nil
		}
		return fmt.Errorf("circuit breaker open: EKS API unavailable (%d consecutive failures in %s, last operation: %s); fast-failing to avoid timeout exhaustion",
			circuitBreakerThreshold, circuitBreakerWindow, cb.lastOperation)
	case circuitHalfOpen:
		return nil
	default:
		return nil
	}
}

func (cb *circuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state != circuitClosed {
		cb.setStateLocked(circuitClosed)
	}
	cb.failures = cb.failures[:0]
}

func (cb *circuitBreaker) recordFailure(operation string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()
	cb.lastOperation = operation

	cutoff := now.Add(-circuitBreakerWindow)
	fresh := cb.failures[:0]
	for _, t := range cb.failures {
		if t.After(cutoff) {
			fresh = append(fresh, t)
		}
	}
	fresh = append(fresh, now)
	cb.failures = fresh

	if len(cb.failures) >= circuitBreakerThreshold && cb.state != circuitOpen {
		cb.openedAt = now
		cb.setStateLocked(circuitOpen)
	}
}

func (cb *circuitBreaker) setState(state circuitState) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.setStateLocked(state)
}

func (cb *circuitBreaker) setStateLocked(state circuitState) {
	if cb.state == state {
		return
	}
	cb.state = state
	if cb.cluster == "" {
		return
	}
	switch state {
	case circuitOpen:
		metrics.EmitCircuitBreakerStateChange(cb.cluster, "open")
	case circuitHalfOpen:
		metrics.EmitCircuitBreakerStateChange(cb.cluster, "half_open")
	case circuitClosed:
		metrics.EmitCircuitBreakerStateChange(cb.cluster, "closed")
	}
}
