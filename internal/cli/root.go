package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/spf13/cobra"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/client"
	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/output"
	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/version"
)

type GlobalOptions struct {
	APIURL       string
	Region       string
	OutputFormat output.Format

	// ClientFactory overrides client creation for testing.
	// When nil, the real AWS-authenticated client is used.
	ClientFactory func(*GlobalOptions) (APIClient, error)
}

func NewRootCommand() *cobra.Command {
	opts := &GlobalOptions{}

	cmd := &cobra.Command{
		Use:   "zoa",
		Short: "ZOA — Zero Operator Access CLI",
		Long: `ZOA executes audited Trusted Actions against target clusters via the Platform API.

All operations are authenticated with AWS SigV4 using your current credentials.
Set ZOA_API_URL to your ZOA endpoint (Function URL, API Gateway, or CNAME).`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			name := cmd.Name()
			if name == "version" || name == "completion" || name == "help" {
				return nil
			}
			if cmd.CalledAs() == "__complete" || cmd.CalledAs() == "__completeNoDesc" {
				return nil
			}
			if opts.APIURL == "" {
				return fmt.Errorf("ZOA_API_URL not set\n\n  export ZOA_API_URL=\"https://<id>.lambda-url.<region>.on.aws\"")
			}
			return nil
		},
	}

	cmd.PersistentFlags().StringVar(&opts.APIURL, "api-url", os.Getenv("ZOA_API_URL"), "ZOA endpoint URL: Function URL, API Gateway, or CNAME (env: ZOA_API_URL)")
	cmd.PersistentFlags().StringVar(&opts.Region, "region", os.Getenv("AWS_REGION"), "AWS region override for custom CNAME endpoints (env: AWS_REGION)")
	var outputFlag string
	cmd.PersistentFlags().StringVarP(&outputFlag, "output", "o", "table", "Output format: table, wide, json")
	cobra.OnInitialize(func() {
		opts.OutputFormat = output.ParseFormat(outputFlag)
	})

	cmd.AddCommand(
		newRunCommand(opts),
		newGetCommand(opts),
		newOutputCommand(opts),
		newLogsCommand(opts),
		newDownloadCommand(opts),
		newRunsCommand(opts),
		newActionsCommand(opts),
		newDescribeCommand(opts),
		newAuditCommand(opts),
		newTargetsCommand(opts),
		newSessionCommand(opts),
		newVersionCommand(opts),
		newCompletionCommand(),
	)

	return cmd
}

func getClient(opts *GlobalOptions) (APIClient, error) {
	if opts.ClientFactory != nil {
		return opts.ClientFactory(opts)
	}
	return newRealClient(opts)
}

func newRealClient(opts *GlobalOptions) (*client.Client, error) {
	ctx := context.Background()
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}

	stsClient := sts.NewFromConfig(cfg)
	identity, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return nil, fmt.Errorf("getting caller identity: %w", err)
	}

	return client.New(opts.APIURL, cfg.Credentials, client.Options{
		AccountID: *identity.Account,
		Operator:  *identity.Arn,
		Region:    opts.Region,
	})
}

func newVersionCommand(global *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print client and server version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			clientInfo := version.Get()

			if global.OutputFormat == output.FormatJSON {
				result := map[string]interface{}{
					"client": clientInfo,
				}
				if global.APIURL != "" {
					c, err := getClient(global)
					if err == nil {
						if sv, err := c.ServerVersion(cmd.Context()); err == nil {
							result["server"] = sv
						} else {
							result["server_error"] = err.Error()
						}
					}
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			}

			fmt.Printf("Client: %s\n", clientInfo.String())

			if global.APIURL == "" {
				fmt.Println("Server: (not configured — set ZOA_API_URL)")
				return nil
			}

			c, err := getClient(global)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Server: error creating client: %v\n", err)
				return nil
			}
			sv, err := c.ServerVersion(cmd.Context())
			if err != nil {
				fmt.Fprintf(os.Stderr, "Server: unreachable (%v)\n", err)
				return nil
			}
			fmt.Printf("Server: zoa %s (commit: %s, built: %s, %s, %s)\n",
				sv.Version, sv.GitCommit, sv.BuildDate, sv.GoVersion, sv.Platform)
			fmt.Printf("Target: %s\n", sv.Target)
			return nil
		},
	}
	return cmd
}

func newCompletionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish]",
		Short: "Generate shell completion scripts (usage: zoa completion [bash|zsh|fish])",
		Long: `Generate shell completion scripts for zoa.

To load completions:

  # bash (current session)
  source <(zoa completion bash)

  # bash (persistent — add to ~/.bashrc)
  zoa completion bash > /etc/bash_completion.d/zoa

  # zsh (current session)
  source <(zoa completion zsh)

  # zsh (persistent — place in $fpath)
  zoa completion zsh > "${fpath[1]}/_zoa"

  # fish
  zoa completion fish | source`,
		ValidArgs:             []string{"bash", "zsh", "fish"},
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletion(os.Stdout)
			case "zsh":
				return cmd.Root().GenZshCompletion(os.Stdout)
			case "fish":
				return cmd.Root().GenFishCompletion(os.Stdout, true)
			default:
				return fmt.Errorf("unsupported shell: %s (valid: bash, zsh, fish)", args[0])
			}
		},
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return []string{"bash", "zsh", "fish"}, cobra.ShellCompDirectiveNoFileComp
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
	}
	cmd.Args = cobra.MaximumNArgs(1)
	return cmd
}
