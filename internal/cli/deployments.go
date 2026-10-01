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
// deployment pointers (deployment_name → Access Function URL + invoker role).
// Written by RC Terraform into the Central Account, read directly by the CLI
// from the SRE's active credentials (Central Account).
const ssmDeploymentsPath = "/zoa/deployments"

// ssmDeployment represents a single deployment entry in SSM.
type ssmDeployment struct {
	DeploymentName string `json:"deployment_name"`
	AccessURL      string `json:"access_url"`
	InvokerRoleARN string `json:"invoker_role_arn"`
	Region         string `json:"region"`
	AccountID      string `json:"account_id"`
}

func newDeploymentsCommand(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deployments",
		Short: "List available ZOA deployments",
		Long: `List all ZOA deployments by reading SSM Parameter Store directly.

Uses your active AWS credentials (Central Account). No Access Lambda
or invoker role needed — this is a direct SSM read.

Each deployment is one regional ZOA installation. The name is HyperFleet
deployment_name (typically the AWS region, e.g. us-east-1). Ephemeral stacks
use the same pattern with an extra suffix; use the DEPLOYMENT column from
this command — not the regional cluster regional_id.`,
		Example: `  # List all deployments
  zoa deployments

  # JSON output
  zoa deployments -o json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
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
				fmt.Println("Hint: deployment names match HyperFleet deployment_name")
				fmt.Println("      (typically the AWS region, e.g. us-east-1 — not regional_id)")
				fmt.Printf("      (SSM path: %s)\n", ssmDeploymentsPath)
				return nil
			}

			tw := output.NewTable(os.Stdout)
			fmt.Fprintln(tw, "DEPLOYMENT\tREGION\tACCESS URL\tINVOKER ROLE")
			for _, d := range deployments {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
					d.DeploymentName,
					output.Dash(d.Region),
					output.Dash(d.AccessURL),
					output.Dash(d.InvokerRoleARN))
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
