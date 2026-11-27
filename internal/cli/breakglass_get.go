package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
	"github.com/openshift-online/rosa-seaplane-zoa/internal/output"
)

func newBreakglassGetCommand(global *GlobalOptions) *cobra.Command {
	var wait bool
	var waitTimeout, pollInterval time.Duration
	cmd := &cobra.Command{
		Use:   "get <request-id>",
		Short: "Get a break-glass request",
		Long:  `Get request status (stdout metadata, like zoa get). With --wait, poll until lifecycle active, denied, or expired. Does not activate — use zoa breakglass activate after approval.`,
		Example: `  zoa breakglass get <request-id>
  zoa breakglass get <request-id> --wait
  zoa breakglass get <request-id> --wait --wait-timeout 10m`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return breakglassGet(cmd.Context(), global, args[0], breakglassGetOpts{
				wait:             wait,
				waitTimeout:      waitTimeout,
				waitPollInterval: pollInterval,
			})
		},
	}
	cmd.Flags().BoolVar(&wait, "wait", false, "Poll until approval decision (does not activate)")
	cmd.Flags().DurationVar(&waitTimeout, "wait-timeout", 5*time.Minute, "Max poll duration when --wait is active")
	cmd.Flags().DurationVar(&pollInterval, "wait-poll-interval", 30*time.Second, "Poll frequency when --wait is active")
	return cmd
}

type breakglassGetOpts struct {
	wait             bool
	waitTimeout      time.Duration
	waitPollInterval time.Duration
}

func breakglassGet(ctx context.Context, global *GlobalOptions, id string, opts breakglassGetOpts) error {
	c, err := getClient(global)
	if err != nil {
		return err
	}
	resp, err := c.GetBreakglassRequest(ctx, id)
	if err != nil {
		return err
	}
	if opts.wait {
		elapsed := time.Duration(0)
		timedOut := false
		if !isTerminalBreakglassStatus(resp.Status) {
			pollRes, pollErr := pollBreakglassRequest(ctx, c, id, breakglassPollConfig{
				interval: opts.waitPollInterval,
				timeout:  opts.waitTimeout,
			})
			if pollErr != nil {
				return pollErr
			}
			resp = pollRes.req
			timedOut = pollRes.timedOut
			elapsed = pollRes.elapsed
			printBreakglassWaitOutcome(resp, timedOut, elapsed)
		}
		return finishBreakglassWait(global, resp, timedOut)
	}
	if global.OutputFormat == output.FormatJSON {
		return output.JSON(os.Stdout, resp)
	}
	renderBreakglassGetHuman(os.Stdout, resp, false)
	return nil
}

type breakglassPollConfig struct {
	interval time.Duration
	timeout  time.Duration
}

type breakglassPollResult struct {
	req      *client.BreakglassRequest
	timedOut bool
	elapsed  time.Duration
}

func isTerminalBreakglassStatus(status string) bool {
	switch status {
	case "active", "denied", "expired":
		return true
	default:
		return false
	}
}

func finishBreakglassWait(global *GlobalOptions, req *client.BreakglassRequest, timedOut bool) error {
	if global.OutputFormat == output.FormatJSON {
		if err := renderBreakglassRequest(global, req, timedOut); err != nil {
			return err
		}
		return breakglassWaitApprovalError(req, timedOut)
	}
	renderBreakglassGetHuman(os.Stdout, req, timedOut)
	return breakglassWaitApprovalError(req, timedOut)
}

// pollBreakglassRequest mirrors run.go poll — shared by breakglass get and request --wait.
func pollBreakglassRequest(ctx context.Context, c APIClient, requestID string, cfg breakglassPollConfig) (*breakglassPollResult, error) {
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
			fmt.Fprintf(os.Stderr, "\r\033[K⠋ %s (%s)", breakglassLifecycleStatus(req), elapsed.Round(time.Second))
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
	lifecycle := breakglassLifecycleStatus(req)
	switch req.Status {
	case "active":
		fmt.Fprintf(breakglassProgressWriter(), "✓ %s (%s)\n", lifecycle, elapsed.Round(time.Second))
	case "denied", "expired":
		fmt.Fprintf(breakglassProgressWriter(), "✗ %s (%s)\n", lifecycle, elapsed.Round(time.Second))
	case "pending":
		if timedOut {
			fmt.Fprintf(breakglassProgressWriter(), "✗ still %s after %s\n", lifecycle, elapsed.Round(time.Second))
		}
	}
}

