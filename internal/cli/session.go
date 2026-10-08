package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/cli/reason"
	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/client"
	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/output"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/boundaryexec"
)

func newSessionCommand(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session",
		Short: "Manage boundary sessions",
		Long: `Start, terminate, join, and list boundary sessions for audited SRE access.

Session IDs use the form <deployment>/<session-id> (see 'zoa session start').`,
	}

	cmd.AddCommand(
		newSessionStartCommand(opts),
		newSessionTerminateCommand(opts),
		newSessionJoinCommand(opts),
		newSessionListCommand(opts),
		newSessionHistoryCommand(opts),
	)

	return cmd
}

func newSessionStartCommand(opts *GlobalOptions) *cobra.Command {
	var flagDeployment, flagTarget string
	var flagReason string
	var flagNoConnect bool

	cmd := &cobra.Command{
		Use:   "start [deployment] [target]",
		Short: "Start a boundary session",
		Long: `Start a boundary session for audited SRE access to a target cluster.

Positional args: <deployment> <target>. Also available as flags for scripts.
The CLI auto-resolves the Access Lambda URL and invoker role from SSM.

By default, after the task is active the CLI connects via ECS Exec (same as
'zoa session join'). Use --no-connect to only print session metadata and exit
(same idea as 'zoa run --no-wait': create, show id, do not attach).

ECS Exec uses scoped credentials returned by the Access API on join (not
deployment-account admin roles).

A reason is required (Jira issue or PagerDuty incident). It is stored
on the session, Access audit, and the boundary task (ZOA_REASON) for zoa run.`,
		Example: `  zoa session start us-east-1 mc01 --reason ROSAENG-1234

  zoa session start us-east-1 mc01 --reason '#123456' --no-connect

  zoa session start -d us-east-1 -t mc01 --reason ROSAENG-1234 --no-connect -o json`,
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			deployment, target := resolveDeploymentTarget(args, flagDeployment, flagTarget)
			if deployment == "" {
				return fmt.Errorf("deployment is required: zoa session start <deployment> <target>")
			}
			if target == "" {
				return fmt.Errorf("target is required: zoa session start <deployment> <target>")
			}

			if opts.APIURL == "" {
				opts.Deployment = deployment
			}

			reasonValue, err := reason.Resolve(flagReason)
			if err != nil {
				return err
			}

			c, err := getClient(opts)
			if err != nil {
				return fmt.Errorf("creating client: %w", err)
			}

			resp, err := c.SessionStart(cmd.Context(), &client.SessionStartRequest{
				DeploymentName: deployment,
				Target:         target,
				Reason:         reasonValue,
			})
			if err != nil {
				return fmt.Errorf("starting session: %w", err)
			}

			compoundID := FormatSessionID(deployment, resp.SessionID)
			displayTarget := sessionStartTarget(deployment, target, resp)

			if opts.OutputFormat != output.FormatJSON {
				printSessionDispatched(sessionProgressWriter(), compoundID, displayTarget)
			}

			connect := !flagNoConnect
			if connect && opts.OutputFormat == output.FormatJSON {
				connect = false
			}

			if connect {
				var joinResp *client.SessionJoinResponse
				err := runWithSpinner(cmd.Context(), "provisioning boundary", func(ctx context.Context) error {
					var joinErr error
					joinResp, joinErr = accessClientForJoin(c).SessionJoin(ctx, resp.SessionID)
					return joinErr
				})
				if err != nil {
					return fmt.Errorf("joining session after start: %w", err)
				}
				region := sessionJoinRegion(opts, joinResp)
				if err := runSessionECSExec(cmd.Context(), deployment, region, resp.SessionID, accessClientForJoin(c), joinResp); err != nil {
					return fmt.Errorf("ECS Exec: %w", err)
				}
				printSessionExecEndedHint(sessionProgressWriter(), deployment, compoundID, joinDeadline(resp.Deadline, joinResp))
				return nil
			}

			if opts.OutputFormat == output.FormatJSON {
				result := map[string]interface{}{
					"session_id": compoundID,
					"raw_id":     resp.SessionID,
					"deployment": deployment,
					"target":     target,
					"status":     resp.Status,
				}
				if resp.TaskArn != "" {
					result["task_arn"] = resp.TaskArn
				}
				if resp.Region != "" {
					result["region"] = resp.Region
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			}

			printSessionJoinHint(sessionProgressWriter(), compoundID)
			return nil
		},
	}

	cmd.Flags().StringVarP(&flagDeployment, "deployment", "d", "", "Deployment name (e.g. us-east-1)")
	cmd.Flags().StringVarP(&flagTarget, "target", "t", "", "Target ID (e.g. mc01)")
	cmd.Flags().StringVar(&flagReason, "reason", "", "Reason for this session (required: Jira issue or PagerDuty incident, e.g. ROSAENG-1234 or #123456)")
	cmd.Flags().BoolVar(&flagNoConnect, "no-connect", false, "Print session metadata only; do not open ECS Exec (same idea as zoa run --no-wait)")

	return cmd
}

