package cli

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/output"
)

type auditOptions struct {
	target   string
	action   string
	operator string
	method   string
	approval string
	force    bool
	dryRun   bool
	since    string
	until    string
	limit    int
}

func newAuditCommand(global *GlobalOptions) *cobra.Command {
	opts := &auditOptions{}

	cmd := &cobra.Command{
		Use:   "audit [flags]",
		Short: "View audit log of API calls",
		Example: `  # Show last 50 audit entries (default)
  zoa audit

  # Show API calls from the last 24 hours
  zoa audit --since 24h

  # Show only executions (no GETs) in the last hour
  zoa audit --method POST --since 1h

  # All activity by a specific operator
  zoa audit --operator slopezma --since 7d

  # Writes to a specific cluster
  zoa audit --action rollout_restart -t mc-useast1-1

  # Filter by approval state
  zoa audit --approval not_required --since 7d

  # Show only forced operations (safety override audit)
  zoa audit --force --since 7d

  # Show only dry-run operations
  zoa audit --dry-run --since 24h

  # Max history (up to 200 entries)
  zoa audit --limit 200 --since 7d

  # JSON output — see full non-truncated values
  zoa audit -o json | jq '.items[] | {timestamp, operator, action, path}'

  # JSON with jq filtering — find errors (4xx/5xx)
  zoa audit -o json | jq '.items[] | select(.status_code >= 400)'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listAudit(cmd.Context(), global, opts)
		},
	}

	cmd.Flags().StringVarP(&opts.target, "target", "t", "", "Filter by target cluster")
	cmd.Flags().StringVar(&opts.action, "action", "", "Filter by action name")
	cmd.Flags().StringVar(&opts.operator, "operator", "", "Filter by operator")
	cmd.Flags().StringVar(&opts.method, "method", "", "Filter by HTTP method (GET|POST)")
	cmd.Flags().StringVar(&opts.approval, "approval", "", "Filter by approval state")
	cmd.Flags().BoolVar(&opts.force, "force", false, "Show only forced operations")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "Show only dry-run operations")
	// Default --since 24h ensures the date-bucket-index GSI is always used efficiently.
	// Without a time bound, the API would need to query all daily partitions — expensive at scale.
	cmd.Flags().StringVar(&opts.since, "since", "24h", "Start of time window (duration: 1h, 7d; date: 2026-08-25; RFC3339: 2026-08-25T14:00:00Z)")
	cmd.Flags().StringVar(&opts.until, "until", "", "End of time window (same formats as --since; default: now)")
	cmd.Flags().IntVar(&opts.limit, "limit", 50, "Max results (max 200)")

	return cmd
}

func listAudit(ctx context.Context, global *GlobalOptions, opts *auditOptions) error {
	c, err := getClient(global)
	if err != nil {
		return err
	}

	query := url.Values{}
	if opts.target != "" {
		query.Set("target", opts.target)
	}
	if opts.action != "" {
		query.Set("action", opts.action)
	}
	if opts.operator != "" {
		query.Set("operator", opts.operator)
	}
	if opts.method != "" {
		query.Set("method", opts.method)
	}
	if opts.approval != "" {
		query.Set("approval_state", opts.approval)
	}
	if opts.force {
		query.Set("force", "true")
	}
	if opts.dryRun {
		query.Set("dry_run", "true")
	}
	if opts.since != "" {
		query.Set("since", opts.since)
	}
	if opts.until != "" {
		query.Set("until", opts.until)
	}
	if opts.limit > 0 {
		query.Set("limit", fmt.Sprintf("%d", opts.limit))
	}

	list, err := c.ListAudit(ctx, query)
	if err != nil {
		return err
	}

	if global.OutputFormat == output.FormatJSON {
		return output.JSON(os.Stdout, list)
	}

	wide := global.OutputFormat == output.FormatWide

	if len(list.Items) == 0 {
		fmt.Fprintln(os.Stderr, "No audit entries found")
		return nil
	}

	type row struct {
		ts, method, operator, signerARN, sessionID, action, target, sourceIP, userAgent, jira, approval, execID, path string
		code                                                                                                              int
	}

	rows := make([]row, 0, len(list.Items))
	maxAction, maxTarget, maxIP, maxUA, maxJira := 6, 6, 9, 10, 4

	for _, e := range list.Items {
		ts := e.Timestamp
		if len(ts) >= 19 {
			ts = strings.Replace(ts[:19], "T", " ", 1)
		}

		actionStr := output.Dash(e.Action)
		if e.DryRun {
			actionStr += " [DRY]"
		}
		if e.Force {
			actionStr += " [FORCED]"
		}

		if len(actionStr) > maxAction {
			maxAction = len(actionStr)
		}
		target := output.Dash(e.TargetCluster)
		if len(target) > maxTarget {
			maxTarget = len(target)
		}
		jira := output.Dash(e.Jira)
		if len(jira) > maxJira {
			maxJira = len(jira)
		}
		sourceIP := output.Dash(e.SourceIP)
		if len(sourceIP) > maxIP {
			maxIP = len(sourceIP)
		}
		userAgent := output.Dash(e.UserAgent)
		if len(userAgent) > maxUA {
			maxUA = len(userAgent)
		}

		rows = append(rows, row{
			ts:        ts,
			method:    e.Method,
			code:      e.StatusCode,
			operator:  e.Operator,
			signerARN: e.SignerARN,
			sessionID: e.SessionID,
			action:    actionStr,
			target:    target,
			sourceIP:  sourceIP,
			userAgent: userAgent,
			jira:      jira,
			approval:  output.Dash(e.ApprovalState),
			execID:    output.Dash(e.ExecutionID),
			path:      strings.TrimPrefix(output.Dash(e.Path), "/api/v0/trusted-actions/"),
		})
	}

	if wide {
		tw := output.NewTable(os.Stdout)
		fmt.Fprintln(tw, "TIMESTAMP\tMETHOD\tCODE\tOPERATOR\tSIGNER_ARN\tSESSION_ID\tACTION\tTARGET\tSOURCE_IP\tUSER_AGENT\tJIRA\tAPPROVAL\tEXEC_ID\tPATH")
		for _, r := range rows {
			fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				r.ts, r.method, r.code, r.operator, r.signerARN, output.Dash(r.sessionID),
				r.action, r.target, r.sourceIP, r.userAgent, r.jira, r.approval, r.execID, r.path)
		}
		return tw.Flush()
	}

	fmtStr := fmt.Sprintf("%%-19s  %%-6s  %%-4s  %%-20s  %%-%ds  %%-%ds  %%-%ds  %%-%ds  %%-%ds  %%-14s  %%-36s  %%s\n",
		maxAction, maxTarget, maxIP, maxUA, maxJira)
	fmt.Fprintf(os.Stdout, fmtStr,
		"TIMESTAMP", "METHOD", "CODE", "OPERATOR", "ACTION", "TARGET", "SOURCE_IP", "USER_AGENT", "JIRA", "APPROVAL", "EXEC_ID", "PATH")

	fmtRow := fmt.Sprintf("%%-19s  %%-6s  %%-4d  %%-20s  %%-%ds  %%-%ds  %%-%ds  %%-%ds  %%-%ds  %%-14s  %%-36s  %%s\n",
		maxAction, maxTarget, maxIP, maxUA, maxJira)
	for _, r := range rows {
		fmt.Fprintf(os.Stdout, fmtRow,
			r.ts,
			r.method,
			r.code,
			r.operator,
			r.action,
			r.target,
			r.sourceIP,
			output.Truncate(r.userAgent, 20),
			r.jira,
			r.approval,
			r.execID,
			r.path,
		)
	}

	return nil
}
