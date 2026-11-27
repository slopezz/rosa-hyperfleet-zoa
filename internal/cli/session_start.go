package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/cli/reason"
	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
	"github.com/openshift-online/rosa-seaplane-zoa/internal/ecsexec"
	"github.com/openshift-online/rosa-seaplane-zoa/internal/output"
)

func newSessionStartCommand(opts *GlobalOptions) *cobra.Command {
	var flagDeployment, flagTarget string
	var flagReason string
	var flagNoConnect bool
	var flagExecScript string
	var flagExecScriptTimeout time.Duration
	var flagExecScriptMaxOutput int

	cmd := &cobra.Command{
		Use:   "start [deployment] [target]",
		Short: "Start a boundary session",
		Long: `Start a boundary session for audited SRE access to a target cluster.

Positional args: <deployment> <target> (from zoa deployments and zoa targets <deployment>).
Resolves Access URL and invoker role from Central SSM unless --access-url and
--invoker-role-arn are set. Global --deployments-region pins the SSM region.

By default, Access provisions the boundary task on start, then the CLI connects
via ECS Exec (same as 'zoa session join'). Use --no-connect to provision the
task and print session metadata without opening Exec (same idea as
'zoa run --no-wait': dispatch, show id, do not attach).

ECS Exec uses scoped credentials returned by the Access API on join (not
deployment-account admin roles).

A reason is required (Jira issue or PagerDuty incident). It is stored
on the session, Access audit, and the boundary task (ZOA_REASON) for zoa run.

` + helpSessionExecScriptAutomation + `
On start, --exec-script provisions the task, joins, runs the script, and exits.`,
		Example: `  zoa session start us-east-1 regional --reason ROSAENG-1234

  zoa session start us-east-1 regional --reason ROSAENG-1234 --exec-script 'zoa actions | head -1'

  zoa session start us-east-1 mc01 --reason ROSAENG-1234 --no-connect -o json

  zoa session start us-east-1 mc01 --reason '#123456' --no-connect

  zoa session start -d us-east-1 -t mc01 --reason ROSAENG-1234 --no-connect -o json`,
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			deployment, target := resolveSessionStartArgs(args, flagDeployment, flagTarget)
			if deployment == "" {
				return fmt.Errorf("deployment is required: zoa session start <deployment> <target>")
			}
			if target == "" {
				return fmt.Errorf("target is required: zoa session start <deployment> <target>")
			}

			if opts.APIURL == "" {
				opts.Deployment = deployment
			}

			reasonValue, err := reason.Resolve(flagReason)
			if err != nil {
				return err
			}

			if flagExecScript != "" && (flagNoConnect || opts.OutputFormat == output.FormatJSON) {
				return fmt.Errorf("--exec-script requires an interactive connect (omit --no-connect and -o json)")
			}
			if flagExecScript != "" && flagExecScriptMaxOutput <= 0 {
				return fmt.Errorf("--exec-script-max-output must be positive")
			}

			c, err := getClient(opts)
			if err != nil {
				return fmt.Errorf("creating client: %w", err)
			}

			resp, err := c.SessionStart(cmd.Context(), &client.SessionStartRequest{
				DeploymentName: deployment,
				Target:         target,
				Reason:         reasonValue,
			})
			if err != nil {
				return fmt.Errorf("starting session: %w", err)
			}

			compoundID := FormatSessionID(deployment, resp.SessionID)
			displayTarget := sessionStartTarget(deployment, target, resp)

			if opts.OutputFormat != output.FormatJSON {
				printSessionDispatched(sessionProgressWriter(), compoundID, displayTarget)
			}

			connect := !flagNoConnect
			if connect && opts.OutputFormat == output.FormatJSON {
				connect = false
			}

			if connect {
				var joinResp *client.SessionJoinResponse
				err := runWithSpinner(cmd.Context(), "connecting to boundary", func(ctx context.Context) error {
					var joinErr error
					joinResp, joinErr = accessClientForJoin(c).SessionJoin(ctx, resp.SessionID)
					return joinErr
				})
				if err != nil {
					return fmt.Errorf("joining session after start: %w", err)
				}
				region := sessionJoinRegion(opts, joinResp)
				if flagExecScript != "" {
					if err := runSessionECSExecScript(cmd.Context(), deployment, region, resp.SessionID, accessClientForJoin(c), joinResp, flagExecScript, flagExecScriptTimeout, flagExecScriptMaxOutput); err != nil {
						return fmt.Errorf("ECS Exec script: %w", err)
					}
				} else if err := runSessionECSExec(cmd.Context(), deployment, region, resp.SessionID, accessClientForJoin(c), joinResp); err != nil {
					return fmt.Errorf("ECS Exec: %w", err)
				}
				printSessionExecEndedHint(sessionProgressWriter(), deployment, compoundID, joinDeadline(resp.Deadline, joinResp))
				return nil
			}

			if opts.OutputFormat == output.FormatJSON {
				result := map[string]interface{}{
					"session_id": compoundID,
					"raw_id":     resp.SessionID,
					"deployment": deployment,
					"target":     target,
					"status":     resp.Status,
				}
				if resp.TaskArn != "" {
					result["task_arn"] = resp.TaskArn
				}
				if resp.Region != "" {
					result["region"] = resp.Region
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			}

			printSessionJoinHint(sessionProgressWriter(), compoundID)
			return nil
		},
	}

	cmd.Flags().StringVarP(&flagDeployment, "deployment", "d", "", "Deployment name (e.g. us-east-1)")
	cmd.Flags().StringVarP(&flagTarget, "target", "t", "", "Target ID (e.g. mc01)")
	cmd.Flags().StringVar(&flagReason, "reason", "", "Reason for this session (required: Jira issue or PagerDuty incident, e.g. ROSAENG-1234 or #123456)")
	cmd.Flags().BoolVar(&flagNoConnect, "no-connect", false, "Provision boundary task and print session metadata; do not open ECS Exec")
	cmd.Flags().StringVar(&flagExecScript, "exec-script", "", "After connect, run a bash script inside the boundary shell via ECS Exec (runbooks, automation, e2e)")
	cmd.Flags().DurationVar(&flagExecScriptTimeout, "exec-script-timeout", 10*time.Minute, "Timeout for --exec-script")
	cmd.Flags().IntVar(&flagExecScriptMaxOutput, "exec-script-max-output", ecsexec.DefaultExecScriptMaxOutput, "Max bytes of --exec-script PTY output captured on the laptop (default 16 MiB; full transcript in CloudWatch)")

	return cmd
}

// resolveSessionStartArgs resolves deployment and target from positional args
// or flag fallbacks. Positional args take precedence over flags.
func resolveSessionStartArgs(args []string, flagDeployment, flagTarget string) (deployment, target string) {
	switch len(args) {
	case 2:
		return args[0], args[1]
	case 1:
		return args[0], flagTarget
	default:
		return flagDeployment, flagTarget
	}
}