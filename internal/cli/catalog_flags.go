package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/cli/actioncatalog"
	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/cli/targettype"
)

type catalogOptions struct {
	offline    bool
	targetType string
}

func registerCatalogFlags(cmd *cobra.Command, opts *catalogOptions) {
	cmd.Flags().BoolVar(&opts.offline, "offline", false,
		"Use the action registry embedded in this CLI (no API call). Live API is authoritative when connected.")
	cmd.Flags().StringVar(&opts.targetType, "target-type", "",
		fmt.Sprintf("Target type rc or mc (TYPE in `zoa targets`; env: %s)", targettype.EnvVar))
	_ = cmd.RegisterFlagCompletionFunc("target-type", func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		var out []string
		for _, v := range []string{targettype.RC, targettype.MC} {
			if toComplete == "" || (len(v) >= len(toComplete) && v[:len(toComplete)] == toComplete) {
				out = append(out, v)
			}
		}
		return out, cobra.ShellCompDirectiveNoFileComp
	})
}

func catalogCommandUsesOffline(cmd *cobra.Command) bool {
	if cmd == nil || cmd.Flags().Lookup("offline") == nil {
		return false
	}
	offline, err := cmd.Flags().GetBool("offline")
	return err == nil && offline
}

func printOfflineCatalogNotice() {
	fmt.Fprintln(os.Stderr, actioncatalog.OfflineNotice)
}

func resolveCatalogTargetType(opts *catalogOptions) (string, error) {
	return targettype.Resolve(opts.targetType)
}
