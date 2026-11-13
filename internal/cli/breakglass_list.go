package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
	"github.com/openshift-online/rosa-seaplane-zoa/internal/output"
)

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

	rowHeader := "CREATED\tSESSION\tREQUEST\tSCOPE\tTARGET\tSTATUS\tAPPROVAL\tREQ_EXPIRES\tGRANT_EXPIRES\tEXPIRED_AT\tDECIDED_AT\tDECIDED_BY\tREASON"
	if fleet {
		rowHeader = "CREATED\tOPERATOR\tSESSION\tREQUEST\tSCOPE\tTARGET\tSTATUS\tAPPROVAL\tREQ_EXPIRES\tGRANT_EXPIRES\tEXPIRED_AT\tDECIDED_AT\tDECIDED_BY\tREASON"
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
		scope := r.Scope
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
				created, session, requestID, scope, target, status, approval,
				reqExp, grantExp, expiredAt, decidedAt, decidedBy, reason,
			)
			if fleet {
				row = fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s",
					created, output.Dash(r.Operator), session, requestID, scope, target, status, approval,
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
				created, output.Dash(r.Operator), session, requestID, scope, target, status, approval,
				reqExp, grantExp, expiredAt, decidedAt, decidedBy, reason,
			)
		} else {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				created, session, requestID, scope, target, status, approval,
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
