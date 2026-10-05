package parambind

import (
	"testing"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/client"
)

func TestCLILineForParam_WhenDedicatedFlagExists_ItShouldReturnFlagSyntax(t *testing.T) {
	if got := CLILineForParam("namespace"); got == "" || got[0] != '-' {
		t.Fatalf("expected dedicated flag line, got %q", got)
	}
}

func TestBindingForParam_WhenNoDedicatedFlag_ItShouldMarkViaParam(t *testing.T) {
	b := BindingForParam(client.ActionParam{Name: "field_selector", Description: "K8s field selector"})
	if !b.ViaParam {
		t.Fatal("expected via_param for field_selector")
	}
	if b.CLI != "--param field_selector=" {
		t.Fatalf("unexpected CLI: %q", b.CLI)
	}
}

func TestRunFlagHelp_WhenKnownFlag_ItShouldMatchRunModifiers(t *testing.T) {
	help := RunFlagHelp("no-wait")
	if help == "" {
		t.Fatal("expected no-wait help")
	}
	for _, m := range RunModifiers() {
		if m.Flags == "--no-wait" && m.Description != help {
			t.Fatalf("RunModifiers and RunFlagHelp diverged for no-wait: %q vs %q", m.Description, help)
		}
	}
}

func TestEnrichDescribe_WhenActionHasParams_ItShouldIncludeRunModifiers(t *testing.T) {
	view := EnrichDescribe(&client.Action{
		Name:   "get_resource",
		Params: []client.ActionParam{{Name: "resource", Required: true}},
	})
	if len(view.CLI.RunModifiers) == 0 {
		t.Fatal("expected run modifiers")
	}
	if len(view.CLI.Parameters) != 1 {
		t.Fatalf("expected 1 parameter binding, got %d", len(view.CLI.Parameters))
	}
}
