package cli

import (
	"github.com/spf13/cobra"
)

func newBreakglassHistoryCommand(global *GlobalOptions) *cobra.Command {
	var since, until, operator, status, approval, decidedBy, decidedSince, target, profile string
	var limit int
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Break-glass requests across all operators",
		Long: `Fleet-wide break-glass history on this deployment (RC + MC). Audited on the API (like zoa session history).

Includes an OPERATOR column; does not set mine=true. Optional --target filters one cluster (same as zoa runs).`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return listBreakglassRequests(cmd.Context(), global, breakglassListOpts{
				fleetView:    true,
				target:       target,
				profile:      profile,
				operator:     operator,
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
	cmd.Flags().StringVar(&operator, "operator", "", "Filter by operator")
	cmd.Flags().StringVar(&status, "status", "", "Filter by lifecycle status")
	cmd.Flags().StringVar(&approval, "approval", "", "Filter by approval outcome")
	cmd.Flags().StringVar(&decidedBy, "decided-by", "", "Filter by approver/rejecter")
	cmd.Flags().StringVar(&decidedSince, "decided-since", "", "Only rows decided on/after this time")
	cmd.Flags().IntVar(&limit, "limit", 50, "Max results (max 200)")
	return cmd
}
