package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
	"github.com/openshift-online/rosa-seaplane-zoa/internal/output"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/scopes"
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
		newBreakglassRequestCommand(global),
		newBreakglassRequestsCommand(global),
		newBreakglassGetCommand(global),
		newBreakglassConnectCommand(global),
		newBreakglassUseCommand(global),
	)
	return cmd
}

func newBreakglassScopesCommand(global *GlobalOptions) *cobra.Command {
	var offline bool
	cmd := &cobra.Command{
		Use:   "scopes [scope]",
		Short: "List or describe break-glass scope catalog",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if offline || global.APIURL == "" {
				return listScopesOffline(global, args)
			}
			c, err := getClient(global)
			if err != nil {
				return err
			}
			if len(args) == 1 {
				return fmt.Errorf("describe scope %q via API: use offline catalog or GET /breakglass/scopes/%s", args[0], args[0])
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

func listScopesOffline(global *GlobalOptions, args []string) error {
	if len(args) == 1 {
		def, ok := scopes.Get(args[0])
		if !ok {
			return fmt.Errorf("unknown scope %q", args[0])
		}
		return output.JSON(os.Stdout, def.Metadata)
	}
	items := scopes.List()
	return renderBreakglassScopes(global.OutputFormat, toClientScopes(items))
}

func toClientScopes(in []scopes.ScopeMetadata) []client.ScopeMetadata {
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
	var scope, reason string
	cmd := &cobra.Command{
		Use:   "request",
		Short: "Create a break-glass request",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := getClient(global)
			if err != nil {
				return err
			}
			resp, err := c.CreateBreakglassRequest(cmd.Context(), scope, reason)
			if err != nil {
				return err
			}
			return output.JSON(os.Stdout, resp)
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "", "Break-glass scope id (required)")
	cmd.Flags().StringVar(&reason, "reason", "", "Incident reason (required)")
	_ = cmd.MarkFlagRequired("scope")
	_ = cmd.MarkFlagRequired("reason")
	return cmd
}

func newBreakglassRequestsCommand(global *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "requests",
		Short:   "List your break-glass requests",
		Aliases: []string{"list"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := getClient(global)
			if err != nil {
				return err
			}
			list, err := c.ListBreakglassRequests(cmd.Context())
			if err != nil {
				return err
			}
			return output.JSON(os.Stdout, list)
		},
	}
	return cmd
}

func newBreakglassGetCommand(global *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <request-id>",
		Short: "Get a break-glass request",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := getClient(global)
			if err != nil {
				return err
			}
			resp, err := c.GetBreakglassRequest(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return output.JSON(os.Stdout, resp)
		},
	}
	return cmd
}

func newBreakglassConnectCommand(global *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connect <request-id>",
		Short: "Provision break-glass access for an approved request",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := getClient(global)
			if err != nil {
				return err
			}
			resp, err := c.ConnectBreakglass(cmd.Context(), args[0], false)
			if err != nil {
				return err
			}
			return output.JSON(os.Stdout, resp)
		},
	}
	return cmd
}

func newBreakglassUseCommand(global *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "use <request-id>",
		Short: "Activate break-glass credentials for kubectl/aws",
		Args:  cobra.ExactArgs(1),
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
	resp, err := c.ConnectBreakglass(ctx, requestID, true)
	if err != nil {
		return err
	}
	dir := filepath.Join(os.Getenv("HOME"), ".config", "zoa", "breakglass", requestID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	active := filepath.Join(os.Getenv("HOME"), ".config", "zoa", "breakglass", "active")
	meta := map[string]string{"request_id": requestID, "scope": resp.Scope, "grant_expires_at": resp.GrantExpiresAt}
	metaBytes, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), metaBytes, 0o600); err != nil {
		return err
	}
	_ = os.WriteFile(active, []byte(requestID), 0o600)
	if resp.KubeToken != "" {
		_ = writeKubeconfig(dir, resp)
	}
	if global.OutputFormat == output.FormatJSON {
		safe := map[string]interface{}{"request_id": requestID, "scope": resp.Scope, "status": resp.Status, "config_dir": dir}
		return output.JSON(os.Stdout, safe)
	}
	fmt.Fprintf(os.Stderr, "break-glass active for request %s (scope %s)\n", requestID, resp.Scope)
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
