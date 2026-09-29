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
		Use:   "targets [deployment]",
		Short: "List boundary targets",
		Long: `List available boundary targets. Optionally filter by deployment name.

Targets represent MCs and RCs where boundary sessions can be started.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := getClient(opts)
			if err != nil {
				return fmt.Errorf("creating client: %w", err)
			}

			if len(args) == 1 {
				list, err := c.ListTargetsByDeployment(cmd.Context(), args[0])
				if err != nil {
					return fmt.Errorf("listing targets: %w", err)
				}

				if opts.OutputFormat == output.FormatJSON {
					enc := json.NewEncoder(os.Stdout)
					enc.SetIndent("", "  ")
					return enc.Encode(list)
				}

				if len(list.Items) == 0 {
					fmt.Printf("No targets found for deployment %q\n", args[0])
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
				fmt.Println("No targets found")
				return nil
			}

			tw := output.NewTable(os.Stdout)
			fmt.Fprintln(tw, "TARGET\tDEPLOYMENT\tTYPE\tREGION\tVPC\tSTATUS")
			for _, t := range list.Items {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
					t.TargetID, t.DeploymentName, t.TargetType, t.Region,
					output.Dash(t.VpcId), output.Dash(t.Status))
			}
			return tw.Flush()
		},
	}

	return cmd
}
