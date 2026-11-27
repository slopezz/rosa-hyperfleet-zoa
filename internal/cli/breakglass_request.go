package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/cli/parambind"
	"github.com/openshift-online/rosa-seaplane-zoa/internal/cli/reason"
	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
	"github.com/openshift-online/rosa-seaplane-zoa/internal/output"
)

func newBreakglassRequestCommand(global *GlobalOptions) *cobra.Command {
	var (
		reasonFlag   string
		wait         bool
		waitTimeout  time.Duration
		pollInterval time.Duration
	)
	cmd := &cobra.Command{
		Use:   "request <profile>",
		Short: "Create a break-glass request",
		Long: `Create a break-glass request. Default (human): stderr ack + hints; -o json prints the request object on stdout (like zoa run).

A reason is required (Jira issue or PagerDuty incident): use --reason or ZOA_REASON in the boundary task.

With --wait: stderr ack, poll with progress until active/denied/expired, then stdout metadata (like zoa get).
--wait does not activate credentials — run zoa breakglass activate <id> yourself after approval.`,
		Example: `  zoa breakglass request kube_read --reason ROSAENG-1234

  zoa breakglass request kube_elevated --reason ROSAENG-1234 --wait

  zoa breakglass request aws_write --reason '#123456' -o json`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("missing profile name\n\n  Usage: zoa breakglass request <profile> --reason <ticket>\n  Run 'zoa breakglass profiles' to see available profiles")
			}
			if len(args) > 1 {
				return fmt.Errorf("unexpected argument %q — pass flags after the profile name\n\n  Example: zoa breakglass request %s --reason <ticket>",
					args[1], args[0])
			}
			return nil
		},
		ValidArgsFunction: func(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return completeProfileNames(toComplete), cobra.ShellCompDirectiveNoFileComp
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := createBreakglassRequest(cmd.Context(), global, args[0], reasonFlag)
			if err != nil {
				return err
			}
			if !wait {
				progress := breakglassProgressWriter()
				printBreakglassRequestDispatched(progress, resp)
				if global.OutputFormat == output.FormatJSON {
					printBreakglassRequestHints(progress, resp)
					return output.JSON(os.Stdout, resp)
				}
				printBreakglassRequestDetail(progress, resp, breakglassApprovalDeployment(), false, breakglassRequestNextStep(resp, breakglassApprovalDeployment(), false))
				return nil
			}
			progress := breakglassProgressWriter()
			printBreakglassRequestDispatched(progress, resp)
			if isTerminalBreakglassStatus(resp.Status) {
				return finishBreakglassWait(global, resp, false)
			}
			c, err := getClient(global)
			if err != nil {
				return err
			}
			pollRes, err := pollBreakglassRequest(cmd.Context(), c, resp.RequestID, breakglassPollConfig{
				interval: pollInterval,
				timeout:  waitTimeout,
			})
			if err != nil {
				return err
			}
			printBreakglassWaitOutcome(pollRes.req, pollRes.timedOut, pollRes.elapsed)
			return finishBreakglassWait(global, pollRes.req, pollRes.timedOut)
		},
	}
	cmd.Flags().StringVar(&reasonFlag, "reason", "", parambind.RequestFlagHelp("reason"))
	cmd.Flags().BoolVar(&wait, "wait", false, parambind.RequestFlagHelp("wait"))
	cmd.Flags().DurationVar(&waitTimeout, "wait-timeout", 5*time.Minute, parambind.RequestFlagHelp("wait-timeout"))
	cmd.Flags().DurationVar(&pollInterval, "wait-poll-interval", 30*time.Second, parambind.RequestFlagHelp("wait-poll-interval"))
	return cmd
}

func createBreakglassRequest(ctx context.Context, global *GlobalOptions, profileName, reasonFlag string) (*client.BreakglassRequest, error) {
	reasonValue, err := reason.Resolve(reasonFlag)
	if err != nil {
		return nil, err
	}
	c, err := getClient(global)
	if err != nil {
		return nil, err
	}
	return c.CreateBreakglassRequest(ctx, profileName, reasonValue)
}

