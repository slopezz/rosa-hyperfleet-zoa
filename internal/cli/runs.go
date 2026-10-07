package cli

import (
	"context"
	"fmt"
	"net/url"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/output"
)

type runsOptions struct {
	target        string
	status        string
	action        string
	operator      string
	scope         string
	actionType    string
	executionMode string
	dryRun        bool
	force         bool
	since         string
	until         string
	limit         int
}

func newRunsCommand(global *GlobalOptions) *cobra.Command {
	opts := &runsOptions{}

	cmd := &cobra.Command{
		Use:   "runs [flags]",
		Short: "List recent executions",
		Example: `  # List recent executions on a specific cluster
  zoa runs -t eph-bc5fee45-mc01

  # Filter by cluster and time window
  zoa runs -t eph-bc5fee45-mc01 --since 1h

  # Show only failed runs in the last 24 hours
  zoa runs --status failed --since 24h

  # Show runs where output capture failed
  zoa runs --output-status failed --since 7d

  # Filter by action, operator, and time window
  zoa runs --action get_pods --operator slopezma --since 7d

  # Show only write operations in the last 12 hours
  zoa runs --type write --since 12h

  # Filter by scope and status
  zoa runs --scope kube-api --status succeeded --limit 50

  # Filter by approval state
  zoa runs --approval not_required --since 7d

  # Show only dry-run or forced executions
  zoa runs --dry-run --since 24h
  zoa runs --force --since 7d

  # JSON output — see full non-truncated values
  zoa runs -o json | jq '.items[] | {id, action, params, target_cluster}'

  # JSON with jq filtering (e.g., slow runs)
  zoa runs -o json | jq '.items[] | select(.runner_seconds > 10)'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listRuns(cmd.Context(), global, opts)
		},
	}

	cmd.Flags().StringVarP(&opts.target, "target", "t", "", "Filter by target cluster")
	cmd.Flags().StringVar(&opts.status, "status", "", "Filter by status")
	cmd.Flags().StringVar(&opts.action, "action", "", "Filter by action name")
	cmd.Flags().StringVar(&opts.operator, "operator", "", "Filter by operator")
	cmd.Flags().StringVar(&opts.scope, "scope", "", "Filter by scope (kube-api|aws-api)")
	cmd.Flags().StringVar(&opts.actionType, "type", "", "Filter by type (read|write)")
	cmd.Flags().StringVar(&opts.executionMode, "execution-mode", "", "Filter by execution class (sync|async)")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "Show only dry-run executions")
	cmd.Flags().BoolVar(&opts.force, "force", false, "Show only forced executions")
	// Default --since 24h ensures the date-bucket-index GSI is always used efficiently.
	// Without a time bound, the API would need to query all daily partitions — expensive at scale.
	cmd.Flags().StringVar(&opts.since, "since", "24h", "Start of time window (duration: 1h, 7d; date: 2026-08-25; RFC3339: 2026-08-25T14:00:00Z)")
	cmd.Flags().StringVar(&opts.until, "until", "", "End of time window (same formats as --since; default: now)")
	cmd.Flags().IntVar(&opts.limit, "limit", 20, "Max results (max 100)")

	return cmd
}

func listRuns(ctx context.Context, global *GlobalOptions, opts *runsOptions) error {
	c, err := getClient(global)
	if err != nil {
		return err
	}

	query := url.Values{}
	if opts.target != "" {
		query.Set("target", opts.target)
	}
	if opts.status != "" {
		query.Set("status", opts.status)
	}
	if opts.action != "" {
		query.Set("action", opts.action)
	}
	if opts.operator != "" {
		query.Set("operator", opts.operator)
	}
	if opts.scope != "" {
		query.Set("scope", opts.scope)
	}
	if opts.actionType != "" {
		query.Set("type", opts.actionType)
	}
	if opts.executionMode != "" {
		query.Set("execution_mode", opts.executionMode)
	}
	if opts.dryRun {
		query.Set("dry_run", "true")
	}
	if opts.force {
		query.Set("force", "true")
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

	list, err := c.ListExecutions(ctx, query)
	if err != nil {
		return err
	}

	if global.OutputFormat == output.FormatJSON {
		return output.JSON(os.Stdout, list)
	}

	if len(list.Items) == 0 {
		fmt.Fprintln(os.Stderr, "No executions found")
		return nil
	}

	wide := global.OutputFormat == output.FormatWide

	type row struct {
		created, operator, signerARN, sessionID, id, action, params, target string
		scope, typ, status, rev, class                                      string
		jira, total, outBytes                                               string
		dispatchedAt, completedAt, logBytes                                 string
	}

	rows := make([]row, 0, len(list.Items))
	maxAction, maxParams, maxTarget, maxJira, maxOperator := 6, 6, 6, 4, 8

	for _, e := range list.Items {
		actionStr := e.Action
		if e.DryRun {
			if e.RequestedAction != "" {
				actionStr += " [DRY←" + e.RequestedAction + "]"
			} else {
				actionStr += " [DRY]"
			}
		}
		if e.Force {
			actionStr += " [FORCED]"
		}
		params := formatParams(e.Params)

		if len(actionStr) > maxAction {
			maxAction = len(actionStr)
		}
		if len(params) > maxParams {
			maxParams = len(params)
		}
		if len(e.TargetCluster) > maxTarget {
			maxTarget = len(e.TargetCluster)
		}
		if len(e.Jira) > maxJira {
			maxJira = len(e.Jira)
		}
		if len(e.Operator) > maxOperator {
			maxOperator = len(e.Operator)
		}

		rev := output.Dash(e.Revision)
		if len(rev) > 7 && rev != "-" {
			rev = rev[:7]
		}

		rows = append(rows, row{
			created:      output.FormatTimestamp(e.CreatedAt),
			operator:     e.Operator,
			signerARN:    e.SignerARN,
			sessionID:    e.SessionID,
			id:           e.ID,
			action:       actionStr,
			params:       params,
			target:       e.TargetCluster,
			scope:        e.Scope,
			typ:          output.Dash(e.Type),
			status:       e.Status,
			rev:          rev,
			class:        output.Dash(e.ExecutionMode),
			jira:         output.Dash(e.Jira),
			total:        output.FormatDuration(e.DurationMs),
			outBytes:     output.FormatBytes(e.OutputBytes),
			dispatchedAt: output.FormatTimestamp(e.DispatchedAt),
			completedAt:  output.FormatTimestamp(e.CompletedAt),
			logBytes:     output.FormatBytes(e.LogBytes),
		})
	}

	if wide {
		tw := output.NewTable(os.Stdout)
		fmt.Fprintln(tw, "CREATED_AT\tOPERATOR\tSIGNER_ARN\tSESSION_ID\tID\tACTION\tPARAMS\tTARGET\tSCOPE\tTYPE\tSTATUS\tREV\tMODE\tJIRA\tDUR\tOUT\tDISPATCHED_AT\tCOMPLETED_AT\tLOGS")
		for _, r := range rows {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				r.created,
				r.operator,
				r.signerARN,
				output.Dash(r.sessionID),
				r.id,
				r.action,
				output.Truncate(r.params, 70),
				r.target,
				r.scope,
				r.typ,
				r.status,
				r.rev,
				r.class,
				r.jira,
				r.total,
				r.outBytes,
				r.dispatchedAt,
				r.completedAt,
				r.logBytes,
			)
		}
		tw.Flush()
		return nil
	}

	// Default table
	if maxAction > 30 {
		maxAction = 30
	}
	if maxParams > 50 {
		maxParams = 50
	}
	if maxTarget > 25 {
		maxTarget = 25
	}
	if maxJira > 15 {
		maxJira = 15
	}

	if maxOperator > 24 {
		maxOperator = 24
	}
	fmtStr := fmt.Sprintf("%%-19s  %%-%ds  %%-36s  %%-%ds  %%-%ds  %%-%ds  %%-9s  %%-6s  %%-12s  %%-7s  %%-6s  %%-%ds  %%-6s  %%s\n",
		maxOperator, maxAction, maxParams, maxTarget, maxJira)

	fmt.Fprintf(os.Stdout, fmtStr,
		"CREATED_AT", "OPERATOR", "ID", "ACTION", "PARAMS", "TARGET", "SCOPE", "TYPE", "STATUS", "REV", "MODE", "JIRA", "DUR", "OUT")

	for _, r := range rows {
		fmt.Fprintf(os.Stdout, fmtStr,
			r.created,
			r.operator,
			r.id,
			output.Truncate(r.action, maxAction),
			output.Truncate(r.params, maxParams),
			output.Truncate(r.target, maxTarget),
			r.scope,
			r.typ,
			r.status,
			r.rev,
			r.class,
			output.Truncate(r.jira, maxJira),
			r.total,
			r.outBytes,
		)
	}

	return nil
}

func formatParams(params map[string]string) string {
	if len(params) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(params))
	for k, v := range params {
		parts = append(parts, k+"="+v)
	}
	return fmt.Sprintf("%s", parts)
}
