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

	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
	"github.com/openshift-online/rosa-seaplane-zoa/internal/output"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/boundaryexec"
)

type sessionListOpts struct {
	deployment string
	mine       bool
	fleetView  bool
	status     string
	target     string
	since      string
	until      string
	operator   string
	limit      int
}

func listSessions(ctx context.Context, global *GlobalOptions, opts sessionListOpts) error {
	if global.APIURL == "" {
		global.Deployment = opts.deployment
	}

	c, err := getClient(global)
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	query := url.Values{}
	if opts.mine {
		query.Set("mine", "true")
		// Legacy Access Lambdas (pre mine=true) still key off scope=mine; harmless when both set.
		query.Set("scope", "mine")
	}
	if opts.since != "" {
		query.Set("since", opts.since)
	}
	if opts.until != "" {
		query.Set("until", opts.until)
	}
	if opts.operator != "" {
		query.Set("operator", opts.operator)
	}
	if opts.status != "" && opts.status != "all" {
		query.Set("status", opts.status)
	}
	if opts.target != "" {
		query.Set("target", opts.target)
	}
	if opts.limit > 0 {
		query.Set("limit", fmt.Sprintf("%d", opts.limit))
	}

	list, err := c.ListSessions(ctx, query)
	if err != nil {
		if opts.fleetView {
			return fmt.Errorf("listing session history: %w", err)
		}
		return fmt.Errorf("listing sessions: %w", err)
	}

	return printSessionTable(os.Stdout, global, opts.deployment, list, opts.fleetView)
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
  zoa session list us-east-1

  zoa session list us-east-1 --status active

  zoa session list us-east-1 -t regional

  zoa session list us-east-1 -o json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return listSessions(cmd.Context(), opts, sessionListOpts{
				deployment: args[0],
				mine:       true,
				status:     status,
				target:     target,
				since:      since,
				until:      until,
				limit:      limit,
			})
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