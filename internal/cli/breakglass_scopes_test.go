package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
	"github.com/openshift-online/rosa-seaplane-zoa/internal/output"
)

func TestRenderBreakglassScopes_WhenTable_ItShouldPrintHeaderRow(t *testing.T) {
	list := &client.BreakglassScopeList{
		Items: []client.ScopeMetadata{{
			ID:          "kube-read",
			Plane:       "kube",
			Type:        "read",
			Description: "read-only",
			Authorization: map[string]interface{}{
				"approval":            "none",
				"default_grant_hours": 4,
			},
		}},
		Count: 1,
	}
	out := captureStdout(t, func() {
		if err := renderBreakglassScopes(output.FormatTable, list); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "SCOPE") || !strings.Contains(out, "APPROVAL") || !strings.Contains(out, "GRANT_TTL") {
		t.Fatalf("missing headers: %q", out)
	}
	if !strings.Contains(out, "kube-read") || !strings.Contains(out, "none") || !strings.Contains(out, "4h") {
		t.Fatalf("missing row: %q", out)
	}
}

func TestRenderBreakglassScopes_WhenJSON_ItShouldEmitListEnvelope(t *testing.T) {
	list := &client.BreakglassScopeList{Items: []client.ScopeMetadata{{ID: "aws-read"}}, Count: 1}
	out := captureStdout(t, func() {
		if err := renderBreakglassScopes(output.FormatJSON, list); err != nil {
			t.Fatal(err)
		}
	})
	var decoded client.BreakglassScopeList
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Count != 1 || len(decoded.Items) != 1 || decoded.Items[0].ID != "aws-read" {
		t.Fatalf("unexpected json: %+v", decoded)
	}
}

func TestScopeCatalogApproval_WhenAuthorizationMissing_ItShouldReturnEmpty(t *testing.T) {
	if scopeCatalogApproval(client.ScopeMetadata{}) != "" {
		t.Fatal("expected empty")
	}
}

func TestScopeCatalogGrantTTL_WhenHoursPresent_ItShouldFormatDuration(t *testing.T) {
	s := client.ScopeMetadata{Authorization: map[string]interface{}{"default_grant_hours": float64(1)}}
	if scopeCatalogGrantTTL(s) != "1h" {
		t.Fatalf("got %q", scopeCatalogGrantTTL(s))
	}
}
