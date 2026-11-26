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
