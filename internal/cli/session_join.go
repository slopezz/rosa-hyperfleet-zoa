package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
	"github.com/openshift-online/rosa-seaplane-zoa/internal/ecsexec"
	"github.com/openshift-online/rosa-seaplane-zoa/internal/output"
)

func newSessionJoinCommand(opts *GlobalOptions) *cobra.Command {
	var flagExecScript string
	var flagExecScriptTimeout time.Duration
	var flagExecScriptMaxOutput int

	cmd := &cobra.Command{
		Use:   "join <deployment/session-id>",
		Short: "Join a boundary session via ECS Exec",
		Long: `Join a boundary session by its compound ID (deployment/session-id).

Calls the Access API for ownership checks, then ECS Exec via session-manager-plugin
using scoped credentials from the join response. Default: interactive shell.

` + helpSessionExecScriptAutomation + `
On join, --exec-script runs the script on an existing session (often after 'session start --no-connect -o json').`,
		Example: `  zoa session join us-east-1/1989937d-0889-485d-b64e-facee9e0150d

  zoa session join us-east-1/1989937d-0889-485d-b64e-facee9e0150d --exec-script 'zoa actions | head -1'

  SESSION_ID=$(zoa session start us-east-1 mc01 --reason ROSAENG-1234 --no-connect -o json | jq -r .session_id)
  zoa session join "$SESSION_ID" --exec-script 'zoa actions | head -1'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deployment, rawID, err := ParseSessionID(args[0])
			if err != nil {
				return err
			}

			if opts.APIURL == "" {
				opts.Deployment = deployment
			}

			if flagExecScript != "" && opts.OutputFormat == output.FormatJSON {
				return fmt.Errorf("--exec-script cannot be used with -o json")
			}
			if flagExecScript != "" && flagExecScriptMaxOutput <= 0 {
				return fmt.Errorf("--exec-script-max-output must be positive")
			}

			c, err := getClient(opts)
			if err != nil {
				return fmt.Errorf("creating client: %w", err)
			}

			resp, err := c.SessionJoin(cmd.Context(), rawID)
			if err != nil {
				return fmt.Errorf("joining session: %w", err)
			}

			if opts.OutputFormat == output.FormatJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(resp)
			}

			region := sessionJoinRegion(opts, resp)
			compoundID := FormatSessionID(deployment, rawID)
			if flagExecScript != "" {
				if err := runSessionECSExecScript(cmd.Context(), deployment, region, rawID, accessClientForJoin(c), resp, flagExecScript, flagExecScriptTimeout, flagExecScriptMaxOutput); err != nil {
					return fmt.Errorf("ECS Exec script: %w", err)
				}
				printSessionExecEndedHint(sessionProgressWriter(), deployment, compoundID, resp.Deadline)
				return nil
			}
			if err := runSessionECSExec(cmd.Context(), deployment, region, rawID, accessClientForJoin(c), resp); err != nil {
				return fmt.Errorf("ECS Exec: %w", err)
			}
			printSessionExecEndedHint(sessionProgressWriter(), deployment, compoundID, resp.Deadline)
			return nil
		},
	}

	cmd.Flags().StringVar(&flagExecScript, "exec-script", "", "Run a bash script inside the boundary shell via ECS Exec")
	cmd.Flags().DurationVar(&flagExecScriptTimeout, "exec-script-timeout", 10*time.Minute, "Timeout for --exec-script")
	cmd.Flags().IntVar(&flagExecScriptMaxOutput, "exec-script-max-output", ecsexec.DefaultExecScriptMaxOutput, "Max bytes of --exec-script PTY output captured on the laptop (default 16 MiB; full transcript in CloudWatch)")

	return cmd
}

func joinDeadline(startDeadline string, join *client.SessionJoinResponse) string {
	if join != nil && join.Deadline != "" {
		return join.Deadline
	}
	return startDeadline
}