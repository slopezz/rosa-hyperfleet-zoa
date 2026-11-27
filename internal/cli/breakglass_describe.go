package cli

import (
	"context"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/cli/parambind"
	"github.com/openshift-online/rosa-seaplane-zoa/internal/cli/profilecatalog"
	"github.com/openshift-online/rosa-seaplane-zoa/internal/output"
)

func newBreakglassDescribeProfileCommand(global *GlobalOptions) *cobra.Command {
	catalogOpts := &catalogOptions{}
	cmd := &cobra.Command{
		Use:   "describe <profile>",
		Short: "Describe a break-glass profile",
		Long: `Show profile capabilities, restrictions, and effective RBAC or IAM notes.

By default, calls the ZOA API. With --offline, reads embedded registry metadata (see zoa breakglass profiles --offline).`,
		Example: `  zoa breakglass describe kube_read

  zoa breakglass describe kube_elevated --target-type mc

  zoa breakglass describe aws_write -o json`,
		Args: cobra.ExactArgs(1),
		ValidArgsFunction: func(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return completeProfileNames(toComplete), cobra.ShellCompDirectiveNoFileComp
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return describeBreakglassProfile(cmd.Context(), global, catalogOpts, args[0])
		},
	}
	registerCatalogFlags(cmd, catalogOpts)
	return cmd
}

func describeBreakglassProfile(ctx context.Context, global *GlobalOptions, catalogOpts *catalogOptions, profileName string) error {
	if global.OutputFormat == output.FormatMarkdown {
		return errMarkdownCatalogOnly("zoa breakglass profiles --offline -o markdown")
	}
	if useProfileCatalogOffline(global, catalogOpts) {
		printOfflineProfileCatalogNotice()
		targetType, err := resolveCatalogTargetType(catalogOpts)
		if err != nil {
			return err
		}
		profile, err := profilecatalog.Get(targetType, profileName)
		if err != nil {
			return err
		}
		if global.OutputFormat == output.FormatJSON {
			return output.JSON(os.Stdout, parambind.EnrichProfileDescribe(profile))
		}
		profilecatalog.FormatDescribeHuman(os.Stdout, profile)
		return nil
	}
	c, err := getClient(global)
	if err != nil {
		return err
	}
	profile, err := c.GetBreakglassProfile(ctx, profileName)
	if err != nil {
		return err
	}
	if global.OutputFormat == output.FormatJSON {
		return output.JSON(os.Stdout, parambind.EnrichProfileDescribe(profile))
	}
	profilecatalog.FormatDescribeHuman(os.Stdout, profile)
	return nil
}
