package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/output"
)

func newTargetsCommand(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "targets <deployment>",
		Short: "List targets within a deployment",
		Long: `List the targets (RC and MC clusters) available in a given deployment.

Requires a deployment name as a positional argument. The CLI auto-resolves
the Access Lambda URL and invoker role from SSM, then queries the Access
Lambda for target details.

Use 'zoa deployments' to discover available deployment names.`,
		Example: `  # List targets in a deployment
  zoa targets us-east-1

  # List targets in an ephemeral deployment
  zoa targets us-east-1-eph-f8d5483c

  # JSON output
  zoa targets us-east-1 -o json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deployment := args[0]
			if opts.APIURL == "" {
				opts.Deployment = deployment
			}

			c, err := getClient(opts)
			if err != nil {
				return fmt.Errorf("creating client: %w", err)
			}

			list, err := c.ListTargets(cmd.Context())
			if err != nil {
				return fmt.Errorf("listing targets: %w", err)
			}

			if opts.OutputFormat == output.FormatJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(list)
			}

			if len(list.Items) == 0 {
				fmt.Printf("No targets found for deployment %q\n", deployment)
				return nil
			}

			tw := output.NewTable(os.Stdout)
			fmt.Fprintln(tw, "TARGET\tTYPE\tREGION\tVPC\tSTATUS")
			for _, t := range list.Items {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
					t.TargetID, t.TargetType, t.Region,
					output.Dash(t.VpcId), output.Dash(t.Status))
			}
			return tw.Flush()
		},
	}

	return cmd
}
