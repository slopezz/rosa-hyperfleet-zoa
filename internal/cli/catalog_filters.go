package cli

import "github.com/spf13/cobra"

func registerCatalogFilterFlags(cmd *cobra.Command, scope, typ *string) {
	cmd.Flags().StringVar(scope, "scope", "", "Filter by credential scope (kube, aws)")
	cmd.Flags().StringVar(typ, "type", "", "Filter by privilege type (read, write; elevated for break-glass profiles only)")
	_ = cmd.RegisterFlagCompletionFunc("scope", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"kube", "aws"}, cobra.ShellCompDirectiveNoFileComp
	})
	_ = cmd.RegisterFlagCompletionFunc("type", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"read", "write", "elevated"}, cobra.ShellCompDirectiveNoFileComp
	})
}