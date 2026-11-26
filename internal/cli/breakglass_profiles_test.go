package cli

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
	"github.com/openshift-online/rosa-seaplane-zoa/internal/output"
)

func TestRenderBreakglassProfiles_WhenTable_ItShouldPrintHeaderRow(t *testing.T) {
	list := &client.BreakglassProfileList{
		Items: []client.ProfileMetadata{{
			Name:        "kube_read",
			Scope:       "kube",
			Type:        "read",
			Description: "read-only",
			Authorization: map[string]interface{}{
				"approval":              "none",
				"default_request_hours": 24,
				"default_grant_hours":   4,
			},
		}},
		Count: 1,
	}
	out := captureStdout(t, func() {
		if err := renderBreakglassProfiles(output.FormatTable, list); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "NAME") || !strings.Contains(out, "SCOPE") || !strings.Contains(out, "APPROVAL") {
		t.Fatalf("missing headers: %q", out)
	}
	if !strings.Contains(out, "kube_read") || !strings.Contains(out, "none") {
		t.Fatalf("missing row: %q", out)
	}
}

func TestRenderBreakglassProfiles_WhenJSON_ItShouldEmitListEnvelope(t *testing.T) {
	list := &client.BreakglassProfileList{Items: []client.ProfileMetadata{{Name: "aws_read"}}, Count: 1}
	out := captureStdout(t, func() {
		if err := renderBreakglassProfiles(output.FormatJSON, list); err != nil {
			t.Fatal(err)
		}
	})
	var decoded client.BreakglassProfileList
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Count != 1 || len(decoded.Items) != 1 || decoded.Items[0].Name != "aws_read" {
		t.Fatalf("unexpected json: %+v", decoded)
	}
}

func TestDescribeBreakglassProfile_WhenMarkdown_ItShouldRejectCatalogOnly(t *testing.T) {
	global := &GlobalOptions{OutputFormat: output.FormatMarkdown}
	opts := &catalogOptions{offline: true, targetType: "rc"}
	err := describeBreakglassProfile(context.Background(), global, opts, "kube_read")
	if err == nil || !strings.Contains(err.Error(), "-o markdown is only for the offline catalog") {
		t.Fatalf("expected catalog-only markdown error, got %v", err)
	}
	if !strings.Contains(err.Error(), "profiles --offline -o markdown") {
		t.Fatalf("expected break-glass catalog hint, got %v", err)
	}
}
