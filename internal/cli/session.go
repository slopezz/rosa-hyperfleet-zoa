package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/client"
	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/output"
)

func newSessionCommand(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session",
		Short: "Manage boundary sessions",
		Long: `Start, stop, join, and list boundary sessions for audited SRE access.

Session IDs use the form <deployment>/<session-id> (see 'zoa session start').`,
	}

	cmd.AddCommand(
		newSessionStartCommand(opts),
		newSessionStopCommand(opts),
		newSessionJoinCommand(opts),
		newSessionListCommand(opts),
		newSessionHistoryCommand(opts),
	)

	return cmd
}

func newSessionStartCommand(opts *GlobalOptions) *cobra.Command {
	var flagDeployment, flagTarget string
	var flagConnect, flagNoConnect bool

	cmd := &cobra.Command{
		Use:   "start [deployment] [target]",
		Short: "Start a boundary session",
		Long: `Start a boundary session for audited SRE access to a target cluster.

Positional args: <deployment> <target>. Also available as flags for scripts.
The CLI auto-resolves the Access Lambda URL and invoker role from SSM.

By default, after the task is active the CLI connects via ECS Exec (same as
'zoa session join'). Use --no-connect to only print the session ID.

ECS Exec uses credentials from ZOA_EXEC_AWS_PROFILE or your default AWS chain
(regional account with ecs:ExecuteCommand), not the Access invoker role.`,
		Example: `  zoa session start us-east-1 mc01

  zoa session start -d us-east-1 -t mc01 --no-connect`,
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			deployment, target := resolveDeploymentTarget(args, flagDeployment, flagTarget)
			if deployment == "" {
				return fmt.Errorf("deployment is required: zoa session start <deployment> <target>")
			}
			if target == "" {
				return fmt.Errorf("target is required: zoa session start <deployment> <target>")
			}

			if opts.APIURL == "" {
				opts.Deployment = deployment
			}

			c, err := getClient(opts)
			if err != nil {
				return fmt.Errorf("creating client: %w", err)
			}

			resp, err := c.SessionStart(cmd.Context(), &client.SessionStartRequest{
				DeploymentName: deployment,
				Target:         target,
			})
			if err != nil {
				return fmt.Errorf("starting session: %w", err)
			}

			compoundID := FormatSessionID(deployment, resp.SessionID)

			connect := flagConnect && !flagNoConnect
			if connect && opts.OutputFormat == output.FormatJSON {
				connect = false
			}

			if connect {
				joinResp, err := c.SessionJoin(cmd.Context(), resp.SessionID)
				if err != nil {
					return fmt.Errorf("joining session after start: %w", err)
				}
				region := sessionJoinRegion(opts, joinResp)
				if err := runSessionECSExec(cmd.Context(), region, joinResp); err != nil {
					return fmt.Errorf("ECS Exec: %w", err)
				}
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

			fmt.Printf("Session started: %s\n", compoundID)
			fmt.Printf("Status: %s\n", resp.Status)
			if resp.TaskArn != "" {
				fmt.Printf("Task:   %s\n", resp.TaskArn)
			}
			if resp.Region != "" {
				fmt.Printf("Region: %s\n", resp.Region)
			}
			fmt.Fprintf(os.Stderr, "\nConnect with: zoa session join %s\n", compoundID)
			return nil
		},
	}

	cmd.Flags().StringVarP(&flagDeployment, "deployment", "d", "", "Deployment name (e.g. us-east-1)")
	cmd.Flags().StringVarP(&flagTarget, "target", "t", "", "Target ID (e.g. mc01)")
	cmd.Flags().BoolVar(&flagConnect, "connect", true, "Connect via ECS Exec after the task is active")
	cmd.Flags().BoolVar(&flagNoConnect, "no-connect", false, "Do not connect; only create the session")

	return cmd
}

func newSessionStopCommand(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop <deployment/session-id>",
		Short: "Stop a boundary session",
		Long: `Stop a boundary session by its compound ID (deployment/session-id).

The compound ID is returned by 'zoa session start' and 'zoa session list'.`,
		Example: `  zoa session stop us-east-1/sess-abc123`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deployment, rawID, err := ParseSessionID(args[0])
			if err != nil {
				return err
			}

			if opts.APIURL == "" {
				opts.Deployment = deployment
			}

			c, err := getClient(opts)
			if err != nil {
				return fmt.Errorf("creating client: %w", err)
			}

			if err := c.SessionStop(cmd.Context(), rawID); err != nil {
				return fmt.Errorf("stopping session: %w", err)
			}

			fmt.Printf("Session %s stopped\n", args[0])
			return nil
		},
	}

	return cmd
}

