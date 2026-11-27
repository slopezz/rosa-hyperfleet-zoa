package cli

import (
	"github.com/spf13/cobra"
)

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
			return listSessions(cmd.Context(), opts, sessionListOpts{
				deployment: args[0],
				fleetView:  true,
				since:      since,
				until:      until,
				operator:   operator,
				target:     target,
				status:     status,
				limit:      limit,
			})
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