package executor

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func captureBreakerStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

func TestCircuitBreaker_WhenBelowThreshold_ItShouldAllow(t *testing.T) {
	cb := newCircuitBreaker("test-cluster")

	cb.recordFailure("op1")
	cb.recordFailure("op2")

	if err := cb.allow("op3"); err != nil {
		t.Fatalf("expected allow with 2 failures (threshold is 3), got: %v", err)
	}
}

func TestCircuitBreaker_WhenThresholdReached_ItShouldOpen(t *testing.T) {
	cb := newCircuitBreaker("test-cluster")

	cb.recordFailure("op1")
	cb.recordFailure("op2")
	line := captureBreakerStdout(t, func() {
		cb.recordFailure("op3")
	})

	if err := cb.allow("op4"); err == nil {
		t.Fatal("expected circuit to be open after 3 failures")
	}
	if !strings.Contains(line, "CircuitBreakerStateChange") || !strings.Contains(line, "open") {
		t.Fatalf("expected open state-change EMF, got: %s", line)
	}
}

func TestCircuitBreaker_WhenSuccessAfterFailures_ItShouldReset(t *testing.T) {
	cb := newCircuitBreaker("test-cluster")

	cb.recordFailure("op1")
	cb.recordFailure("op2")
	cb.recordSuccess()

	if err := cb.allow("op3"); err != nil {
		t.Fatalf("expected allow after success reset, got: %v", err)
	}

	cb.recordFailure("op3")
	cb.recordFailure("op4")

	if err := cb.allow("op5"); err != nil {
		t.Fatalf("expected allow with only 2 failures after reset, got: %v", err)
	}
}

func TestCircuitBreaker_WhenOpenDurationExpires_ItShouldHalfOpen(t *testing.T) {
	cb := newCircuitBreaker("test-cluster")

	cb.recordFailure("op1")
	cb.recordFailure("op2")
	cb.recordFailure("op3")

	cb.mu.Lock()
	cb.openedAt = time.Now().Add(-circuitBreakerOpenFor - time.Second)
	cb.mu.Unlock()

	if err := cb.allow("op4"); err != nil {
		t.Fatalf("expected half-open after open duration expired, got: %v", err)
	}

	cb.mu.Lock()
	if cb.state != circuitHalfOpen {
		t.Fatalf("expected state=halfOpen, got %d", cb.state)
	}
	cb.mu.Unlock()
}

func TestCircuitBreaker_WhenHalfOpenSucceeds_ItShouldClose(t *testing.T) {
	cb := newCircuitBreaker("test-cluster")

	cb.recordFailure("op1")
	cb.recordFailure("op2")
	cb.recordFailure("op3")

	cb.mu.Lock()
	cb.openedAt = time.Now().Add(-circuitBreakerOpenFor - time.Second)
	cb.mu.Unlock()

	_ = cb.allow("op4")
	cb.recordSuccess()

	cb.mu.Lock()
	if cb.state != circuitClosed {
		t.Fatalf("expected state=closed after half-open success, got %d", cb.state)
	}
	if len(cb.failures) != 0 {
		t.Fatalf("expected failures cleared, got %d", len(cb.failures))
	}
	cb.mu.Unlock()
}

func TestCircuitBreaker_WhenFailuresOutsideWindow_ItShouldNotOpen(t *testing.T) {
	cb := newCircuitBreaker("test-cluster")

	cb.mu.Lock()
	cb.failures = append(cb.failures,
		time.Now().Add(-circuitBreakerWindow-time.Second),
		time.Now().Add(-circuitBreakerWindow-time.Second),
	)
	cb.mu.Unlock()

	cb.recordFailure("op3")

	if err := cb.allow("op4"); err != nil {
		t.Fatalf("expected allow: old failures outside window should be evicted, got: %v", err)
	}
}
