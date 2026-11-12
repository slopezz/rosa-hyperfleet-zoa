package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/cli/reason"
	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
	"github.com/openshift-online/rosa-seaplane-zoa/internal/output"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/breakglass"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/breakglass/active"
)

func newBreakglassCommand(global *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "breakglass",
		Short: "Break-glass emergency scoped access (boundary)",
		Long: `Request, list, and activate break-glass grants from inside the boundary (ZOA_API_URL).

Laptop approvers use: zoa approve <deployment>/<request-id> [--comment ...]`,
	}
	cmd.AddCommand(
		newBreakglassScopesCommand(global),
		newBreakglassDescribeScopeCommand(global),
		newBreakglassRequestCommand(global),
		newBreakglassRequestsCommand(global),
		newBreakglassHistoryCommand(global),
		newBreakglassGetCommand(global),
		newBreakglassUseCommand(global),
	)
	return cmd
}

func newBreakglassScopesCommand(global *GlobalOptions) *cobra.Command {
	var offline bool
	cmd := &cobra.Command{
		Use:   "scopes",
		Short: "List break-glass scope catalog",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if offline || global.APIURL == "" {
				items := breakglass.List()
				return renderBreakglassScopes(global.OutputFormat, toClientScopes(items))
			}
			c, err := getClient(global)
			if err != nil {
				return err
			}
			list, err := c.ListBreakglassScopes(cmd.Context())
			if err != nil {
				return err
			}
			return renderBreakglassScopes(global.OutputFormat, list.Items)
		},
	}
	cmd.Flags().BoolVar(&offline, "offline", false, "Use embedded scope catalog (no API)")
	return cmd
}

func newBreakglassDescribeScopeCommand(global *GlobalOptions) *cobra.Command {
	var offline bool
	cmd := &cobra.Command{
		Use:   "describe <scope>",
		Short: "Describe a break-glass scope",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return describeBreakglassScope(cmd.Context(), global, offline, args[0])
		},
	}
	cmd.Flags().BoolVar(&offline, "offline", false, "Use embedded scope catalog (no API)")
	return cmd
}

func describeBreakglassScope(ctx context.Context, global *GlobalOptions, offline bool, scopeID string) error {
	if offline || global.APIURL == "" {
		def, ok := breakglass.Get(scopeID)
		if !ok {
			return fmt.Errorf("unknown scope %q", scopeID)
		}
		if global.OutputFormat == output.FormatJSON {
			return output.JSON(os.Stdout, def.Metadata)
		}
		printScopeMetadata(def.Metadata)
		return nil
	}
	c, err := getClient(global)
	if err != nil {
		return err
	}
	scope, err := c.GetBreakglassScope(ctx, scopeID)
	if err != nil {
		return err
	}
	if global.OutputFormat == output.FormatJSON {
		return output.JSON(os.Stdout, scope)
	}
	printScopeMetadata(clientScopeToLocal(scope))
	return nil
}

func clientScopeToLocal(s *client.ScopeMetadata) breakglass.ScopeMetadata {
	out := breakglass.ScopeMetadata{
		ID:           s.ID,
		Plane:        s.Plane,
		Type:         s.Type,
		Description:  s.Description,
		Capabilities: s.Capabilities,
		Restrictions: s.Restrictions,
	}
	if s.Authorization != nil {
		if v, ok := s.Authorization["approval"].(string); ok {
			out.Authorization.Approval = v
		}
	}
	return out
}

func printScopeMetadata(meta breakglass.ScopeMetadata) {
	fmt.Printf("Scope:       %s\n", meta.ID)
	fmt.Printf("Plane:       %s\n", meta.Plane)
	fmt.Printf("Type:        %s\n", meta.Type)
	fmt.Printf("Approval:    %s\n", meta.Authorization.Approval)
	fmt.Printf("Description: %s\n", meta.Description)
	for _, c := range meta.Capabilities {
		fmt.Printf("  • %s\n", c)
	}
	for _, r := range meta.Restrictions {
		fmt.Printf("  ✗ %s\n", r)
	}
	fmt.Printf("\nExample: zoa breakglass request --scope %s --reason %s\n", meta.ID, reason.FormatHint)
}

