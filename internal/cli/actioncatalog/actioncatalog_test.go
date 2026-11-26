package actioncatalog

import (
	"strings"
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"
)

func TestMarkdown_WhenTargetTypeRC_ItShouldIncludeRCContext(t *testing.T) {
	body, err := Markdown(actions.TargetTypeRC)
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
	if !strings.Contains(body, "Global `zoa run` flags") {
		t.Fatal("expected global run flags section")
	}
	if !strings.Contains(body, "**Timeout:**") {
		t.Fatal("expected timeout in action section")
	}
}

func TestMarkdown_WhenTargetTypeRCAndMC_ItShouldDifferInEffectiveRBAC(t *testing.T) {
	rcBody, err := Markdown(actions.TargetTypeRC)
	if err != nil {
		t.Fatal(err)
	}
	mcBody, err := Markdown(actions.TargetTypeMC)
	if err != nil {
		t.Fatal(err)
	}
	if rcBody == mcBody {
		t.Fatal("expected rc and mc break-glass catalogs to differ (profile/RBAC filtering)")
	}
}

func TestList_WhenTargetTypeMC_ItShouldReturnActions(t *testing.T) {
	items, err := List(actions.TargetTypeMC)
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
	_, err := Get(actions.TargetTypeRC, "not_a_real_action_xyz")
	if err == nil {
		t.Fatal("expected error")
	}
}
