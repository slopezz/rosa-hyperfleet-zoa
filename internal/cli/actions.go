package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/cli/parambind"
	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/output"
)

func newActionsCommand(global *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "actions [action]",
		Short:   "List available Trusted Actions",
		Aliases: []string{"catalog"},
		Example: `  # List all available Trusted Actions
  zoa actions

  # Show details for a specific action (alias for describe)
  zoa actions get_pods

  # Output as JSON (for scripting)
  zoa actions -o json`,
		Args: cobra.MaximumNArgs(1),
		ValidArgsFunction: func(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return completeActionNames(toComplete), cobra.ShellCompDirectiveNoFileComp
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				return describeAction(cmd.Context(), global, args[0])
			}
			return listActions(cmd.Context(), global)
		},
	}
	return cmd
}

func newDescribeCommand(global *GlobalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "describe <action>",
		Short: "Show Trusted Action details",
		Example: `  # Show action details (parameters, scope, approval requirements)
  zoa describe get_pods

  # Output as JSON
  zoa describe rollout_restart -o json`,
		Args: cobra.ExactArgs(1),
		ValidArgsFunction: func(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return completeActionNames(toComplete), cobra.ShellCompDirectiveNoFileComp
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return describeAction(cmd.Context(), global, args[0])
		},
	}
}

func listActions(ctx context.Context, global *GlobalOptions) error {
	c, err := getClient(global)
	if err != nil {
		return err
	}

	list, err := c.ListActions(ctx)
	if err != nil {
		return err
	}

	if global.OutputFormat == output.FormatJSON {
		return output.JSON(os.Stdout, list)
	}

	tw := output.NewTable(os.Stdout)
	fmt.Fprintf(tw, "NAME\tSCOPE\tTYPE\tMODE\tDESCRIPTION\n")
	for _, a := range list.Items {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", a.Name, a.Scope, a.Type, output.Dash(a.ExecutionMode), a.Description)
	}
	return tw.Flush()
}

func describeAction(ctx context.Context, global *GlobalOptions, name string) error {
	c, err := getClient(global)
	if err != nil {
		return err
	}

	action, err := c.GetAction(ctx, name)
	if err != nil {
		return err
	}

	if global.OutputFormat == output.FormatJSON {
		view := parambind.EnrichDescribe(action)
		return output.JSON(os.Stdout, view)
	}

	printDescribeHuman(action, os.Stdout)
	return nil
}
