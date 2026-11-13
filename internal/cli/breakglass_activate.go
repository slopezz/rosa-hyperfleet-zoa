package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/breakglass"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/breakglass/active"
)

func validateBreakglassActivateArtifacts(home, requestID, plane string, resp *client.BreakglassActivateResponse) error {
	dir := active.RequestDir(home, requestID)
	switch plane {
	case breakglass.PlaneKube:
		if resp.KubeToken == "" {
			return fmt.Errorf("activate: API returned no kube token for scope %q", resp.Scope)
		}
		kcfg := filepath.Join(dir, "kubeconfig")
		if _, err := os.Stat(kcfg); err != nil {
			return fmt.Errorf("activate: kubeconfig not written at %s", kcfg)
		}
	case breakglass.PlaneAWS:
		if resp.AWS == nil || fmt.Sprint(resp.AWS["access_key_id"]) == "" {
			return fmt.Errorf("activate: API returned no AWS credentials for scope %q (check BREAKGLASS_AWS_*_ROLE_ARN on API Lambda)", resp.Scope)
		}
		creds := active.AWSCredentialsPath(home)
		if _, err := os.Stat(creds); err != nil {
			return fmt.Errorf("activate: AWS credentials file not written at %s", creds)
		}
	}
	return nil
}

// breakglassCredExpireNote returns a short line for when bearer creds expire (kube ~1h cap; STS from API).
func breakglassCredExpireNote(plane string, resp *client.BreakglassActivateResponse) string {
	switch plane {
	case breakglass.PlaneKube:
		if resp.KubeTokenExpiresAt != "" {
			if t, err := time.Parse(time.RFC3339Nano, resp.KubeTokenExpiresAt); err == nil {
				return formatBreakglassCredTTL(time.Until(t))
			}
		}
		return "1h"
	case breakglass.PlaneAWS:
		if resp.AWS != nil {
			if raw, ok := resp.AWS["expiration"].(string); ok && raw != "" {
				if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
					return formatBreakglassCredTTL(time.Until(t))
				}
			}
		}
		return "1h"
	default:
		return "1h"
	}
}

func breakglassBearerExpiresAt(plane string, resp *client.BreakglassActivateResponse) string {
	switch plane {
	case breakglass.PlaneKube:
		return resp.KubeTokenExpiresAt
	case breakglass.PlaneAWS:
		if resp.AWS != nil {
			if raw, ok := resp.AWS["expiration"].(string); ok {
				return raw
			}
		}
	}
	return ""
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

// printBreakglassActivateHints explains grant vs bearer clocks (no manual export; shell wires env).
func printBreakglassActivateHints(w io.Writer, home, requestID, plane string, resp *client.BreakglassActivateResponse) {
	_ = home
	credTTL := breakglassCredExpireNote(plane, resp)
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
