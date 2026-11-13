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
	if wide {
		fmt.Fprintln(tw, "CREATED\tOPERATOR\tID\tSCOPE\tTARGET\tSTATUS\tAPPROVAL\tREASON\tAPPROVED_BY\tGRANT_EXPIRES")
	} else if fleet {
		fmt.Fprintln(tw, "CREATED\tOPERATOR\tID\tSCOPE\tTARGET\tSTATUS\tREASON")
	} else {
		fmt.Fprintln(tw, "CREATED\tID\tSCOPE\tTARGET\tSTATUS\tREASON")
	}
	for i := range list.Items {
		r := &list.Items[i]
		created := formatBreakglassTime(r.CreatedAt)
		reason := output.Dash(r.Reason)
		target := output.Truncate(breakglassTargetCluster(r.TargetCluster), 25)
		if wide {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				created,
				output.Dash(r.Operator),
				r.RequestID,
				r.Scope,
				target,
				r.Status,
				output.Dash(r.Approval),
				output.Truncate(reason, 40),
				output.Dash(r.ApprovedBy),
				formatBreakglassTime(r.GrantExpiresAt),
			)
			continue
		}
		if fleet {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				created,
				output.Dash(r.Operator),
				r.RequestID,
				r.Scope,
				target,
				r.Status,
				output.Truncate(reason, 30),
			)
		} else {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
				created,
				r.RequestID,
				r.Scope,
				target,
				r.Status,
				output.Truncate(reason, 30),
			)
		}
	}
	return tw.Flush()
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
