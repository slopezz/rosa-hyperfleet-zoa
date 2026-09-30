package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/spf13/cobra"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/output"
)

// ssmDeploymentsPath is the SSM Parameter Store path that contains
// deployment pointers (deployment_name → APIGW URL). Written by each
// RC pipeline, read directly by the CLI from the SRE's active credentials
// (Central Account or RC for dev/ephemeral).
const ssmDeploymentsPath = "/zoa/deployments"

// ssmDeployment represents a single deployment entry in SSM.
type ssmDeployment struct {
	DeploymentName string `json:"deployment_name"`
	APIGWURL       string `json:"apigw_url"`
	Enabled        bool   `json:"enabled"`
}

func newTargetsCommand(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "targets [deployment]",
		Short: "List boundary targets",
		Long: `List available ZOA deployments and targets.

Without arguments, lists all available deployments by reading SSM Parameter
Store directly (uses your active AWS credentials — no ZOA_API_URL needed).

With a deployment argument, lists targets within that deployment by querying
the ZOA Access Lambda API Gateway (requires ZOA_API_URL or auto-resolves
from the deployment's APIGW URL).`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// With deployment arg: list targets within that deployment via Access Lambda.
			if len(args) == 1 {
				c, err := getClient(opts)
				if err != nil {
					return fmt.Errorf("creating client: %w", err)
				}

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

			// No args: list deployments from SSM directly (no Access Lambda needed).
			deployments, err := listDeploymentsFromSSM(cmd.Context())
			if err != nil {
				return fmt.Errorf("listing deployments from SSM: %w", err)
			}

			if opts.OutputFormat == output.FormatJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]interface{}{
					"items": deployments,
					"count": len(deployments),
				})
			}

			if len(deployments) == 0 {
				fmt.Println("No deployments found in SSM")
				fmt.Println()
				fmt.Println("Hint: ensure your AWS credentials point to the correct account")
				fmt.Printf("      (SSM path: %s)\n", ssmDeploymentsPath)
				return nil
			}

			tw := output.NewTable(os.Stdout)
			fmt.Fprintln(tw, "DEPLOYMENT\tAPGIW URL\tENABLED")
			for _, d := range deployments {
				fmt.Fprintf(tw, "%s\t%s\t%v\n",
					d.DeploymentName, d.APIGWURL, d.Enabled)
			}
			return tw.Flush()
		},
	}

	return cmd
}

// listDeploymentsFromSSM reads /zoa/deployments from SSM Parameter Store
// using the SRE's active AWS credentials (direct read, no Lambda involved).
func listDeploymentsFromSSM(ctx context.Context) ([]ssmDeployment, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}

	ssmClient := ssm.NewFromConfig(cfg)

	out, err := ssmClient.GetParametersByPath(ctx, &ssm.GetParametersByPathInput{
		Path:      aws.String(ssmDeploymentsPath + "/"),
		Recursive: aws.Bool(false),
	})
	if err != nil {
		return nil, fmt.Errorf("reading SSM %s: %w", ssmDeploymentsPath, err)
	}

	var deployments []ssmDeployment
	for _, param := range out.Parameters {
		var d ssmDeployment
		if err := json.Unmarshal([]byte(aws.ToString(param.Value)), &d); err != nil {
			return nil, fmt.Errorf("parsing SSM parameter %s: %w", aws.ToString(param.Name), err)
		}
		deployments = append(deployments, d)
	}
	return deployments, nil
}
