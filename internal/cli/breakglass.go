package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/catalog"

	"github.com/spf13/cobra"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/cli/parambind"
	"github.com/openshift-online/rosa-seaplane-zoa/internal/cli/profilecatalog"
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
		newBreakglassProfilesCommand(global),
		newBreakglassDescribeProfileCommand(global),
		newBreakglassRequestCommand(global),
		newBreakglassRequestsCommand(global),
		newBreakglassHistoryCommand(global),
		newBreakglassGetCommand(global),
		newBreakglassActivateCommand(global),
	)
	return cmd
}

func newBreakglassProfilesCommand(global *GlobalOptions) *cobra.Command {
	catalogOpts := &catalogOptions{}
	var scopeFilter, typeFilter string
	cmd := &cobra.Command{
		Use:   "profiles",
		Short: "List break-glass profiles",
		Long: `List break-glass profiles. With --offline (or without ZOA_API_URL in the boundary), uses the
catalog embedded in this CLI build, filtered by --target-type or ZOA_TARGET_TYPE.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if global.OutputFormat == output.FormatMarkdown && !useProfileCatalogOffline(global, catalogOpts) {
				return fmt.Errorf("-o markdown requires --offline (embedded catalog from this CLI build)")
			}
			if useProfileCatalogOffline(global, catalogOpts) {
				return listBreakglassProfilesOffline(global, catalogOpts, scopeFilter, typeFilter)
			}
			c, err := getClient(global)
			if err != nil {
				return err
			}
			q := catalogFilterQuery(scopeFilter, typeFilter)
			list, err := c.ListBreakglassProfiles(cmd.Context(), q)
			if err != nil {
				return err
			}
			return renderBreakglassProfiles(global.OutputFormat, list)
		},
	}
	registerCatalogFlags(cmd, catalogOpts)
	registerCatalogFilterFlags(cmd, &scopeFilter, &typeFilter)
	return cmd
}

func newBreakglassDescribeProfileCommand(global *GlobalOptions) *cobra.Command {
	catalogOpts := &catalogOptions{}
	cmd := &cobra.Command{
		Use:   "describe <profile>",
		Short: "Describe a break-glass profile",
		Long: `Show profile capabilities, restrictions, and effective RBAC or IAM notes.

By default, calls the ZOA API. With --offline, reads embedded registry metadata (see zoa breakglass profiles --offline).`,
		Example: `  zoa breakglass describe kube_read

  zoa breakglass describe kube_elevated --target-type mc

  zoa breakglass describe aws_write -o json`,
		Args: cobra.ExactArgs(1),
		ValidArgsFunction: func(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return completeProfileNames(toComplete), cobra.ShellCompDirectiveNoFileComp
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return describeBreakglassProfile(cmd.Context(), global, catalogOpts, args[0])
		},
	}
	registerCatalogFlags(cmd, catalogOpts)
	return cmd
}

func useProfileCatalogOffline(global *GlobalOptions, catalogOpts *catalogOptions) bool {
	return catalogOpts.offline || global.APIURL == ""
}

func catalogFilterQuery(scopeFilter, typeFilter string) url.Values {
	q := make(url.Values)
	if scopeFilter != "" {
		q.Set("scope", scopeFilter)
	}
	if typeFilter != "" {
		q.Set("type", typeFilter)
	}
	return q
}

func listBreakglassProfilesOffline(global *GlobalOptions, catalogOpts *catalogOptions, scopeFilter, typeFilter string) error {
	printOfflineProfileCatalogNotice()
	targetType, err := resolveCatalogTargetType(catalogOpts)
	if err != nil {
		return err
	}
	if global.OutputFormat == output.FormatMarkdown {
		body, err := profilecatalog.Markdown(targetType)
		if err != nil {
			return err
		}
		fmt.Fprint(os.Stdout, body)
		return nil
	}
	items, err := profilecatalog.List(targetType)
	if err != nil {
		return err
	}
	filtered := items[:0]
	for _, item := range items {
		if !catalog.MatchCredentialScope(item.Scope, scopeFilter) {
			continue
		}
		if !catalog.MatchPrivilegeType(item.Type, typeFilter) {
			continue
		}
		filtered = append(filtered, item)
	}
	return renderBreakglassProfiles(global.OutputFormat, &client.BreakglassProfileList{
		Items: filtered,
		Count: len(filtered),
	})
}

func describeBreakglassProfile(ctx context.Context, global *GlobalOptions, catalogOpts *catalogOptions, profileName string) error {
	if useProfileCatalogOffline(global, catalogOpts) {
		printOfflineProfileCatalogNotice()
		targetType, err := resolveCatalogTargetType(catalogOpts)
		if err != nil {
			return err
		}
		scope, err := profilecatalog.Get(targetType, profileName)
		if err != nil {
			return err
		}
		if global.OutputFormat == output.FormatJSON {
			return output.JSON(os.Stdout, parambind.EnrichProfileDescribe(scope))
		}
		profilecatalog.FormatDescribeHuman(os.Stdout, scope)
		return nil
	}
	c, err := getClient(global)
	if err != nil {
		return err
	}
	scope, err := c.GetBreakglassProfile(ctx, profileName)
	if err != nil {
		return err
	}
	if global.OutputFormat == output.FormatJSON {
		return output.JSON(os.Stdout, parambind.EnrichProfileDescribe(scope))
	}
	profilecatalog.FormatDescribeHuman(os.Stdout, scope)
	return nil
}

func profileCatalogApproval(s client.ProfileMetadata) string {
	return profileCatalogAuthString(s, "approval")
}

func profileCatalogRequestTTL(s client.ProfileMetadata) string {
	h := profileCatalogAuthInt(s, "default_request_hours")
	if h <= 0 {
		return ""
	}
	return fmt.Sprintf("%dh", h)
}

func profileCatalogGrantTTL(s client.ProfileMetadata) string {
	h := profileCatalogAuthInt(s, "default_grant_hours")
	if h <= 0 {
		return ""
	}
	return fmt.Sprintf("%dh", h)
}

func profileCatalogCredentialTTL(s client.ProfileMetadata) string {
	if s.Scope == "" {
		return ""
	}
	return breakglass.CredentialMaxTTLDisplay(s.Scope)
}

func profileCatalogAuthString(s client.ProfileMetadata, key string) string {
	if s.Authorization == nil {
		return ""
	}
	raw, ok := s.Authorization[key]
	if !ok || raw == nil {
		return ""
	}
	if v, ok := raw.(string); ok {
		return v
	}
	return fmt.Sprint(raw)
}

func profileCatalogAuthInt(s client.ProfileMetadata, key string) int {
	if s.Authorization == nil {
		return 0
	}
	raw, ok := s.Authorization[key]
	if !ok || raw == nil {
		return 0
	}
	switch v := raw.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func renderBreakglassProfiles(format output.Format, list *client.BreakglassProfileList) error {
	if list == nil {
		list = &client.BreakglassProfileList{}
	}
	if format == output.FormatJSON {
		return output.JSON(os.Stdout, list)
	}
	tw := output.NewTable(os.Stdout)
	fmt.Fprintf(tw, "NAME\tSCOPE\tTYPE\tAPPROVAL\tREQUEST_TTL\tGRANT_TTL\tCREDENTIAL_TTL\tDESCRIPTION\n")
	for _, s := range list.Items {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			s.Name, s.Scope, s.Type,
			output.Dash(profileCatalogApproval(s)),
			output.Dash(profileCatalogRequestTTL(s)),
			output.Dash(profileCatalogGrantTTL(s)),
			output.Dash(profileCatalogCredentialTTL(s)),
			s.Description)
	}
	return tw.Flush()
}

func newBreakglassRequestCommand(global *GlobalOptions) *cobra.Command {
	var (
		reasonFlag   string
		wait         bool
		waitTimeout  time.Duration
		pollInterval time.Duration
	)
	cmd := &cobra.Command{
		Use:   "request <profile>",
		Short: "Create a break-glass request",
		Long: `Create a break-glass request. Default (human): stderr ack + hints; -o json prints the request object on stdout (like zoa run).

A reason is required (Jira issue or PagerDuty incident): use --reason or ZOA_REASON in the boundary task.

With --wait: stderr ack, poll with progress until active/denied/expired, then stdout metadata (like zoa get).
--wait does not activate credentials — run zoa breakglass activate <id> yourself after approval.`,
		Example: `  zoa breakglass request kube_read --reason ROSAENG-1234

  zoa breakglass request kube_elevated --reason ROSAENG-1234 --wait

  zoa breakglass request aws_write --reason '#123456' -o json`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("missing profile name\n\n  Usage: zoa breakglass request <profile> --reason <ticket>\n  Run 'zoa breakglass profiles' to see available profiles")
			}
			if len(args) > 1 {
				return fmt.Errorf("unexpected argument %q — pass flags after the profile name\n\n  Example: zoa breakglass request %s --reason <ticket>",
					args[1], args[0])
			}
			return nil
		},
		ValidArgsFunction: func(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return completeProfileNames(toComplete), cobra.ShellCompDirectiveNoFileComp
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := createBreakglassRequest(cmd.Context(), global, args[0], reasonFlag)
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
	cmd.Flags().StringVar(&reasonFlag, "reason", "", parambind.RequestFlagHelp("reason"))
	cmd.Flags().BoolVar(&wait, "wait", false, parambind.RequestFlagHelp("wait"))
	cmd.Flags().DurationVar(&waitTimeout, "wait-timeout", 5*time.Minute, parambind.RequestFlagHelp("wait-timeout"))
	cmd.Flags().DurationVar(&pollInterval, "wait-poll-interval", 30*time.Second, parambind.RequestFlagHelp("wait-poll-interval"))
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
	case "active", "denied", "expired":
		return true
	default:
		return false
	}
}

func newBreakglassRequestsCommand(global *GlobalOptions) *cobra.Command {
	var since, until, status, approval, decidedBy, decidedSince, target string
	var limit int
	cmd := &cobra.Command{
		Use:     "requests",
		Short:   "List your break-glass requests",
		Aliases: []string{"list"},
		Long: `List your requests on this deployment (mine=true). Not audited.

Lists RC and MC targets in the shared break-glass table unless --target is set (same as zoa runs).

Use 'zoa breakglass history' for fleet-wide audit view.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := getClient(global)
			if err != nil {
				return err
			}
			list, err := c.ListBreakglassRequests(cmd.Context(), &client.BreakglassListParams{
				Mine:         true,
				Target:       target,
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
	cmd.Flags().StringVarP(&target, "target", "t", "", "Filter by target cluster (default: all targets in deployment)")
	cmd.Flags().StringVar(&since, "since", "24h", "Start of time window (duration: 1h, 7d; date: 2026-08-25; RFC3339)")
	cmd.Flags().StringVar(&until, "until", "", "End of time window (same formats as --since; default: now)")
	cmd.Flags().StringVar(&status, "status", "", "Filter by lifecycle status (pending, active, expired, denied)")
	cmd.Flags().StringVar(&approval, "approval", "", "Filter by approval outcome (pending, approved, rejected)")
	cmd.Flags().StringVar(&decidedBy, "decided-by", "", "Filter by approver/rejecter (peer decisions)")
	cmd.Flags().StringVar(&decidedSince, "decided-since", "", "Only rows decided on/after this time (duration or RFC3339)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Max results (max 200)")
	return cmd
}

func newBreakglassHistoryCommand(global *GlobalOptions) *cobra.Command {
	var since, until, operator, status, approval, decidedBy, decidedSince, target string
	var limit int
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Break-glass requests across all operators",
		Long: `Fleet-wide break-glass history on this deployment (RC + MC). Audited on the API (like zoa session history).

Optional --target filters one cluster (same as zoa runs). Fleet list; does not set mine=true.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := getClient(global)
			if err != nil {
				return err
			}
			list, err := c.ListBreakglassRequests(cmd.Context(), &client.BreakglassListParams{
				Target:       target,
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
	cmd.Flags().StringVarP(&target, "target", "t", "", "Filter by target cluster (default: all targets in deployment)")
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
		Long:  `Get request status (stdout metadata, like zoa get). With --wait, poll until lifecycle active, denied, or expired. Does not activate — use zoa breakglass activate after approval.`,
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
	meta := map[string]string{"request_id": requestID, "name": resp.Name, "grant_expires_at": resp.GrantExpiresAt}
	metaBytes, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), metaBytes, 0o600); err != nil {
		return err
	}
	plane, err := active.ScopeForProfile(resp.Name)
	if err != nil {
		return err
	}
	stanza := active.Stanza{
		RequestID:           requestID,
		Name:                  resp.Name,
		GrantExpiresAt:      resp.GrantExpiresAt,
		CredentialExpiresAt: breakglassCredentialExpiresAt(plane, resp),
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
	if err := validateBreakglassActivateArtifacts(home, requestID, plane, resp); err != nil {
		return err
	}
	progress := breakglassProgressWriter()
	printBreakglassActivateDispatched(progress, requestID, resp.TargetCluster, resp.Name)
	printBreakglassActivateHints(progress, home, requestID, plane, resp)
	if global.OutputFormat == output.FormatJSON {
		id := resp.ID
		if id == "" {
			id = requestID
		}
		safe := map[string]interface{}{
			"id":             id,
			"request_id":     requestID,
			"name":           resp.Name,
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