func printBreakglassRequestDispatched(w io.Writer, req *client.BreakglassRequest) {
	if req == nil || req.RequestID == "" {
		return
	}
	profile := req.Profile
	if profile == "" {
		profile = "unknown"
	}
	fmt.Fprintf(w, "✓ %s [%s] profile=%s\n", req.RequestID, breakglassTargetCluster(req.TargetCluster), profile)
}

func renderBreakglassRequest(global *GlobalOptions, req *client.BreakglassRequest, timedOut bool) error {
	if req == nil {
		return fmt.Errorf("break-glass request is nil")
	}
	deployment := breakglassApprovalDeployment()
	next := breakglassRequestNextStep(req, deployment, timedOut)
	progress := breakglassProgressWriter()
	printBreakglassRequestDispatched(progress, req)

	if global.OutputFormat == output.FormatJSON {
		payload := map[string]interface{}{
			"id":                      req.RequestID,
			"profile":                 req.Profile,
			"target_cluster":          req.TargetCluster,
			"status":                  req.Status,
			"reason":                  req.Reason,
			"operator":                req.Operator,
			"grant_expires_at":        req.GrantExpiresAt,
			"request_expires_at":      req.RequestExpiresAt,
			"approval_wait_timed_out": timedOut && req.Status == "pending",
			"next_step":               next,
		}
		return output.JSON(os.Stdout, payload)
	}

	printBreakglassRequestDetail(progress, req, deployment, timedOut, next)
	return nil
}

func breakglassRequestNextStep(req *client.BreakglassRequest, deployment string, timedOut bool) string {
	switch req.Status {
	case "active":
		return fmt.Sprintf("zoa breakglass activate %s", req.RequestID)
	case "pending":
		if timedOut {
			return fmt.Sprintf("zoa breakglass get %s --wait", req.RequestID)
		}
		return fmt.Sprintf("Peer: zoa approve %s/%s", deployment, req.RequestID)
	case "denied", "expired":
		return ""
	default:
		return fmt.Sprintf("zoa breakglass get %s", req.RequestID)
	}
}

func printBreakglassRequestDetail(w io.Writer, req *client.BreakglassRequest, deployment string, timedOut bool, next string) {
	fmt.Fprintf(w, "  status: %s\n", breakglassLifecycleStatus(req))
	fmt.Fprintf(w, "  approval: %s\n", breakglassWorkflowApproval(req))
	if d := breakglassDecidedSummary(req); d != "" {
		fmt.Fprintf(w, "  decided: %s\n", d)
	}
	if req.RequestExpiresAt != "" {
		fmt.Fprintf(w, "  request_expires: %s\n", formatBreakglassTime(req.RequestExpiresAt))
	}
	if req.GrantExpiresAt != "" {
		fmt.Fprintf(w, "  grant_expires: %s\n", formatBreakglassTime(req.GrantExpiresAt))
	}
	if req.ExpiredAt != "" {
		fmt.Fprintf(w, "  expired_at: %s\n", formatBreakglassTime(req.ExpiredAt))
	}

	switch req.Status {
	case "active":
		fmt.Fprintf(w, "  run: %s\n", next)
	case "pending":
		if timedOut {
			fmt.Fprintln(w, "  still pending approval (poll timed out)")
		}
		fmt.Fprintf(w, "  peer approve: zoa approve %s/%s\n", deployment, req.RequestID)
		fmt.Fprintf(w, "  track:        zoa breakglass get %s --wait\n", req.RequestID)
	case "denied":
		fmt.Fprintln(w, "  denied — do not activate")
	case "expired":
		fmt.Fprintln(w, "  expired — create a new request if still needed")
	}
}

func printBreakglassRequestHints(w io.Writer, req *client.BreakglassRequest) {
	if req == nil {
		return
	}
	deployment := breakglassApprovalDeployment()
	switch req.Status {
	case "active":
		fmt.Fprintf(w, "  run: zoa breakglass activate %s\n", req.RequestID)
	case "pending":
		fmt.Fprintf(w, "  peer approve: zoa approve %s/%s\n", deployment, req.RequestID)
		fmt.Fprintf(w, "  track:        zoa breakglass get %s --wait\n", req.RequestID)
	}
}
