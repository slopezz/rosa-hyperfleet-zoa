package scopecatalog

import (
	"strings"
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"
)

func TestMarkdown_WhenRC_ItShouldIncludeAgentRulesAndScopes(t *testing.T) {
	body, err := Markdown(actions.TargetTypeRC)
	if err != nil {
		t.Fatalf("Markdown: %v", err)
	}
	for _, sub := range []string{
		"Never suggest break-glass first",
		"kube-read",
		"activate",
	} {
		if !strings.Contains(body, sub) {
			t.Fatalf("expected markdown to contain %q", sub)
		}
	}
}
