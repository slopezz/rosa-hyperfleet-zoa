package cli

import (
	"context"
	"fmt"
	"net/url"
	"os"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/catalog"

	"github.com/spf13/cobra"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/cli/parambind"
	"github.com/openshift-online/rosa-seaplane-zoa/internal/cli/profilecatalog"
	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
	"github.com/openshift-online/rosa-seaplane-zoa/internal/output"
)

func newBreakglassProfilesCommand(global *GlobalOptions) *cobra.Command {
	catalogOpts := &catalogOptions{}
	var scopeFilter, typeFilter string
	cmd := &cobra.Command{
		Use:   "profiles",
		Short: "List break-glass profiles",
		Long: `List break-glass profiles. With --offline (or without ZOA_API_URL in the boundary), uses the
catalog embedded in this CLI build, filtered by --target-type or ZOA_TARGET_TYPE.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if global.OutputFormat == output.FormatMarkdown && !useProfileCatalogOffline(global, catalogOpts) {
				return fmt.Errorf("-o markdown requires --offline (embedded catalog from this CLI build)")
			}
			if useProfileCatalogOffline(global, catalogOpts) {
				return listBreakglassProfilesOffline(global, catalogOpts, scopeFilter, typeFilter)
			}
			c, err := getClient(global)
			if err != nil {
				return err
			}
			q := catalogFilterQuery(scopeFilter, typeFilter)
			list, err := c.ListBreakglassProfiles(cmd.Context(), q)
			if err != nil {
				return err
			}
			return renderBreakglassProfiles(global.OutputFormat, list)
		},
	}
	registerCatalogFlags(cmd, catalogOpts)
	registerCatalogFilterFlags(cmd, &scopeFilter, &typeFilter)
	return cmd
}

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

func useProfileCatalogOffline(global *GlobalOptions, catalogOpts *catalogOptions) bool {
	return catalogOpts.offline || global.APIURL == ""
}

func catalogFilterQuery(scopeFilter, typeFilter string) url.Values {
	q := make(url.Values)
	if scopeFilter != "" {
		q.Set("scope", scopeFilter)
	}
	if typeFilter != "" {
		q.Set("type", typeFilter)
	}
	return q
}

func listBreakglassProfilesOffline(global *GlobalOptions, catalogOpts *catalogOptions, scopeFilter, typeFilter string) error {
	printOfflineProfileCatalogNotice()
	targetType, err := resolveCatalogTargetType(catalogOpts)
	if err != nil {
		return err
	}
	if global.OutputFormat == output.FormatMarkdown {
		body, err := profilecatalog.Markdown(targetType)
		if err != nil {
			return err
		}
		fmt.Fprint(os.Stdout, body)
		return nil
	}
	items, err := profilecatalog.List(targetType)
	if err != nil {
		return err
	}
	filtered := items[:0]
	for _, item := range items {
		if !catalog.MatchCredentialScope(item.Scope, scopeFilter) {
			continue
		}
		if !catalog.MatchPrivilegeType(item.Type, typeFilter) {
			continue
		}
		filtered = append(filtered, item)
	}
	return renderBreakglassProfiles(global.OutputFormat, &client.BreakglassProfileList{
		Items: filtered,
		Count: len(filtered),
	})
}

func describeBreakglassProfile(ctx context.Context, global *GlobalOptions, catalogOpts *catalogOptions, profileName string) error {
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

func renderBreakglassProfiles(format output.Format, list *client.BreakglassProfileList) error {
	if list == nil {
		list = &client.BreakglassProfileList{}
	}
	if format == output.FormatJSON {
		return output.JSON(os.Stdout, list)
	}
	tw := output.NewTable(os.Stdout)
	fmt.Fprintf(tw, "NAME\tSCOPE\tTYPE\tAPPROVAL\tREQUEST_TTL\tGRANT_TTL\tCREDENTIAL_TTL\tDESCRIPTION\n")
	for _, s := range list.Items {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			s.Name, s.Scope, s.Type,
			output.Dash(profilecatalog.Approval(s)),
			output.Dash(profilecatalog.RequestTTL(s)),
			output.Dash(profilecatalog.GrantTTL(s)),
			output.Dash(profilecatalog.CredentialTTL(s)),
			s.Description)
	}
	return tw.Flush()
}