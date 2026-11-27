package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/cli/breakglassactivegrants"
	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
	"github.com/openshift-online/rosa-seaplane-zoa/internal/output"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/breakglass"
)

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
	dir := breakglassactivegrants.RequestDir(home, requestID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	meta := map[string]string{"request_id": requestID, "profile": resp.Profile, "grant_expires_at": resp.GrantExpiresAt}
	metaBytes, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), metaBytes, 0o600); err != nil {
		return err
	}
	scope, err := breakglassactivegrants.ScopeForProfile(resp.Profile)
	if err != nil {
		return err
	}
	stanza := breakglassactivegrants.GrantStanza{
		RequestID:           requestID,
		Profile:             resp.Profile,
		GrantExpiresAt:      resp.GrantExpiresAt,
		CredentialExpiresAt: breakglassCredentialExpiresAt(scope, resp),
	}
	if err := breakglassactivegrants.SetGrant(home, stanza); err != nil {
		return err
	}
	if resp.KubeToken != "" {
		if err := writeBreakglassKubeconfig(dir, resp); err != nil {
			return err
		}
	}
	if err := writeBreakglassAWSProfile(home, resp.AWS); err != nil {
		return err
	}
	if err := validateBreakglassActivateArtifacts(home, requestID, scope, resp); err != nil {
		return err
	}
	progress := breakglassProgressWriter()
	printBreakglassActivateDispatched(progress, requestID, resp.TargetCluster, resp.Profile)
	printBreakglassActivateHints(progress, home, requestID, scope, resp)
	if global.OutputFormat == output.FormatJSON {
		id := resp.ID
		if id == "" {
			id = requestID
		}
		safe := map[string]interface{}{
			"id":             id,
			"request_id":     requestID,
			"profile":        resp.Profile,
			"target_cluster": resp.TargetCluster,
			"status":         resp.Status,
			"config_dir":     dir,
		}
		return output.JSON(os.Stdout, safe)
	}
	return nil
}

func validateBreakglassActivateArtifacts(home, requestID, scope string, resp *client.BreakglassActivateResponse) error {
	dir := breakglassactivegrants.RequestDir(home, requestID)
	switch scope {
	case breakglass.ScopeKube:
		if resp.KubeToken == "" {
			return fmt.Errorf("activate: API returned no kube token for profile %q", resp.Profile)
		}
		kcfg := filepath.Join(dir, "kubeconfig")
		if _, err := os.Stat(kcfg); err != nil {
			return fmt.Errorf("activate: kubeconfig not written at %s", kcfg)
		}
	case breakglass.ScopeAWS:
		if resp.AWS == nil || fmt.Sprint(resp.AWS["access_key_id"]) == "" {
			return fmt.Errorf("activate: API returned no AWS credentials for profile %q (check BREAKGLASS_AWS_*_ROLE_ARN on API Lambda)", resp.Profile)
		}
		creds := breakglassactivegrants.AWSCredentialsPath(home)
		if _, err := os.Stat(creds); err != nil {
			return fmt.Errorf("activate: AWS credentials file not written at %s", creds)
		}
	}
	return nil
}

func writeBreakglassKubeconfig(dir string, resp *client.BreakglassActivateResponse) error {
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

// writeBreakglassAWSProfile writes ~/.config/zoa/breakglass/aws-credentials as [default]
// (single active aws grant; plain `aws` CLI, no AWS_PROFILE).
func writeBreakglassAWSProfile(home string, aws map[string]interface{}) error {
	if aws == nil {
		return nil
	}
	accessKey := strings.TrimSpace(fmt.Sprint(aws["access_key_id"]))
	if accessKey == "" {
		return nil
	}
	secretKey := fmt.Sprint(aws["secret_access_key"])
	sessionToken := fmt.Sprint(aws["session_token"])
	region := fmt.Sprint(aws["region"])
	block := fmt.Sprintf("[default]\naws_access_key_id = %s\naws_secret_access_key = %s\naws_session_token = %s\n",
		accessKey, secretKey, sessionToken)
	if region != "" && region != "<nil>" {
		block += fmt.Sprintf("region = %s\n", region)
	}
	dir := breakglassactivegrants.ConfigDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := breakglassactivegrants.AWSCredentialsPath(home)
	return os.WriteFile(path, []byte(block), 0o600)
}

func breakglassCredentialExpiresAt(scope string, resp *client.BreakglassActivateResponse) string {
	switch scope {
	case breakglass.ScopeKube:
		return resp.KubeTokenExpiresAt
	case breakglass.ScopeAWS:
		if resp.AWS != nil {
			if raw, ok := resp.AWS["expiration"].(string); ok {
				return raw
			}
		}
	}
	return ""
}

func breakglassCredExpireNote(scope string, resp *client.BreakglassActivateResponse) string {
	switch scope {
	case breakglass.ScopeKube:
		if resp.KubeTokenExpiresAt != "" {
			if t, err := time.Parse(time.RFC3339Nano, resp.KubeTokenExpiresAt); err == nil {
				return formatBreakglassCredTTL(time.Until(t))
			}
		}
		return breakglass.CredentialMaxTTLDisplay(scope)
	case breakglass.ScopeAWS:
		if resp.AWS != nil {
			if raw, ok := resp.AWS["expiration"].(string); ok && raw != "" {
				if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
					return formatBreakglassCredTTL(time.Until(t))
				}
			}
		}
		return breakglass.CredentialMaxTTLDisplay(scope)
	default:
		return breakglass.CredentialMaxTTLDisplay(scope)
	}
}

func formatBreakglassCredTTL(d time.Duration) string {
	if d < 0 {
		return "0m"
	}
	if d < time.Minute {
		return "1m"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Round(time.Minute)/time.Minute))
	}
	h := int(d.Round(time.Minute) / time.Hour)
	m := int(d.Round(time.Minute)/time.Minute) % 60
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh%dm", h, m)
}

func printBreakglassActivateDispatched(w io.Writer, requestID, targetCluster, profileName string) {
	if requestID == "" {
		return
	}
	if profileName == "" {
		profileName = "unknown"
	}
	fmt.Fprintf(w, "✓ %s [%s] profile=%s · activated\n", requestID, breakglassTargetCluster(targetCluster), profileName)
}

func printBreakglassActivateHints(w io.Writer, home, requestID, scope string, resp *client.BreakglassActivateResponse) {
	_ = home
	credTTL := breakglassCredExpireNote(scope, resp)
	grantLeft := breakglassTimeLeftShort(resp.GrantExpiresAt)
	if resp.GrantExpiresAt != "" {
		fmt.Fprintf(w, "  until %s", formatBreakglassTime(resp.GrantExpiresAt))
		if grantLeft != "" {
			fmt.Fprintf(w, " (%s left)", grantLeft)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "  refresh: zoa breakglass activate %s (before credentials expire, ~%s)\n", requestID, credTTL)
}
