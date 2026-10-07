package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/client"
	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/output"
)

func newGetCommand(global *GlobalOptions) *cobra.Command {
	var includeOutput, includeLogs, includeAll, wait bool
	var waitTimeout, waitPollInterval time.Duration

	cmd := &cobra.Command{
		Use:   "get <execution-id>",
		Short: "Get execution details",
		Example: `  # Get execution status and metadata
  zoa get fa65418c-f4eb-4f5c-8314-baaeb695ba7d

  # Include the TA output
  zoa get fa65418c-f4eb-4f5c-8314-baaeb695ba7d --include-output

  # Wait for a running execution to finish, then show output
  zoa get fa65418c-f4eb-4f5c-8314-baaeb695ba7d --wait --include-output

  # Reconnect after a dropped connection
  zoa get fa65418c-f4eb-4f5c-8314-baaeb695ba7d --wait --wait-timeout 3m --include-all

  # JSON output for scripting
  zoa get fa65418c-f4eb-4f5c-8314-baaeb695ba7d -o json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if includeAll {
				includeOutput = true
				includeLogs = true
			}
			return getExecution(cmd.Context(), global, args[0], getOpts{
				includeOutput:    includeOutput,
				includeLogs:      includeLogs,
				wait:             wait,
				waitTimeout:      waitTimeout,
				waitPollInterval: waitPollInterval,
			})
		},
	}

	cmd.Flags().BoolVar(&includeOutput, "include-output", false, "Include execution output")
	cmd.Flags().BoolVar(&includeLogs, "include-logs", false, "Include execution logs")
	cmd.Flags().BoolVar(&includeAll, "include-all", false, "Include output + logs")
	cmd.Flags().BoolVar(&wait, "wait", false, "Poll until execution reaches terminal status (useful to reconnect)")
	cmd.Flags().DurationVar(&waitTimeout, "wait-timeout", 5*time.Minute, "Max poll duration when --wait is active")
	cmd.Flags().DurationVar(&waitPollInterval, "wait-poll-interval", 30*time.Second, "Poll frequency when --wait is active")

	return cmd
}

type getOpts struct {
	includeOutput    bool
	includeLogs      bool
	wait             bool
	waitTimeout      time.Duration
	waitPollInterval time.Duration
}

func getExecution(ctx context.Context, global *GlobalOptions, id string, opts getOpts) error {
	c, err := getClient(global)
	if err != nil {
		return err
	}

	exec, err := c.GetExecution(ctx, id, "")
	if err != nil {
		return err
	}

	if opts.wait && !isTerminalStatus(exec.Status) {
		result, pollErr := poll(ctx, c, id, pollConfig{
			interval: opts.waitPollInterval,
			timeout:  opts.waitTimeout,
		})
		if pollErr != nil {
			return pollErr
		}
		exec = result
	}

	include := ""
	if opts.includeOutput && opts.includeLogs {
		include = "output,logs"
	} else if opts.includeOutput {
		include = "output"
	} else if opts.includeLogs {
		include = "logs"
	}
	if include != "" && isTerminalStatus(exec.Status) {
		full, fullErr := c.GetExecution(ctx, id, include)
		if fullErr == nil {
			exec = full
		}
	}

	// Auto-download if server returned a hint instead of actual output
	if opts.includeOutput && hasDownloadableArtifact(exec) {
		outPath, nbytes, err := autoDownloadOutput(ctx, c, id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Auto-download failed: %v (use 'zoa download %s' manually)\n", err, id)
		} else {
			exec.Output = "" // Clear hint so renderExecution doesn't print it
			printSavedArtifact("output", nbytes, outPath)
		}
	}

	return renderExecution(global, exec, opts)
}

// hasDownloadableArtifact returns true when the execution produced a binary
// artifact (tar.gz) that must be retrieved via 'zoa download' instead of
// being displayed inline.
func hasDownloadableArtifact(exec *client.Execution) bool {
	return exec.OutputFormat == "tar.gz"
}

func renderExecution(global *GlobalOptions, exec *client.Execution, opts getOpts) error {
	if global.OutputFormat == output.FormatJSON {
		return output.JSON(os.Stdout, exec)
	}

	fmt.Printf("ID:        %s\n", exec.ID)
	fmt.Printf("ACTION:    %s\n", exec.Action)
	if exec.RequestedAction != "" {
		fmt.Printf("REQUESTED: %s\n", exec.RequestedAction)
	}
	fmt.Printf("TARGET:    %s\n", exec.TargetCluster)
	fmt.Printf("STATUS:    %s\n", exec.Status)
	fmt.Printf("MODE:      %s\n", output.Dash(exec.ExecutionMode))
	fmt.Printf("DRY-RUN:   %s\n", output.FormatBool(exec.DryRun))
	fmt.Printf("FORCE:     %s\n", output.FormatBool(exec.Force))
	fmt.Printf("JIRA:      %s\n", output.Dash(exec.Jira))
	fmt.Printf("OPERATOR:   %s\n", exec.Operator)
	if exec.SignerARN != "" {
		fmt.Printf("SIGNER_ARN: %s\n", exec.SignerARN)
	}
	if exec.SessionID != "" {
		fmt.Printf("SESSION_ID: %s\n", exec.SessionID)
	}
	fmt.Printf("REVISION:   %s\n", output.Dash(exec.Revision))
	if len(exec.Params) > 0 {
		parts := make([]string, 0, len(exec.Params))
		for k, v := range exec.Params {
			parts = append(parts, k+"="+v)
		}
		fmt.Printf("PARAMS:    %s\n", strings.Join(parts, " "))
	} else {
		fmt.Printf("PARAMS:    -\n")
	}
	fmt.Printf("CREATED:   %s\n", output.FormatTimestamp(exec.CreatedAt))
	fmt.Printf("COMPLETED: %s\n", output.FormatTimestamp(exec.CompletedAt))
	fmt.Printf("DURATION:  %s\n", output.FormatDuration(exec.DurationMs))

	if opts.includeOutput && exec.Output.String() != "" {
		fmt.Println("---")
		output.PrintTAOutput(os.Stdout, exec.Output.String())
	}
	if opts.includeLogs && exec.Logs != "" {
		fmt.Println("---")
		fmt.Print(exec.Logs)
	}

	return nil
}
