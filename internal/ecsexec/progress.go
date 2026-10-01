package ecsexec

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/output"
)

func runWithSpinner(ctx context.Context, label string, fn func(context.Context) error) error {
	if !output.IsTerminal() {
		return fn(ctx)
	}

	done := make(chan error, 1)
	go func() {
		done <- fn(ctx)
	}()

	start := time.Now()
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case err := <-done:
			fmt.Fprintf(os.Stderr, "\r\033[K")
			return err
		case <-ctx.Done():
			fmt.Fprintf(os.Stderr, "\r\033[K")
			return ctx.Err()
		case <-tick.C:
			fmt.Fprintf(os.Stderr, "\r\033[K⠋ %s (%s)", label, time.Since(start).Round(time.Second))
		}
	}
}
