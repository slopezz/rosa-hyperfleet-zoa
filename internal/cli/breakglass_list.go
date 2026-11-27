package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
	"github.com/openshift-online/rosa-seaplane-zoa/internal/output"
)

type breakglassListOpts struct {
	mine         bool
	fleetView    bool
	target       string
	profile      string
	operator     string
	since        string
	until        string
	status       string
	approval     string
	decidedBy    string
	decidedSince string
	limit        int
}

func listBreakglassRequests(ctx context.Context, global *GlobalOptions, opts breakglassListOpts) error {
	c, err := getClient(global)
	if err != nil {
		return err
	}
	list, err := c.ListBreakglassRequests(ctx, &client.BreakglassListParams{
		Mine:         opts.mine,
		Target:       opts.target,
		Profile:      opts.profile,
		Operator:     opts.operator,
		Since:        opts.since,
		Until:        opts.until,
		Status:       opts.status,
		Approval:     opts.approval,
		DecidedBy:    opts.decidedBy,
		DecidedSince: opts.decidedSince,
		Limit:        opts.limit,
	})
	if err != nil {
		return err
	}
	return printBreakglassRequestTable(global, list, opts.fleetView)
}

func newBreakglassListCommand(global *GlobalOptions) *cobra.Command {
	var since, until, status, approval, decidedBy, decidedSince, target, profile string
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List your break-glass requests",
		Long: `List your break-glass requests on this deployment (mine=true). Not audited — same idea as zoa session list.

Lists RC and MC targets in the shared break-glass table unless --target is set (same as zoa runs).

Use 'zoa breakglass history' for fleet-wide audit across all operators.`,
		Example: `  zoa breakglass list
  zoa breakglass list --since 7d --status pending
  zoa breakglass list -t eph-regional -o wide`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return listBreakglassRequests(cmd.Context(), global, breakglassListOpts{
				mine:         true,
				target:       target,
				profile:      profile,
				since:        since,
				until:        until,
				status:       status,
				approval:     approval,
				decidedBy:    decidedBy,
				decidedSince: decidedSince,
				limit:        limit,
			})
		},
	}
	cmd.Flags().StringVarP(&target, "target", "t", "", "Filter by target cluster (default: all targets in deployment)")
	cmd.Flags().StringVar(&profile, "profile", "", "Filter by break-glass profile (e.g. kube_read)")
	cmd.Flags().StringVar(&since, "since", "24h", "Start of time window (duration: 1h, 7d; date: 2026-08-25; RFC3339)")
	cmd.Flags().StringVar(&until, "until", "", "End of time window (same formats as --since; default: now)")
	cmd.Flags().StringVar(&status, "status", "", "Filter by lifecycle status (pending, active, expired, denied)")
	cmd.Flags().StringVar(&approval, "approval", "", "Filter by approval outcome (pending, approved, rejected)")
	cmd.Flags().StringVar(&decidedBy, "decided-by", "", "Filter by approver/rejecter (peer decisions)")
	cmd.Flags().StringVar(&decidedSince, "decided-since", "", "Only rows decided on/after this time (duration or RFC3339)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Max results (max 200)")
	return cmd
}

func breakglassProgressWriter() io.Writer {
	return os.Stderr
}

func breakglassTargetCluster(cluster string) string {
	if cluster != "" {
		return cluster
	}
	if t := os.Getenv("ZOA_TARGET"); t != "" {
		return t
	}
	return "unknown"
}

func breakglassApprovalDeployment() string {
	if d := os.Getenv("ZOA_DEPLOYMENT"); d != "" {
		return d
	}
	return "<deployment>"
}

func breakglassLifecycleStatus(req *client.BreakglassRequest) string {
	if req == nil || req.Status == "" {
		return "-"
	}
	return req.Status
}

func breakglassWorkflowApproval(req *client.BreakglassRequest) string {
	if req == nil || req.Approval == "" {
		return "-"
	}
	return req.Approval
}

func breakglassDecidedAt(req *client.BreakglassRequest) string {
	if req == nil {
		return ""
	}
	return req.DecidedAt
}

func breakglassDecidedBy(req *client.BreakglassRequest) string {
	if req == nil {
		return ""
	}
	return req.DecidedBy
}

func breakglassDecidedSummary(req *client.BreakglassRequest) string {
	at := breakglassDecidedAt(req)
	if at == "" {
		return ""
	}
	ts := formatBreakglassTime(at)
	by := breakglassDecidedBy(req)
	if by != "" {
		return fmt.Sprintf("%s · %s", ts, by)
	}
	return ts
}

func breakglassActivatesSummary(req *client.BreakglassRequest) string {
	if req == nil || len(req.ActivateAt) == 0 {
		return "-"
	}
	if len(req.ActivateAt) == 1 {
		return formatBreakglassTime(req.ActivateAt[0])
	}
	return fmt.Sprintf("%d activates", len(req.ActivateAt))
}