func newSessionTerminateCommand(opts *GlobalOptions) *cobra.Command {
	run := func(cmd *cobra.Command, args []string) error {
		deployment, rawID, err := ParseSessionID(args[0])
		if err != nil {
			return err
		}

		if opts.APIURL == "" {
			opts.Deployment = deployment
		}

		c, err := getClient(opts)
		if err != nil {
			return fmt.Errorf("creating client: %w", err)
		}

		if err := c.SessionTerminate(cmd.Context(), rawID); err != nil {
			return fmt.Errorf("terminating session: %w", err)
		}

		fmt.Printf("Session %s terminated\n", args[0])
		return nil
	}

	return &cobra.Command{
		Use:   "terminate <deployment/session-id>",
		Short: "Terminate a boundary session",
		Long: `Terminate a boundary session by its compound ID (deployment/session-id).

Stops the ECS task and marks the session terminated in DynamoDB. This is final —
the task cannot be restarted. The compound ID is returned by 'zoa session start'
and 'zoa session list'.`,
		Example: `  zoa session terminate us-east-1/sess-abc123`,
		Args:    cobra.ExactArgs(1),
		RunE:    run,
	}
}

func newSessionJoinCommand(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "join <deployment/session-id>",
		Short: "Join a boundary session via ECS Exec",
		Long: `Join a boundary session by its compound ID (deployment/session-id).

Calls the Access API for ownership checks, then opens an interactive shell via
ECS Exec and session-manager-plugin using scoped credentials from the join response.`,
		Example: `  zoa session join us-east-1/sess-abc123`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deployment, rawID, err := ParseSessionID(args[0])
			if err != nil {
				return err
			}

			if opts.APIURL == "" {
				opts.Deployment = deployment
			}

			c, err := getClient(opts)
			if err != nil {
				return fmt.Errorf("creating client: %w", err)
			}

			resp, err := c.SessionJoin(cmd.Context(), rawID)
			if err != nil {
				return fmt.Errorf("joining session: %w", err)
			}

			if opts.OutputFormat == output.FormatJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(resp)
			}

			region := sessionJoinRegion(opts, resp)
			if err := runSessionECSExec(cmd.Context(), deployment, region, rawID, accessClientForJoin(c), resp); err != nil {
				return fmt.Errorf("ECS Exec: %w", err)
			}
			compoundID := FormatSessionID(deployment, rawID)
			printSessionExecEndedHint(sessionProgressWriter(), deployment, compoundID, resp.Deadline)
			return nil
		},
	}

	return cmd
}

func joinDeadline(startDeadline string, join *client.SessionJoinResponse) string {
	if join != nil && join.Deadline != "" {
		return join.Deadline
	}
	return startDeadline
}

func newSessionListCommand(opts *GlobalOptions) *cobra.Command {
	var status, target, since, until string
	var limit int

	cmd := &cobra.Command{
		Use:   "list <deployment>",
		Short: "List your boundary sessions",
		Long: `List your boundary sessions for a deployment (last 24 hours).

Only the signed-in operator's sessions are returned; the API does not allow
listing other operators here. Use 'zoa session history' for fleet-wide audit.`,
		Example: `  # All your sessions in the last 24h (default)
  zoa session list us-east-1-eph-f37869e8

  zoa session list us-east-1-eph-f37869e8 --status active

  zoa session list us-east-1-eph-f37869e8 -t eph-f37869e8-regional

  zoa session list us-east-1-eph-f37869e8 -o json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deployment := args[0]
			if opts.APIURL == "" {
				opts.Deployment = deployment
			}

			c, err := getClient(opts)
			if err != nil {
				return fmt.Errorf("creating client: %w", err)
			}

			query := url.Values{}
			query.Set("scope", "mine")
			if since != "" {
				query.Set("since", since)
			}
			if until != "" {
				query.Set("until", until)
			}
			if status != "" && status != "all" {
				query.Set("status", status)
			}
			if target != "" {
				query.Set("target", target)
			}
			if limit > 0 {
				query.Set("limit", fmt.Sprintf("%d", limit))
			}

			list, err := c.ListSessions(cmd.Context(), query)
			if err != nil {
				return fmt.Errorf("listing sessions: %w", err)
			}

			return printSessionTable(os.Stdout, opts, deployment, list, false)
		},
	}

	cmd.Flags().StringVar(&status, "status", "all", "Filter by status (creating, active, terminated, failed, all)")
	cmd.Flags().StringVarP(&target, "target", "t", "", "Filter by target cluster")
	// Default --since 24h keeps date-bucket-index queries bounded (same as zoa runs / session history).
	cmd.Flags().StringVar(&since, "since", "24h", "Start of time window (duration: 1h, 7d; date: 2026-08-25; RFC3339)")
	cmd.Flags().StringVar(&until, "until", "", "End of time window (same formats as --since; default: now)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Max results (max 200)")

	return cmd
}

func newSessionHistoryCommand(opts *GlobalOptions) *cobra.Command {
	var since, until, operator, target, status string
	var limit int

	cmd := &cobra.Command{
		Use:   "history <deployment>",
		Short: "View session history across all operators",
		Long: `Show session history across all operators (audit view).
Defaults to last 24 hours.`,
		Example: `  zoa session history us-east-1

  zoa session history us-east-1 --since 7d

  zoa session history us-east-1 --operator slopezma --since 7d

  zoa session history us-east-1 --status terminated --since 7d

  zoa session history us-east-1 -o json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deployment := args[0]
			if opts.APIURL == "" {
				opts.Deployment = deployment
			}

			c, err := getClient(opts)
			if err != nil {
				return fmt.Errorf("creating client: %w", err)
			}

			query := url.Values{}
			if since != "" {
				query.Set("since", since)
			}
			if until != "" {
				query.Set("until", until)
			}
			if operator != "" {
				query.Set("operator", operator)
			}
			if target != "" {
				query.Set("target", target)
			}
			if status != "" && status != "all" {
				query.Set("status", status)
			}
			if limit > 0 {
				query.Set("limit", fmt.Sprintf("%d", limit))
			}

			list, err := c.ListSessions(cmd.Context(), query)
			if err != nil {
				return fmt.Errorf("listing session history: %w", err)
			}

			return printSessionTable(os.Stdout, opts, deployment, list, true)
		},
	}

	cmd.Flags().StringVar(&since, "since", "24h", "Start of time window (duration: 1h, 7d; date: 2026-08-25; RFC3339)")
	cmd.Flags().StringVar(&until, "until", "", "End of time window (same formats as --since; default: now)")
	cmd.Flags().StringVar(&operator, "operator", "", "Filter by operator")
	cmd.Flags().StringVarP(&target, "target", "t", "", "Filter by target cluster")
	cmd.Flags().StringVar(&status, "status", "", "Filter by status (creating, active, terminated, failed, all)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Max results (max 200)")

	return cmd
}

