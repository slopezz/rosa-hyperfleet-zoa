package executor

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/metrics"
)

const (
	maxRetries     = 5
	baseRetryDelay = 500 // milliseconds
)

func (e *Executor) withRetry(ctx context.Context, operation string, fn func() error) error {
	if e.eksCircuit != nil {
		if err := e.eksCircuit.allow(operation); err != nil {
			if e.eksCircuit.cluster != "" {
				metrics.EmitRejection(e.eksCircuit.cluster, metrics.RejectionCircuitBreakerOpen)
			}
			return err
		}
	}

	var lastErr error
	for attempt := range maxRetries {
		lastErr = fn()
		if lastErr == nil {
			if e.eksCircuit != nil {
				e.eksCircuit.recordSuccess()
			}
			return nil
		}
		if errors.IsAlreadyExists(lastErr) {
			if e.eksCircuit != nil {
				e.eksCircuit.recordSuccess()
			}
			return nil
		}
		if !isTransientError(lastErr) {
			return lastErr
		}
		delay := time.Duration(baseRetryDelay*(1<<attempt)) * time.Millisecond
		e.logger.Warn("transient error, retrying",
			"operation", operation,
			"attempt", attempt+1,
			"delay", delay,
			"error", lastErr,
		)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	if e.eksCircuit != nil {
		e.eksCircuit.recordFailure(operation)
	}
	return fmt.Errorf("operation %q failed after %d retries: %w", operation, maxRetries, lastErr)
}

func isTransientError(err error) bool {
	if errors.IsServerTimeout(err) || errors.IsTimeout(err) {
		return true
	}
	if errors.IsTooManyRequests(err) {
		return true
	}
	if errors.IsServiceUnavailable(err) {
		return true
	}
	if errors.IsInternalError(err) {
		return true
	}
	if errors.ReasonForError(err) == metav1.StatusReasonUnknown {
		return true
	}
	return false
}
