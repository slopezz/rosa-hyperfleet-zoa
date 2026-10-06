package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/spf13/cobra"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/accessclient"
	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/client"
	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/output"
	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/version"
)

type GlobalOptions struct {
	APIURL       string
	Region       string
	OutputFormat output.Format

	// Deployment is set by individual commands (targets positional arg,
	// session --deployment flag) to enable auto-resolution of the Access
	// URL and invoker role from SSM. Not a user-facing global flag.
	Deployment string

	// ClientFactory overrides client creation for testing.
	// When nil, the real AWS-authenticated client is used.
	ClientFactory func(*GlobalOptions) (APIClient, error)
}

const (
	cmdGroupDiscovery = "discovery"
	cmdGroupBoundary  = "boundary"
	cmdGroupTA        = "trusted-actions"
	cmdGroupAudit     = "audit"
	cmdGroupOther     = "other"
)

func NewRootCommand() *cobra.Command {
	opts := &GlobalOptions{}

	cmd := &cobra.Command{
		Use:   "zoa",
		Short: "ZOA — Zero Operator Access CLI",
		Long: `ZOA (Zero Operator Access) — audited SRE operations on HyperFleet clusters.

Typical workflow (boundary):
  zoa deployments                           List deployments (central SSM; no API URL)
  zoa targets us-east-1                     List RC/MC targets (use your deployment name)
  zoa session start us-east-1 mc01          Start a time-boxed session for audited SRE access

Trusted Actions (per-target API Lambda in each VPC):
  export ZOA_API_URL="https://..."     Function URL for the target you are operating on
  zoa run <action> ...                 Execute a Trusted Action

Session and discovery commands resolve the Access Lambda from the deployment name
via SSM and assume the invoker role automatically. TA commands use ZOA_API_URL.

Boundary session/join: use your normal Jump/Central AWS login for the Access API (invoker role).
ECS Exec uses scoped credentials returned by Access on join — not deployment-account admin roles.
Requires session-manager-plugin.

All requests start from your default AWS credential chain (SigV4).`,
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
			if catalogCommandUsesOffline(cmd) {
				return nil
			}
			// `zoa deployments` reads SSM directly — no API URL needed.
			if name == "deployments" {
				return nil
			}
			// `zoa targets <deployment>` auto-resolves from SSM via positional arg.
			if name == "targets" {
				return nil
			}
			// Session subcommands resolve deployment from positional args or
			// compound session IDs. Let getClient decide at call time.
			if cmd.Parent() != nil && cmd.Parent().Name() == "session" {
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

	cmd.AddGroup(
		&cobra.Group{ID: cmdGroupDiscovery, Title: "Discovery:"},
		&cobra.Group{ID: cmdGroupBoundary, Title: "Boundary access:"},
		&cobra.Group{ID: cmdGroupTA, Title: "Trusted Actions:"},
		&cobra.Group{ID: cmdGroupAudit, Title: "Audit:"},
		&cobra.Group{ID: cmdGroupOther, Title: "Other:"},
	)

	deploymentsCmd := newDeploymentsCommand(opts)
	deploymentsCmd.GroupID = cmdGroupDiscovery

	targetsCmd := newTargetsCommand(opts)
	targetsCmd.GroupID = cmdGroupDiscovery

	sessionCmd := newSessionCommand(opts)
	sessionCmd.GroupID = cmdGroupBoundary

	runCmd := newRunCommand(opts)
	runCmd.GroupID = cmdGroupTA

	getCmd := newGetCommand(opts)
	getCmd.GroupID = cmdGroupTA

	outputCmd := newOutputCommand(opts)
	outputCmd.GroupID = cmdGroupTA

	logsCmd := newLogsCommand(opts)
	logsCmd.GroupID = cmdGroupTA

	downloadCmd := newDownloadCommand(opts)
	downloadCmd.GroupID = cmdGroupTA

	runsCmd := newRunsCommand(opts)
	runsCmd.GroupID = cmdGroupTA

	actionsCmd := newActionsCommand(opts)
	actionsCmd.GroupID = cmdGroupTA

	describeCmd := newDescribeCommand(opts)
	describeCmd.GroupID = cmdGroupTA

	auditCmd := newAuditCommand(opts)
	auditCmd.GroupID = cmdGroupAudit

	versionCmd := newVersionCommand(opts)
	versionCmd.GroupID = cmdGroupOther

	completionCmd := newCompletionCommand()
	completionCmd.GroupID = cmdGroupOther

	cmd.AddCommand(
		deploymentsCmd,
		targetsCmd,
		sessionCmd,
		runCmd,
		getCmd,
		outputCmd,
		logsCmd,
		downloadCmd,
		runsCmd,
		actionsCmd,
		describeCmd,
		auditCmd,
		versionCmd,
		completionCmd,
	)

	return cmd
}

func getClient(opts *GlobalOptions) (APIClient, error) {
	if opts.ClientFactory != nil {
		return opts.ClientFactory(opts)
	}
	// When ZOA_API_URL is set, use it directly with the caller's credentials.
	if opts.APIURL != "" {
		return newRealClient(opts)
	}
	// Otherwise, auto-resolve from the deployment SSM entry: read the Access URL
	// and invoker role, assume the role transparently, and create the client.
	if opts.Deployment != "" {
		return newDeploymentClient(opts)
	}
	return nil, fmt.Errorf("ZOA_API_URL not set and no --deployment provided")
}

// newRealClient creates a client using the caller's existing credentials
// and the explicit API URL.
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

// newDeploymentClient auto-resolves the Access URL and invoker role from SSM,
// assumes the invoker role transparently (preserving the SRE's identity as the
// session name), and creates a client pointing at the Access Lambda.
func newDeploymentClient(opts *GlobalOptions) (*client.Client, error) {
	ctx := context.Background()

	info, err := accessclient.Resolve(ctx, opts.Deployment)
	if err != nil {
		return nil, fmt.Errorf("resolving deployment %q: %w", opts.Deployment, err)
	}

	region := opts.Region
	if region == "" {
		region = info.Region
	}

	operatorARN := info.OperatorARN
	if operatorARN == "" {
		return nil, fmt.Errorf("deployment %q: missing operator ARN after invoker assume", opts.Deployment)
	}

	return client.New(info.AccessURL, info.Credentials, client.Options{
		AccountID: info.AccountID,
		Operator:  operatorARN,
		Region:    region,
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
