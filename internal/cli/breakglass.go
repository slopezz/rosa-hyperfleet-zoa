package cli

import (
	"github.com/spf13/cobra"
)

func newBreakglassCommand(global *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "breakglass",
		Short: "Break-glass emergency access (boundary)",
		Long: `Request, list, and activate break-glass grants from inside the boundary (ZOA_API_URL).

Laptop approvers use: zoa approve <deployment>/<request-id> [--comment ...]`,
	}
	cmd.AddCommand(
		newBreakglassProfilesCommand(global),
		newBreakglassDescribeProfileCommand(global),
		newBreakglassRequestCommand(global),
		newBreakglassRequestsCommand(global),
		newBreakglassHistoryCommand(global),
		newBreakglassGetCommand(global),
		newBreakglassActivateCommand(global),
	)
	return cmd
}

func newApproveCommand(global *GlobalOptions) *cobra.Command {
	var comment string
	cmd := &cobra.Command{
		Use:   "approve <deployment>/<request-id>",
		Short: "Approve a pending break-glass request (laptop / Access)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			global.Deployment = deploymentFromCompound(args[0])
			c, err := getClient(global)
			if err != nil {
				return err
			}
			return c.Approve(cmd.Context(), args[0], comment)
		},
	}
	cmd.Flags().StringVar(&comment, "comment", "", "Decision note")
	return cmd
}

func newRejectCommand(global *GlobalOptions) *cobra.Command {
	var comment string
	cmd := &cobra.Command{
		Use:   "reject <deployment>/<request-id>",
		Short: "Reject a pending break-glass request (laptop / Access)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			global.Deployment = deploymentFromCompound(args[0])
			c, err := getClient(global)
			if err != nil {
				return err
			}
			return c.Reject(cmd.Context(), args[0], comment)
		},
	}
	cmd.Flags().StringVar(&comment, "comment", "", "Decision note")
	return cmd
}

func deploymentFromCompound(compound string) string {
	dep, _, err := ParseSessionID(compound)
	if err != nil {
		return ""
	}
	return dep
}
