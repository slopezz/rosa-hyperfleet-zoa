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
	"github.com/openshift-online/rosa-seaplane-zoa/internal/cli/scopecatalog"
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
		newBreakglassActivateCommand(global),
	)
	return cmd
}

func newBreakglassScopesCommand(global *GlobalOptions) *cobra.Command {
	catalogOpts := &catalogOptions{}
	cmd := &cobra.Command{
		Use:   "scopes",
		Short: "List break-glass scope catalog",
		Long: `List break-glass scopes. With --offline (or without ZOA_API_URL in the boundary), uses the
catalog embedded in this CLI build, filtered by --target-type or ZOA_TARGET_TYPE.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if global.OutputFormat == output.FormatMarkdown && !useScopeCatalogOffline(global, catalogOpts) {
				return fmt.Errorf("-o markdown requires --offline (embedded catalog from this CLI build)")
			}
			if useScopeCatalogOffline(global, catalogOpts) {
				return listBreakglassScopesOffline(global, catalogOpts)
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
	registerCatalogFlags(cmd, catalogOpts)
	return cmd
}

func newBreakglassDescribeScopeCommand(global *GlobalOptions) *cobra.Command {
	catalogOpts := &catalogOptions{}
	cmd := &cobra.Command{
		Use:   "describe <scope>",
		Short: "Describe a break-glass scope",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return describeBreakglassScope(cmd.Context(), global, catalogOpts, args[0])
		},
	}
	registerCatalogFlags(cmd, catalogOpts)
	return cmd
}

func useScopeCatalogOffline(global *GlobalOptions, catalogOpts *catalogOptions) bool {
	return catalogOpts.offline || global.APIURL == ""
}

func listBreakglassScopesOffline(global *GlobalOptions, catalogOpts *catalogOptions) error {
	printOfflineScopeCatalogNotice()
	targetType, err := resolveCatalogTargetType(catalogOpts)
	if err != nil {
		return err
	}
	if global.OutputFormat == output.FormatMarkdown {
		body, err := scopecatalog.Markdown(targetType)
		if err != nil {
			return err
		}
		fmt.Fprint(os.Stdout, body)
		return nil
	}
	items, err := scopecatalog.List(targetType)
	if err != nil {
		return err
	}
	return renderBreakglassScopes(global.OutputFormat, items)
}

func describeBreakglassScope(ctx context.Context, global *GlobalOptions, catalogOpts *catalogOptions, scopeID string) error {
	if useScopeCatalogOffline(global, catalogOpts) {
		printOfflineScopeCatalogNotice()
		targetType, err := resolveCatalogTargetType(catalogOpts)
		if err != nil {
			return err
		}
		scope, err := scopecatalog.Get(targetType, scopeID)
		if err != nil {
			return err
		}
		if global.OutputFormat == output.FormatJSON {
			return output.JSON(os.Stdout, scope)
		}
		printScopeMetadata(scope)
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
	printScopeMetadata(scope)
	return nil
}

func printScopeMetadata(scope *client.ScopeMetadata) {
	if scope == nil {
		return
	}
	fmt.Printf("Scope:       %s\n", scope.ID)
	fmt.Printf("Plane:       %s\n", scope.Plane)
	fmt.Printf("Type:        %s\n", scope.Type)
	if scope.Authorization != nil {
		if v, ok := scope.Authorization["approval"].(string); ok {
			fmt.Printf("Approval:    %s\n", v)
		}
		if v, ok := scope.Authorization["default_grant_hours"].(float64); ok {
			fmt.Printf("Grant TTL:   %.0fh default after approve\n", v)
		}
	}
	fmt.Printf("Description: %s\n", scope.Description)
	composition := scope.Composition
	if len(composition) == 0 {
		composition = scope.Capabilities
	}
	for _, c := range composition {
		fmt.Printf("  • %s\n", c)
	}
	for _, r := range scope.Restrictions {
		fmt.Printf("  ✗ %s\n", r)
	}
	if scope.Plane == breakglass.PlaneKube && len(scope.RBAC) > 0 {
		fmt.Printf("\nEffective RBAC (target %s, %d rules):\n", scope.TargetType, len(scope.RBAC))
		for i, rule := range scope.RBAC {
			fmt.Printf("  [%d] apiGroups=%v resources=%v verbs=%v\n", i+1, rule.APIGroups, rule.Resources, rule.Verbs)
		}
	}
	if scope.Plane == breakglass.PlaneAWS {
		if scope.AwsRoleKey != "" {
			fmt.Printf("\nAWS role key: %s\n", scope.AwsRoleKey)
		}
		for _, line := range scope.IAMComposition {
			fmt.Printf("  • %s\n", line)
		}
	}
	fmt.Printf("\nExample: zoa breakglass request --scope %s --reason %s\n", scope.ID, reason.ExampleJiraIssue)
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
		Long: `Create a break-glass request. Default (human): stderr ack + hints; -o json prints the request object on stdout (like zoa run).

With --wait: stderr ack, poll with progress until approved/rejected/expired, then stdout metadata (like zoa get).
--wait does not activate credentials — run zoa breakglass activate <id> yourself after approval.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := createBreakglassRequest(cmd.Context(), global, scope, reasonFlag)
			if err != nil {
				return err
			}
			if !wait {
				progress := breakglassProgressWriter()
				printBreakglassRequestDispatched(progress, resp)
				if global.OutputFormat == output.FormatJSON {
					printBreakglassRequestHints(progress, resp)
					return output.JSON(os.Stdout, resp)
				}
				printBreakglassRequestDetail(progress, resp, breakglassApprovalDeployment(), false, breakglassRequestNextStep(resp, breakglassApprovalDeployment(), false))
				return nil
			}
			progress := breakglassProgressWriter()
			printBreakglassRequestDispatched(progress, resp)
			if isTerminalBreakglassStatus(resp.Status) {
				return finishBreakglassWait(global, resp, false, 0)
			}
			c, err := getClient(global)
			if err != nil {
				return err
			}
			pollRes, err := pollBreakglassRequestWithProgress(cmd.Context(), c, resp.RequestID, breakglassPollConfig{
				interval: pollInterval,
				timeout:  waitTimeout,
			})
			if err != nil {
				return err
			}
			printBreakglassWaitOutcome(pollRes.req, pollRes.timedOut, pollRes.elapsed)
			return finishBreakglassWait(global, pollRes.req, pollRes.timedOut, pollRes.elapsed)
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "", "Break-glass scope id (required)")
	cmd.Flags().StringVar(&reasonFlag, "reason", "",
		fmt.Sprintf("Required; env %s (%s)", reason.EnvVar, reason.FormatHint))
	cmd.Flags().BoolVar(&wait, "wait", false, "Wait for approval decision (does not activate)")
	cmd.Flags().DurationVar(&waitTimeout, "wait-timeout", 5*time.Minute, "Max poll duration when --wait is active")
	cmd.Flags().DurationVar(&pollInterval, "wait-poll-interval", 30*time.Second, "Poll frequency when --wait is active")
	_ = cmd.MarkFlagRequired("scope")
	return cmd
}