func renderBreakglassGetHuman(w io.Writer, req *client.BreakglassRequest, timedOut bool) {
	if req == nil {
		return
	}
	deployment := breakglassApprovalDeployment()
	next := breakglassRequestNextStep(req, deployment, timedOut)

	fmt.Fprintf(w, "ID:        %s\n", req.RequestID)
	fmt.Fprintf(w, "PROFILE:   %s\n", req.Profile)
	fmt.Fprintf(w, "TARGET:    %s\n", breakglassTargetCluster(req.TargetCluster))
	fmt.Fprintf(w, "STATUS:    %s\n", breakglassLifecycleStatus(req))
	fmt.Fprintf(w, "APPROVAL:  %s\n", breakglassWorkflowApproval(req))
	fmt.Fprintf(w, "REASON:    %s\n", output.Dash(req.Reason))
	fmt.Fprintf(w, "OPERATOR:  %s\n", output.Dash(req.Operator))
	if req.ReqSessionID != "" {
		fmt.Fprintf(w, "REQ_SESSION_ID: %s\n", req.ReqSessionID)
	}
	if req.ReqSignerARN != "" {
		fmt.Fprintf(w, "REQ_SIGNER_ARN: %s\n", req.ReqSignerARN)
	}
	if req.ReqTaskID != "" {
		fmt.Fprintf(w, "REQ_TASK_ID:    %s\n", req.ReqTaskID)
	}
	if req.TargetType != "" {
		fmt.Fprintf(w, "TYPE:      %s\n", req.TargetType)
	}
	fmt.Fprintf(w, "CREATED:   %s\n", formatBreakglassTime(req.CreatedAt))
	if req.RequestExpiresAt != "" {
		fmt.Fprintf(w, "REQ_EXP:   %s\n", formatBreakglassTime(req.RequestExpiresAt))
	}
	if req.GrantExpiresAt != "" {
		fmt.Fprintf(w, "GRANT_EXP: %s\n", formatBreakglassTime(req.GrantExpiresAt))
	}
	if req.ExpiredAt != "" {
		fmt.Fprintf(w, "EXPIRED:   %s\n", formatBreakglassTime(req.ExpiredAt))
	}
	if at := breakglassDecidedAt(req); at != "" {
		fmt.Fprintf(w, "DECIDED_AT:   %s\n", formatBreakglassTime(at))
	}
	if by := breakglassDecidedBy(req); by != "" {
		fmt.Fprintf(w, "DECIDED_BY:%s\n", by)
	}
	for i, ev := range req.ActivateEvents {
		fmt.Fprintf(w, "ACTIVATE[%d]: %s task=%s session=%s\n",
			i+1, formatBreakglassTime(ev.At), output.Dash(ev.TaskID), output.Dash(ev.SessionID))
	}
	if req.DecisionNote != "" {
		fmt.Fprintf(w, "NOTE:      %s\n", req.DecisionNote)
	}
	fmt.Fprintf(w, "REVISION:  %s\n", output.Dash(req.Revision))
	if timedOut && req.Status == "pending" {
		fmt.Fprintln(w, "NOTE:     poll timed out; still pending")
	}
	if next != "" {
		fmt.Fprintf(w, "NEXT:     %s\n", next)
	}
}

func breakglassWaitApprovalError(req *client.BreakglassRequest, timedOut bool) error {
	switch req.Status {
	case "denied":
		return fmt.Errorf("break-glass request %s was denied", req.RequestID)
	case "expired":
		return fmt.Errorf("break-glass request %s expired before approval", req.RequestID)
	case "pending":
		if timedOut {
			return fmt.Errorf("break-glass request %s still pending after wait timeout", req.RequestID)
		}
		return fmt.Errorf("break-glass request %s not active yet (status %s)", req.RequestID, breakglassLifecycleStatus(req))
	case "active":
		return nil
	default:
		return fmt.Errorf("break-glass request %s unexpected status %q after wait", req.RequestID, req.Status)
	}
}