func newSessionJoinCommand(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "join <deployment/session-id>",
		Short: "Join a boundary session via ECS Exec",
		Long: `Join a boundary session by its compound ID (deployment/session-id).

Calls the Access API for ownership checks, then opens an interactive shell via
ECS Exec and session-manager-plugin.

ECS Exec uses ZOA_EXEC_AWS_PROFILE or your default AWS credentials in the
deployment region (regional account), not the Access invoker role.`,
		Example: `  zoa session join us-east-1/sess-abc123`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deployment, rawID, err := ParseSessionID(args[0])
			if err != nil {
				return err
			}

			if opts.APIURL == "" {
				opts.Deployment = deployment
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
			if err := runSessionECSExec(cmd.Context(), region, resp); err != nil {
				return fmt.Errorf("ECS Exec: %w", err)
			}
			return nil
		},
	}

	return cmd
}

func newSessionListCommand(opts *GlobalOptions) *cobra.Command {
	var status, target string

	cmd := &cobra.Command{
		Use:   "list <deployment>",
		Short: "List boundary sessions",
		Long: `List boundary sessions for a deployment. Defaults to active sessions.
Use 'zoa session history' to see past sessions with extended filters.`,
		Example: `  # Active sessions
  zoa session list us-east-1

  zoa session list us-east-1 --status all

  zoa session list us-east-1 --target mc01

  zoa session list us-east-1 -o json`,
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

			query := url.Values{}
			query.Set("scope", "mine")
			if status != "" {
				query.Set("status", status)
			}
			if target != "" {
				query.Set("target", target)
			}

			list, err := c.ListSessions(cmd.Context(), query)
			if err != nil {
				return fmt.Errorf("listing sessions: %w", err)
			}

			if opts.OutputFormat == output.FormatJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(list)
			}

			if len(list.Items) == 0 {
				fmt.Println("No sessions found")
				return nil
			}

			tw := output.NewTable(os.Stdout)
			fmt.Fprintln(tw, "SESSION ID\tTARGET\tSTATUS\tCREATED\tDEADLINE")
			for _, s := range list.Items {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
					FormatSessionID(deployment, s.SessionID),
					s.Target, s.Status,
					output.Dash(s.CreatedAt), output.Dash(s.Deadline))
			}
			return tw.Flush()
		},
	}

	cmd.Flags().StringVar(&status, "status", "", "Filter by status (active, terminated, failed, all)")
	cmd.Flags().StringVar(&target, "target", "", "Filter by target cluster")

	return cmd
}

func newSessionHistoryCommand(opts *GlobalOptions) *cobra.Command {
	var since, until, operator, target, status string

	cmd := &cobra.Command{
		Use:   "history <deployment>",
		Short: "View session history across all operators",
		Long: `Show session history across all operators (audit view).
Defaults to last 24 hours.`,
		Example: `  zoa session history us-east-1

  zoa session history us-east-1 --since 7d

  zoa session history us-east-1 --operator slopezma --since 7d

  zoa session history us-east-1 -o json`,
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

			query := url.Values{}
			if since != "" {
				query.Set("since", since)
			}
			if until != "" {
				query.Set("until", until)
			}
			if operator != "" {
				query.Set("operator", operator)
			}
			if target != "" {
				query.Set("target", target)
			}
			if status != "" {
				query.Set("status", status)
			}

			list, err := c.ListSessions(cmd.Context(), query)
			if err != nil {
				return fmt.Errorf("listing session history: %w", err)
			}

			if opts.OutputFormat == output.FormatJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(list)
			}

			if len(list.Items) == 0 {
				fmt.Println("No sessions found")
				return nil
			}

			tw := output.NewTable(os.Stdout)
			fmt.Fprintln(tw, "SESSION ID\tOPERATOR\tTARGET\tSTATUS\tCREATED\tDEADLINE")
			for _, s := range list.Items {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
					FormatSessionID(deployment, s.SessionID),
					s.Operator, s.Target, s.Status,
					output.Dash(s.CreatedAt), output.Dash(s.Deadline))
			}
			return tw.Flush()
		},
	}

	cmd.Flags().StringVar(&since, "since", "24h", "Show sessions since (e.g. 1h, 7d, 2026-01-01)")
	cmd.Flags().StringVar(&until, "until", "", "Show sessions until (e.g. 1h, 2026-01-01)")
	cmd.Flags().StringVar(&operator, "operator", "", "Filter by operator")
	cmd.Flags().StringVar(&target, "target", "", "Filter by target cluster")
	cmd.Flags().StringVar(&status, "status", "", "Filter by status")

	return cmd
}

// resolveDeploymentTarget resolves deployment and target from positional args
// or flag fallbacks. Positional args take precedence over flags.
func resolveDeploymentTarget(args []string, flagDeployment, flagTarget string) (deployment, target string) {
	switch len(args) {
	case 2:
		return args[0], args[1]
	case 1:
		return args[0], flagTarget
	default:
		return flagDeployment, flagTarget
	}
}