func toClientScopes(in []breakglass.ScopeMetadata) []client.ScopeMetadata {
	out := make([]client.ScopeMetadata, len(in))
	for i, s := range in {
		out[i] = client.ScopeMetadata{ID: s.ID, Plane: s.Plane, Type: s.Type, Description: s.Description}
	}
	return out
}

func renderBreakglassScopes(format output.Format, items []client.ScopeMetadata) error {
	if format == output.FormatJSON {
		return output.JSON(os.Stdout, map[string]interface{}{"items": items, "count": len(items)})
	}
	for _, s := range items {
		fmt.Printf("%-14s %-6s %-8s %s\n", s.ID, s.Plane, s.Type, s.Description)
	}
	return nil
}

func newBreakglassRequestCommand(global *GlobalOptions) *cobra.Command {
	var (
		scope        string
		reasonFlag   string
		wait         bool
		waitTimeout  time.Duration
		pollInterval time.Duration
	)
	cmd := &cobra.Command{
		Use:   "request",
		Short: "Create a break-glass request",
		Long: `Create a break-glass request. Default: print the request JSON and exit (like async zoa run without --wait).

With --wait: poll until approved or terminal failure, then provision credentials (same as zoa breakglass use).`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := createBreakglassRequest(cmd.Context(), global, scope, reasonFlag, wait, pollInterval, waitTimeout)
			if err != nil {
				return err
			}
			requestID := resp.RequestID
			if !wait {
				if !isTerminalBreakglassStatus(resp.Status) {
					fmt.Fprintf(os.Stderr, "  request %s — use 'zoa breakglass get %s' or --wait to track\n", requestID, requestID)
				}
				return output.JSON(os.Stdout, resp)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "", "Break-glass scope id (required)")
	cmd.Flags().StringVar(&reasonFlag, "reason", "",
		fmt.Sprintf("Required; env %s (%s)", reason.EnvVar, reason.FormatHint))
	cmd.Flags().BoolVar(&wait, "wait", false, "Wait for approval then activate (like async zoa run --wait)")
	cmd.Flags().DurationVar(&waitTimeout, "wait-timeout", 5*time.Minute, "Max poll duration when --wait is active")
	cmd.Flags().DurationVar(&pollInterval, "wait-poll-interval", 30*time.Second, "Poll frequency when --wait is active")
	_ = cmd.MarkFlagRequired("scope")
	return cmd
}

// createBreakglassRequest resolves reason (--reason, then ZOA_REASON), creates the row, and optionally waits.
func createBreakglassRequest(ctx context.Context, global *GlobalOptions, scope, reasonFlag string, wait bool, pollInterval, waitTimeout time.Duration) (*client.BreakglassRequest, error) {
	reasonValue, err := reason.Resolve(reasonFlag)
	if err != nil {
		return nil, err
	}
	c, err := getClient(global)
	if err != nil {
		return nil, err
	}
	resp, err := c.CreateBreakglassRequest(ctx, scope, reasonValue)
	if err != nil {
		return nil, err
	}
	if !wait {
		return resp, nil
	}
	final, err := pollBreakglassRequest(ctx, c, resp.RequestID, pollInterval, waitTimeout)
	if err != nil {
		return nil, err
	}
	switch final.Status {
	case "rejected":
		return nil, fmt.Errorf("break-glass request %s was rejected", resp.RequestID)
	case "expired":
		return nil, fmt.Errorf("break-glass request %s expired before approval", resp.RequestID)
	case "approved":
		if err := activateBreakglass(ctx, global, resp.RequestID); err != nil {
			return nil, err
		}
		return final, nil
	default:
		return nil, fmt.Errorf("break-glass request %s not approved before timeout (status %s)", resp.RequestID, final.Status)
	}
}

func isTerminalBreakglassStatus(status string) bool {
	switch status {
	case "approved", "rejected", "expired":
		return true
	default:
		return false
	}
}

func pollBreakglassRequest(ctx context.Context, c APIClient, requestID string, interval, timeout time.Duration) (*client.BreakglassRequest, error) {
	deadline := time.Now().Add(timeout)
	for {
		req, err := c.GetBreakglassRequest(ctx, requestID)
		if err != nil {
			return nil, err
		}
		switch req.Status {
		case "approved", "rejected", "expired":
			return req, nil
		}
		if time.Now().After(deadline) {
			return req, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}
}

func newBreakglassRequestsCommand(global *GlobalOptions) *cobra.Command {
	var since, until, status string
	var limit int
	cmd := &cobra.Command{
		Use:     "requests",
		Short:   "List your break-glass requests",
		Aliases: []string{"list"},
		Long: `List your requests on this target (scope=mine). Not audited.

Use 'zoa breakglass history' for fleet-wide audit view.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := getClient(global)
			if err != nil {
				return err
			}
			list, err := c.ListBreakglassRequests(cmd.Context(), &client.BreakglassListParams{
				Scope:  "mine",
				Since:  since,
				Until:  until,
				Status: status,
				Limit:  limit,
			})
			if err != nil {
				return err
			}
			return output.JSON(os.Stdout, list)
		},
	}
	cmd.Flags().StringVar(&since, "since", "24h", "Start of time window (duration: 1h, 7d; date: 2026-08-25; RFC3339)")
	cmd.Flags().StringVar(&until, "until", "", "End of time window (same formats as --since; default: now)")
	cmd.Flags().StringVar(&status, "status", "", "Filter by status (omit or all for any)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Max results (max 200)")
	return cmd
}

func newBreakglassHistoryCommand(global *GlobalOptions) *cobra.Command {
	var since, until, operator, status string
	var limit int
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Break-glass requests across all operators",
		Long: `Fleet-wide break-glass history on this target (RC or MC). Audited on the API (like zoa session history).

Does not pass scope=mine — same pattern as session history vs session list.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := getClient(global)
			if err != nil {
				return err
			}
			list, err := c.ListBreakglassRequests(cmd.Context(), &client.BreakglassListParams{
				Operator: operator,
				Status:   status,
				Since:    since,
				Until:    until,
				Limit:    limit,
			})
			if err != nil {
				return err
			}
			return output.JSON(os.Stdout, list)
		},
	}
	cmd.Flags().StringVar(&since, "since", "24h", "Start of time window (duration: 1h, 7d; date: 2026-08-25; RFC3339)")
	cmd.Flags().StringVar(&until, "until", "", "End of time window (same formats as --since; default: now)")
	cmd.Flags().StringVar(&operator, "operator", "", "Filter by operator")
	cmd.Flags().StringVar(&status, "status", "", "Filter by status")
	cmd.Flags().IntVar(&limit, "limit", 50, "Max results (max 200)")
	return cmd
}

func newBreakglassGetCommand(global *GlobalOptions) *cobra.Command {
	var wait bool
	var waitTimeout, pollInterval time.Duration
	cmd := &cobra.Command{
		Use:   "get <request-id>",
		Short: "Get a break-glass request",
		Long:  `Get request status. With --wait, poll GET until approved/rejected/expired, then activate on approved (like zoa run --wait: poll get, then deliver the result — here use/credentials instead of TA output).`,
		Example: `  zoa breakglass get <request-id>
  zoa breakglass get <request-id> --wait
  zoa breakglass get <request-id> --wait --wait-timeout 10m`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := getClient(global)
			if err != nil {
				return err
			}
			id := args[0]
			resp, err := c.GetBreakglassRequest(cmd.Context(), id)
			if err != nil {
				return err
			}
			if wait {
				if !isTerminalBreakglassStatus(resp.Status) {
					resp, err = pollBreakglassRequest(cmd.Context(), c, id, pollInterval, waitTimeout)
					if err != nil {
						return err
					}
				}
				switch resp.Status {
				case "approved":
					return activateBreakglass(cmd.Context(), global, id)
				case "rejected":
					return fmt.Errorf("break-glass request %s was rejected", id)
				case "expired":
					return fmt.Errorf("break-glass request %s expired", id)
				default:
					return fmt.Errorf("break-glass request %s not approved before timeout (status %s)", id, resp.Status)
				}
			}
			return output.JSON(os.Stdout, resp)
		},
	}
	cmd.Flags().BoolVar(&wait, "wait", false, "Poll until terminal, then use when approved")
	cmd.Flags().DurationVar(&waitTimeout, "wait-timeout", 5*time.Minute, "Max poll duration when --wait is active")
	cmd.Flags().DurationVar(&pollInterval, "wait-poll-interval", 30*time.Second, "Poll frequency when --wait is active")
	return cmd
}