// createBreakglassRequest resolves reason (--reason, then ZOA_REASON) and creates the row.
func createBreakglassRequest(ctx context.Context, global *GlobalOptions, scope, reasonFlag string) (*client.BreakglassRequest, error) {
	reasonValue, err := reason.Resolve(reasonFlag)
	if err != nil {
		return nil, err
	}
	c, err := getClient(global)
	if err != nil {
		return nil, err
	}
	return c.CreateBreakglassRequest(ctx, scope, reasonValue)
}

func finishBreakglassWait(global *GlobalOptions, req *client.BreakglassRequest, timedOut bool, _ time.Duration) error {
	if global.OutputFormat == output.FormatJSON {
		if err := renderBreakglassRequest(global, req, timedOut); err != nil {
			return err
		}
		return breakglassWaitApprovalError(req, timedOut)
	}
	renderBreakglassGetHuman(os.Stdout, req, timedOut)
	return breakglassWaitApprovalError(req, timedOut)
}

func isTerminalBreakglassStatus(status string) bool {
	switch status {
	case "approved", "rejected", "expired":
		return true
	default:
		return false
	}
}

func newBreakglassRequestsCommand(global *GlobalOptions) *cobra.Command {
	var since, until, status, approval, decidedBy, decidedSince string
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
				Scope:        "mine",
				Since:        since,
				Until:        until,
				Status:       status,
				Approval:     approval,
				DecidedBy:    decidedBy,
				DecidedSince: decidedSince,
				Limit:        limit,
			})
			if err != nil {
				return err
			}
			return printBreakglassRequestTable(global, list, false)
		},
	}
	cmd.Flags().StringVar(&since, "since", "24h", "Start of time window (duration: 1h, 7d; date: 2026-08-25; RFC3339)")
	cmd.Flags().StringVar(&until, "until", "", "End of time window (same formats as --since; default: now)")
	cmd.Flags().StringVar(&status, "status", "", "Filter by lifecycle status (pending, approved, rejected, expired)")
	cmd.Flags().StringVar(&approval, "approval", "", "Filter by approval outcome (pending, approved, rejected)")
	cmd.Flags().StringVar(&decidedBy, "decided-by", "", "Filter by approver/rejecter (peer decisions)")
	cmd.Flags().StringVar(&decidedSince, "decided-since", "", "Only rows decided on/after this time (duration or RFC3339)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Max results (max 200)")
	return cmd
}

