package actioncatalog

import (
	"strings"
	"testing"

	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/actions"
)

func TestMarkdown_WhenTargetTypeRC_ItShouldIncludeRCContext(t *testing.T) {
	body, err := Markdown(actions.DeploymentTargetRC)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "**Target type:** `rc`") {
		t.Fatal("expected rc target type header")
	}
	if !strings.Contains(body, "platform-api") {
		t.Fatal("expected RC namespace hints")
	}
	if !strings.Contains(body, "## get_resource") {
		t.Fatal("expected get_resource section")
	}
}

func TestList_WhenTargetTypeMC_ItShouldReturnActions(t *testing.T) {
	items, err := List(actions.DeploymentTargetMC)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 {
		t.Fatal("expected actions for mc")
	}
	found := false
	for _, a := range items {
		if a.Name == "must_gather" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected must_gather on mc")
	}
}

func TestGet_WhenActionMissing_ItShouldError(t *testing.T) {
	_, err := Get(actions.DeploymentTargetRC, "not_a_real_action_xyz")
	if err == nil {
		t.Fatal("expected error")
	}
}
