package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/breakglass"
)

func TestPrintBreakglassActivateDispatched_WhenComplete_ItShouldAppendActivatedSuffix(t *testing.T) {
	var buf bytes.Buffer
	printBreakglassActivateDispatched(&buf, "req-1", "eph-regional", "aws_read")
	want := "✓ req-1 [eph-regional] profile=aws_read · activated\n"
	if buf.String() != want {
		t.Fatalf("activate line = %q, want %q", buf.String(), want)
	}
}

func TestPrintBreakglassActivateHints_WhenKubePlane_ItShouldMentionRefreshWithoutExport(t *testing.T) {
	var buf bytes.Buffer
	resp := &client.BreakglassActivateResponse{
		GrantExpiresAt:     "2026-11-13T18:18:23.699030131Z",
		KubeTokenExpiresAt: "2026-11-13T15:19:00Z",
	}
	printBreakglassActivateHints(&buf, "/home/sre", "req-1", breakglass.ScopeKube, resp)
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
	printBreakglassActivateHints(&buf, "/home/sre", "req-2", breakglass.ScopeAWS, resp)
	out := buf.String()
	if !strings.Contains(out, "until") || !strings.Contains(out, "refresh:") {
		t.Fatalf("expected until + refresh, got %q", out)
	}
}

func TestWriteBreakglassAWSProfile_WhenCalled_ItShouldWriteDefaultProfileOnly(t *testing.T) {
	home := t.TempDir()
	aws := map[string]interface{}{
		"access_key_id":     "AKIATEST",
		"secret_access_key": "secret",
		"session_token":     "token",
		"region":            "us-east-1",
	}
	if err := writeBreakglassAWSProfile(home, aws); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".config", "zoa", "breakglass", "aws-credentials"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, "[default]") {
		t.Fatalf("expected [default]: %s", body)
	}
	if strings.Contains(body, "zoa-bg-") {
		t.Fatalf("expected no named bg profile: %s", body)
	}
	if !strings.Contains(body, "AKIATEST") {
		t.Fatalf("missing access key: %s", body)
	}
	if err := writeBreakglassAWSProfile(home, map[string]interface{}{
		"access_key_id":     "AKIASECOND",
		"secret_access_key": "s2",
		"session_token":     "t2",
	}); err != nil {
		t.Fatal(err)
	}
	data2, _ := os.ReadFile(filepath.Join(home, ".config", "zoa", "breakglass", "aws-credentials"))
	if strings.Contains(string(data2), "AKIATEST") {
		t.Fatalf("expected replace not merge: %s", data2)
	}
}
