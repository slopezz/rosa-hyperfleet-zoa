package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
	"github.com/openshift-online/rosa-seaplane-zoa/internal/output"
)

type breakglassPollConfig struct {
	interval time.Duration
	timeout  time.Duration
}

type breakglassPollResult struct {
	req      *client.BreakglassRequest
	timedOut bool
	elapsed  time.Duration
}

func pollBreakglassRequestWithProgress(ctx context.Context, c APIClient, requestID string, cfg breakglassPollConfig) (*breakglassPollResult, error) {
	start := time.Now()
	for {
		req, err := c.GetBreakglassRequest(ctx, requestID)
		if err != nil {
			clearBreakglassPollLine()
			return nil, fmt.Errorf("polling break-glass request: %w", err)
		}
		if isTerminalBreakglassStatus(req.Status) {
			clearBreakglassPollLine()
			return &breakglassPollResult{req: req, elapsed: time.Since(start)}, nil
		}
		elapsed := time.Since(start)
		if elapsed >= cfg.timeout {
			clearBreakglassPollLine()
			return &breakglassPollResult{req: req, timedOut: true, elapsed: elapsed}, nil
		}
		if output.IsTerminal() {
			fmt.Fprintf(os.Stderr, "\r\033[K⠋ %s (%s)", req.Status, elapsed.Round(time.Second))
		}
		select {
		case <-ctx.Done():
			clearBreakglassPollLine()
			return nil, ctx.Err()
		case <-time.After(cfg.interval):
		}
	}
}

func clearBreakglassPollLine() {
	if output.IsTerminal() {
		fmt.Fprintf(os.Stderr, "\r\033[K")
	}
}

func printBreakglassWaitOutcome(req *client.BreakglassRequest, timedOut bool, elapsed time.Duration) {
	if req == nil {
		return
	}
	switch req.Status {
	case "approved":
		fmt.Fprintf(breakglassProgressWriter(), "✓ %s (%s)\n", req.Status, elapsed.Round(time.Second))
	case "rejected", "expired":
		fmt.Fprintf(breakglassProgressWriter(), "✗ %s (%s)\n", req.Status, elapsed.Round(time.Second))
	case "pending_approval":
		if timedOut {
			fmt.Fprintf(breakglassProgressWriter(), "✗ still %s after %s\n", req.Status, elapsed.Round(time.Second))
		}
	}
}
