package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/breakglass"
)

func TestPrintBreakglassActivateHints_WhenKubePlane_ItShouldMentionRefreshWithoutExport(t *testing.T) {
	var buf bytes.Buffer
	resp := &client.BreakglassActivateResponse{
		GrantExpiresAt:     "2026-11-13T18:18:23.699030131Z",
		KubeTokenExpiresAt: "2026-11-13T15:19:00Z",
	}
	printBreakglassActivateHints(&buf, "/home/sre", "req-1", breakglass.PlaneKube, resp)
	out := buf.String()
	if strings.Contains(out, "export now") {
		t.Fatalf("SRE should not see export lines, got %q", out)
	}
	if !strings.Contains(out, "refresh:") || !strings.Contains(out, "req-1") {
		t.Fatalf("expected refresh hint, got %q", out)
	}
	if strings.Contains(out, "KUBECONFIG") || strings.Contains(out, "export") {
		t.Fatalf("must not expose env wiring, got %q", out)
	}
}

func TestPrintBreakglassActivateHints_WhenAWSPlane_ItShouldShowUntilAndRefreshOnly(t *testing.T) {
	var buf bytes.Buffer
	resp := &client.BreakglassActivateResponse{
		GrantExpiresAt: "2026-11-13T18:24:22Z",
		AWS: map[string]interface{}{
			"expiration": "2026-11-13T15:25:00Z",
		},
	}
	printBreakglassActivateHints(&buf, "/home/sre", "req-2", breakglass.PlaneAWS, resp)
	out := buf.String()
	if !strings.Contains(out, "until") || !strings.Contains(out, "refresh:") {
		t.Fatalf("expected until + refresh, got %q", out)
	}
}
