package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/cli/actioncatalog"
	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/cli/parambind"
	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/output"
)

func newActionsCommand(global *GlobalOptions) *cobra.Command {
	catalogOpts := &catalogOptions{}

	cmd := &cobra.Command{
		Use:     "actions [action]",
		Short:   "List available Trusted Actions",
		Aliases: []string{"catalog"},
		Long: `List or describe Trusted Actions.

By default, commands call the ZOA API (requires ZOA_API_URL). Use --offline to read the
action registry embedded in this CLI build, filtered by --target-type or ` + "`" + targettypeEnvInHelp() + "`" + ` (rc or mc, same as TYPE in zoa targets).

Offline output includes a notice on stderr; the live API is the source of truth when connected.`,
		Example: `  # List actions from the API (authoritative when connected)
  zoa actions

  # Offline catalog for RC (no API URL required)
  zoa actions --offline --target-type rc

  # Markdown catalog for Claude / boundary (target type from env in boundary tasks)
  zoa actions --offline -o markdown

  # Show details for a specific action (alias for describe)
  zoa actions get_resource

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
				return describeAction(cmd.Context(), global, catalogOpts, args[0])
			}
			return listActions(cmd.Context(), global, catalogOpts)
		},
	}
	registerCatalogFlags(cmd, catalogOpts)
	return cmd
}

func targettypeEnvInHelp() string {
	return "ZOA_TARGET_TYPE"
}

func newDescribeCommand(global *GlobalOptions) *cobra.Command {
	catalogOpts := &catalogOptions{}

	cmd := &cobra.Command{
		Use:   "describe <action>",
		Short: "Show Trusted Action details",
		Long: `Show Trusted Action parameters and CLI bindings.

By default, calls the ZOA API. With --offline, reads embedded registry metadata (see zoa actions --offline).`,
		Example: `  # Show action details (parameters, scope, approval requirements)
  zoa describe get_resource

  # Offline describe for RC
  zoa describe get_resource --offline --target-type rc

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
			return describeAction(cmd.Context(), global, catalogOpts, args[0])
		},
	}
	registerCatalogFlags(cmd, catalogOpts)
	return cmd
}

func listActions(ctx context.Context, global *GlobalOptions, catalogOpts *catalogOptions) error {
	if global.OutputFormat == output.FormatMarkdown && !catalogOpts.offline {
		return fmt.Errorf("-o markdown requires --offline (embedded catalog from this CLI build)")
	}
	if catalogOpts.offline {
		return listActionsOffline(global, catalogOpts)
	}
	return listActionsOnline(ctx, global)
}

func listActionsOnline(ctx context.Context, global *GlobalOptions) error {
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

func listActionsOffline(global *GlobalOptions, catalogOpts *catalogOptions) error {
	printOfflineCatalogNotice()
	targetType, err := resolveCatalogTargetType(catalogOpts)
	if err != nil {
		return err
	}
	if global.OutputFormat == output.FormatMarkdown {
		body, err := actioncatalog.Markdown(targetType)
		if err != nil {
			return err
		}
		fmt.Fprint(os.Stdout, body)
		return nil
	}

	items, err := actioncatalog.List(targetType)
	if err != nil {
		return err
	}

	if global.OutputFormat == output.FormatJSON {
		return output.JSON(os.Stdout, map[string]any{"items": items})
	}

	tw := output.NewTable(os.Stdout)
	fmt.Fprintf(tw, "NAME\tSCOPE\tTYPE\tMODE\tDESCRIPTION\n")
	for _, a := range items {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", a.Name, a.Scope, a.Type, output.Dash(a.ExecutionMode), a.Description)
	}
	return tw.Flush()
}

func describeAction(ctx context.Context, global *GlobalOptions, catalogOpts *catalogOptions, name string) error {
	if catalogOpts.offline {
		return describeActionOffline(global, catalogOpts, name)
	}
	return describeActionOnline(ctx, global, name)
}

func describeActionOnline(ctx context.Context, global *GlobalOptions, name string) error {
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

func describeActionOffline(global *GlobalOptions, catalogOpts *catalogOptions, name string) error {
	if global.OutputFormat == output.FormatMarkdown {
		return fmt.Errorf("--offline describe does not support -o markdown; use `zoa actions --offline -o markdown` for the full catalog")
	}
	printOfflineCatalogNotice()
	targetType, err := resolveCatalogTargetType(catalogOpts)
	if err != nil {
		return err
	}
	action, err := actioncatalog.Get(targetType, name)
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