func newBreakglassUseCommand(global *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "use <request-id>",
		Short: "Provision and activate break-glass credentials",
		Long: `Calls POST …/requests/{id}/connect: provisions RBAC or assumes AWS role, returns credentials,
writes ~/.config/zoa/breakglass/, updates the shell prompt. Safe to retry (kube RBAC is idempotent by request id).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return activateBreakglass(cmd.Context(), global, args[0])
		},
	}
	return cmd
}

func activateBreakglass(ctx context.Context, global *GlobalOptions, requestID string) error {
	c, err := getClient(global)
	if err != nil {
		return err
	}
	resp, err := c.ActivateBreakglass(ctx, requestID)
	if err != nil {
		return err
	}
	home := os.Getenv("HOME")
	dir := active.RequestDir(home, requestID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	meta := map[string]string{"request_id": requestID, "scope": resp.Scope, "grant_expires_at": resp.GrantExpiresAt}
	metaBytes, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), metaBytes, 0o600); err != nil {
		return err
	}
	stanza := active.Stanza{
		RequestID:      requestID,
		Scope:          resp.Scope,
		GrantExpiresAt: resp.GrantExpiresAt,
	}
	if err := active.Activate(home, stanza); err != nil {
		return err
	}
	if resp.KubeToken != "" {
		if err := writeKubeconfig(dir, resp); err != nil {
			return err
		}
	}
	if err := writeBreakglassAWSProfile(home, requestID, resp.AWS); err != nil {
		return err
	}
	if global.OutputFormat == output.FormatJSON {
		safe := map[string]interface{}{"request_id": requestID, "scope": resp.Scope, "status": resp.Status, "config_dir": dir}
		return output.JSON(os.Stdout, safe)
	}
	return nil
}

func writeKubeconfig(dir string, resp *client.BreakglassConnectResponse) error {
	if resp.Kube == nil || resp.KubeToken == "" {
		return nil
	}
	cfg := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Config",
		"clusters": []map[string]interface{}{{
			"name": "breakglass",
			"cluster": map[string]string{
				"server":                     fmt.Sprint(resp.Kube["server"]),
				"certificate-authority-data": fmt.Sprint(resp.Kube["certificate_authority_data"]),
			},
		}},
		"contexts": []map[string]interface{}{{
			"name": "breakglass",
			"context": map[string]string{
				"cluster": "breakglass",
				"user":    "breakglass",
			},
		}},
		"current-context": "breakglass",
		"users": []map[string]interface{}{{
			"name": "breakglass",
			"user": map[string]string{"token": resp.KubeToken},
		}},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "kubeconfig"), data, 0o600)
}

func newApproveCommand(global *GlobalOptions) *cobra.Command {
	var comment string
	cmd := &cobra.Command{
		Use:   "approve <deployment>/<request-id>",
		Short: "Approve a pending break-glass request (laptop / Access)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			global.Deployment = deploymentFromCompound(args[0])
			c, err := getClient(global)
			if err != nil {
				return err
			}
			return c.Approve(cmd.Context(), args[0], comment)
		},
	}
	cmd.Flags().StringVar(&comment, "comment", "", "Decision note")
	return cmd
}

func newRejectCommand(global *GlobalOptions) *cobra.Command {
	var comment string
	cmd := &cobra.Command{
		Use:   "reject <deployment>/<request-id>",
		Short: "Reject a pending break-glass request (laptop / Access)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			global.Deployment = deploymentFromCompound(args[0])
			c, err := getClient(global)
			if err != nil {
				return err
			}
			return c.Reject(cmd.Context(), args[0], comment)
		},
	}
	cmd.Flags().StringVar(&comment, "comment", "", "Decision note")
	return cmd
}

func deploymentFromCompound(compound string) string {
	dep, _, err := ParseSessionID(compound)
	if err != nil {
		return ""
	}
	return dep
}