func newBreakglassHistoryCommand(global *GlobalOptions) *cobra.Command {
	var since, until, operator, status, approval, decidedBy, decidedSince string
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
				Operator:     operator,
				Status:       status,
				Approval:     approval,
				DecidedBy:    decidedBy,
				DecidedSince: decidedSince,
				Since:        since,
				Until:        until,
				Limit:        limit,
			})
			if err != nil {
				return err
			}
			return printBreakglassRequestTable(global, list, true)
		},
	}
	cmd.Flags().StringVar(&since, "since", "24h", "Start of time window (duration: 1h, 7d; date: 2026-08-25; RFC3339)")
	cmd.Flags().StringVar(&until, "until", "", "End of time window (same formats as --since; default: now)")
	cmd.Flags().StringVar(&operator, "operator", "", "Filter by operator")
	cmd.Flags().StringVar(&status, "status", "", "Filter by lifecycle status")
	cmd.Flags().StringVar(&approval, "approval", "", "Filter by approval outcome")
	cmd.Flags().StringVar(&decidedBy, "decided-by", "", "Filter by approver/rejecter")
	cmd.Flags().StringVar(&decidedSince, "decided-since", "", "Only rows decided on/after this time")
	cmd.Flags().IntVar(&limit, "limit", 50, "Max results (max 200)")
	return cmd
}

func newBreakglassGetCommand(global *GlobalOptions) *cobra.Command {
	var wait bool
	var waitTimeout, pollInterval time.Duration
	cmd := &cobra.Command{
		Use:   "get <request-id>",
		Short: "Get a break-glass request",
		Long:  `Get request status (stdout metadata, like zoa get). With --wait, poll with progress until approved/rejected/expired. Does not activate — use zoa breakglass activate after approval.`,
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
				elapsed := time.Duration(0)
				timedOut := false
				if !isTerminalBreakglassStatus(resp.Status) {
					pollRes, pollErr := pollBreakglassRequestWithProgress(cmd.Context(), c, id, breakglassPollConfig{
						interval: pollInterval,
						timeout:  waitTimeout,
					})
					if pollErr != nil {
						return pollErr
					}
					resp = pollRes.req
					timedOut = pollRes.timedOut
					elapsed = pollRes.elapsed
					printBreakglassWaitOutcome(resp, timedOut, elapsed)
				}
				return finishBreakglassWait(global, resp, timedOut, elapsed)
			}
			if global.OutputFormat == output.FormatJSON {
				return output.JSON(os.Stdout, resp)
			}
			renderBreakglassGetHuman(os.Stdout, resp, false)
			return nil
		},
	}
	cmd.Flags().BoolVar(&wait, "wait", false, "Poll until approval decision (does not activate)")
	cmd.Flags().DurationVar(&waitTimeout, "wait-timeout", 5*time.Minute, "Max poll duration when --wait is active")
	cmd.Flags().DurationVar(&pollInterval, "wait-poll-interval", 30*time.Second, "Poll frequency when --wait is active")
	return cmd
}

func newBreakglassActivateCommand(global *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "activate <request-id>",
		Short: "Provision and activate break-glass credentials",
		Long: `Calls POST …/requests/{id}/activate: provisions RBAC or assumes AWS role, returns credentials,
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
	plane, err := active.PlaneForScope(resp.Scope)
	if err != nil {
		return err
	}
	if err := validateBreakglassActivateArtifacts(home, requestID, plane, resp); err != nil {
		return err
	}
	progress := breakglassProgressWriter()
	printBreakglassActivateDispatched(progress, requestID, resp.TargetCluster, resp.Scope)
	printBreakglassActivateHints(progress, home, requestID, plane, resp)
	if global.OutputFormat == output.FormatJSON {
		id := resp.ID
		if id == "" {
			id = requestID
		}
		safe := map[string]interface{}{
			"id":             id,
			"request_id":     requestID,
			"scope":          resp.Scope,
			"target_cluster": resp.TargetCluster,
			"status":         resp.Status,
			"config_dir":     dir,
		}
		return output.JSON(os.Stdout, safe)
	}
	return nil
}

func writeKubeconfig(dir string, resp *client.BreakglassActivateResponse) error {
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
