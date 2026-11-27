package cli

import (
	"fmt"
	"net/url"
	"os"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/catalog"

	"github.com/spf13/cobra"

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
