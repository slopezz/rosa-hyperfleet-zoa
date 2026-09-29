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
		Long:  `Start, stop, join, and list boundary sessions for audited SRE access.`,
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
	var deployment, target string

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start a boundary session",
		RunE: func(cmd *cobra.Command, args []string) error {
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

			if opts.OutputFormat == output.FormatJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(resp)
			}

			fmt.Printf("Session started: %s\n", resp.SessionID)
			fmt.Printf("Status: %s\n", resp.Status)
			if resp.TaskArn != "" {
				fmt.Printf("Task:   %s\n", resp.TaskArn)
			}
			if resp.Region != "" {
				fmt.Printf("Region: %s\n", resp.Region)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&deployment, "deployment", "", "Deployment name (e.g. us-east-1)")
	cmd.Flags().StringVar(&target, "target", "", "Target ID (e.g. mc01)")
	_ = cmd.MarkFlagRequired("target")

	return cmd
}

func newSessionStopCommand(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop <session-id>",
		Short: "Stop a boundary session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := getClient(opts)
			if err != nil {
				return fmt.Errorf("creating client: %w", err)
			}

			if err := c.SessionStop(cmd.Context(), args[0]); err != nil {
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
		Use:   "join <session-id>",
		Short: "Join a boundary session via ECS Exec",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := getClient(opts)
			if err != nil {
				return fmt.Errorf("creating client: %w", err)
			}

			resp, err := c.SessionJoin(cmd.Context(), args[0])
			if err != nil {
				return fmt.Errorf("joining session: %w", err)
			}

			if opts.OutputFormat == output.FormatJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(resp)
			}

			fmt.Println("Connect with:")
			fmt.Printf("  aws ecs execute-command \\\n")
			fmt.Printf("    --cluster %s \\\n", resp.ClusterArn)
			fmt.Printf("    --task %s \\\n", resp.TaskArn)
			fmt.Printf("    --container %s \\\n", resp.ContainerName)
			fmt.Printf("    --interactive \\\n")
			fmt.Printf("    --command /bin/bash \\\n")
			fmt.Printf("    --region %s\n", resp.Region)
			return nil
		},
	}

	return cmd
}

func newSessionListCommand(opts *GlobalOptions) *cobra.Command {
	var status, operator string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List boundary sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := getClient(opts)
			if err != nil {
				return fmt.Errorf("creating client: %w", err)
			}

			query := url.Values{}
			if status != "" {
				query.Set("status", status)
			}
			if operator != "" {
				query.Set("operator", operator)
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
			fmt.Fprintln(tw, "SESSION ID\tOPERATOR\tTARGET\tSTATUS\tCREATED")
			for _, s := range list.Items {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
					s.SessionID, s.Operator, s.Target, s.Status, output.Dash(s.CreatedAt))
			}
			return tw.Flush()
		},
	}

	cmd.Flags().StringVar(&status, "status", "", "Filter by session status")
	cmd.Flags().StringVar(&operator, "operator", "", "Filter by operator")

	return cmd
}

func newSessionHistoryCommand(opts *GlobalOptions) *cobra.Command {
	var since, until, operator string

	cmd := &cobra.Command{
		Use:   "history",
		Short: "List all sessions across operators (with time filters)",
		Long:  `Show historical sessions across all operators. Defaults to last 24 hours.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := getClient(opts)
			if err != nil {
				return fmt.Errorf("creating client: %w", err)
			}

			query := url.Values{}
			query.Set("all", "true")
			if since != "" {
				query.Set("since", since)
			}
			if until != "" {
				query.Set("until", until)
			}
			if operator != "" {
				query.Set("operator", operator)
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
					s.SessionID, s.Operator, s.Target, s.Status,
					output.Dash(s.CreatedAt), output.Dash(s.Deadline))
			}
			return tw.Flush()
		},
	}

	cmd.Flags().StringVar(&since, "since", "24h", "Show sessions since (e.g. 1h, 7d, 2026-01-01)")
	cmd.Flags().StringVar(&until, "until", "", "Show sessions until (e.g. 1h, 2026-01-01)")
	cmd.Flags().StringVar(&operator, "operator", "", "Filter by operator")

	return cmd
}