// formatExecSessionsForTable renders ECS Exec (SSM) join ids for session list/history.
// Full stream names are in JSON (-o json) as exec_session_ids; table view stays compact.
func formatExecSessionsForTable(ids []string) string {
	if len(ids) == 0 {
		return output.Dash("")
	}
	if len(ids) == 1 {
		return output.Truncate(boundaryexec.NormalizeExecSessionID(ids[0]), 28)
	}
	return strconv.Itoa(len(ids)) + " joins"
}

func printSessionTable(w io.Writer, opts *GlobalOptions, deployment string, list *client.SessionList, history bool) error {
	if opts.OutputFormat == output.FormatJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(list)
	}

	if len(list.Items) == 0 {
		fmt.Fprintln(w, "No sessions found")
		return nil
	}

	wide := opts.OutputFormat == output.FormatWide
	tw := output.NewTable(w)
	if wide {
		header := "SESSION ID\tOPERATOR\tSIGNER_ARN\tACCOUNT_ID\tTASK_ID\tTARGET\tREASON\tSTATUS\tSTOP REASON\tEXEC SESSIONS\tCREATED\tENDED\tDEADLINE"
		fmt.Fprintln(tw, header)
	} else if history {
		fmt.Fprintln(tw, "SESSION ID\tOPERATOR\tTARGET\tREASON\tSTATUS\tSTOP REASON\tEXEC SESSIONS\tCREATED\tENDED\tDEADLINE")
	} else {
		fmt.Fprintln(tw, "SESSION ID\tTARGET\tREASON\tSTATUS\tSTOP REASON\tEXEC SESSIONS\tCREATED\tENDED\tDEADLINE")
	}
	for _, s := range list.Items {
		sessionID := FormatSessionID(deployment, s.SessionID)
		reasonCol := output.Dash(s.Reason)
		stopReason := output.Dash(s.StopReason)
		execSessions := formatExecSessionsForTable(s.ExecSessionIDs)
		created := output.Dash(s.CreatedAt)
		ended := output.Dash(s.CompletedAt)
		deadline := output.Dash(s.Deadline)
		target := s.TargetCluster
		if wide {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				sessionID, s.Operator, s.SignerARN, output.Dash(s.AccountID), output.Dash(s.TaskID),
				target, reasonCol, s.Status, stopReason, execSessions, created, ended, deadline)
			continue
		}
		if history {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				sessionID, s.Operator, target, reasonCol, s.Status,
				stopReason, execSessions, created, ended, deadline)
		} else {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				sessionID, target, reasonCol, s.Status,
				stopReason, execSessions, created, ended, deadline)
		}
	}
	return tw.Flush()
}

// resolveDeploymentTarget resolves deployment and target from positional args
// or flag fallbacks. Positional args take precedence over flags.
func resolveDeploymentTarget(args []string, flagDeployment, flagTarget string) (deployment, target string) {
	switch len(args) {
	case 2:
		return args[0], args[1]
	case 1:
		return args[0], flagTarget
	default:
		return flagDeployment, flagTarget
	}
}
