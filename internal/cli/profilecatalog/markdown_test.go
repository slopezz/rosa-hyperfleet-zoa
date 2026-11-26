package profilecatalog

import (
	"strings"
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"
)

func TestMarkdown_WhenRC_ItShouldIncludeAgentRulesAndProfiles(t *testing.T) {
	body, err := Markdown(actions.TargetTypeRC)
	if err != nil {
		t.Fatalf("Markdown: %v", err)
	}
	for _, sub := range []string{
		"Quick rules",
		"Break-glass request flags",
		"**Request TTL:**",
		"**Grant TTL:**",
		"**Credential TTL:**",
		"**RBAC**",
		"apiGroups=",
		"**Scope:**",
		"request kube_read",
		"kube_read",
		"```bash",
	} {
		if !strings.Contains(body, sub) {
			t.Fatalf("expected markdown to contain %q", sub)
		}
	}
}

func TestMarkdown_WhenRCAndMC_ItShouldDifferInKubeReadRBAC(t *testing.T) {
	rcBody, err := Markdown(actions.TargetTypeRC)
	if err != nil {
		t.Fatal(err)
	}
	mcBody, err := Markdown(actions.TargetTypeMC)
	if err != nil {
		t.Fatal(err)
	}
	if rcBody == mcBody {
		t.Fatal("expected rc and mc profile catalogs to differ (EffectiveRBAC)")
	}
	if !strings.Contains(rcBody, "**Target type:** `rc`") || !strings.Contains(mcBody, "**Target type:** `mc`") {
		t.Fatal("expected target type in catalog header")
	}
}