func breakglassSessionDisplay(sessionID string, wide bool) string {
	if sessionID == "" {
		return "-"
	}
	if wide || len(sessionID) <= 12 {
		return sessionID
	}
	return sessionID[:8]
}

func breakglassSignerShort(arn string) string {
	if arn == "" {
		return "-"
	}
	if i := strings.LastIndex(arn, "/"); i >= 0 && i < len(arn)-1 {
		return arn[i+1:]
	}
	return arn
}

func breakglassValidUntil(req *client.BreakglassRequest) string {
	if req == nil {
		return ""
	}
	switch req.Status {
	case "pending":
		return req.RequestExpiresAt
	case "active":
		return req.GrantExpiresAt
	default:
		return ""
	}
}

func breakglassTimeLeftShort(until string) string {
	if until == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339Nano, until)
	if err != nil {
		t, err = time.Parse(time.RFC3339, until)
	}
	if err != nil {
		return ""
	}
	d := time.Until(t)
	if d < 0 {
		return "ended"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Round(time.Minute)/time.Minute))
	}
	h := int((d + time.Hour - time.Nanosecond) / time.Hour)
	return fmt.Sprintf("%dh", h)
}

func printBreakglassRequestTable(global *GlobalOptions, list *client.BreakglassRequestList, fleet bool) error {
	if global.OutputFormat == output.FormatJSON {
		return output.JSON(os.Stdout, list)
	}
	if list == nil || len(list.Items) == 0 {
		fmt.Fprintln(os.Stdout, "No break-glass requests found")
		return nil
	}

	wide := global.OutputFormat == output.FormatWide
	tw := output.NewTable(os.Stdout)

	rowHeader := "CREATED\tSESSION\tID\tPROFILE\tTARGET\tSTATUS\tAPPROVAL\tREQ_EXPIRES\tGRANT_EXPIRES\tEXPIRED_AT\tDECIDED_AT\tDECIDED_BY\tREASON"
	if fleet {
		rowHeader = "CREATED\tOPERATOR\tSESSION\tID\tPROFILE\tTARGET\tSTATUS\tAPPROVAL\tREQ_EXPIRES\tGRANT_EXPIRES\tEXPIRED_AT\tDECIDED_AT\tDECIDED_BY\tREASON"
	}
	if wide {
		fmt.Fprintln(tw, rowHeader+"\tSIGNER\tTASK_ID\tACCOUNT\tACTIVATES\tNOTE")
	} else {
		fmt.Fprintln(tw, rowHeader)
	}

	for i := range list.Items {
		r := &list.Items[i]
		created := formatBreakglassTime(r.CreatedAt)
		session := breakglassSessionDisplay(r.SessionID, wide)
		requestID := r.RequestID
		profile := r.Profile
		target := output.Truncate(breakglassTargetCluster(r.TargetCluster), 25)
		status := breakglassLifecycleStatus(r)
		approval := breakglassWorkflowApproval(r)
		reqExp := formatBreakglassTime(r.RequestExpiresAt)
		grantExp := formatBreakglassTime(r.GrantExpiresAt)
		expiredAt := formatBreakglassTime(r.ExpiredAt)
		decidedAt := formatBreakglassTime(breakglassDecidedAt(r))
		decidedBy := output.Dash(breakglassDecidedBy(r))
		reason := output.Truncate(output.Dash(r.Reason), 40)
		if !wide && !fleet {
			reason = output.Truncate(output.Dash(r.Reason), 30)
		}

		if wide {
			row := fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s",
				created, session, requestID, profile, target, status, approval,
				reqExp, grantExp, expiredAt, decidedAt, decidedBy, reason,
			)
			if fleet {
				row = fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s",
					created, output.Dash(r.Operator), session, requestID, profile, target, status, approval,
					reqExp, grantExp, expiredAt, decidedAt, decidedBy, reason,
				)
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
				row,
				output.Truncate(breakglassSignerShort(r.SignerARN), 24),
				output.Dash(r.CreatorTaskID),
				output.Dash(r.AccountID),
				breakglassActivatesSummary(r),
				output.Truncate(output.Dash(r.DecisionNote), 30),
			)
			continue
		}

		if fleet {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				created, output.Dash(r.Operator), session, requestID, profile, target, status, approval,
				reqExp, grantExp, expiredAt, decidedAt, decidedBy, reason,
			)
		} else {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				created, session, requestID, profile, target, status, approval,
				reqExp, grantExp, expiredAt, decidedAt, decidedBy, reason,
			)
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	return nil
}

func formatBreakglassTime(s string) string {
	if s == "" {
		return "-"
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC().Format("2006-01-02 15:04:05")
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC().Format("2006-01-02 15:04:05")
	}
	return output.Truncate(s, 19)
}
