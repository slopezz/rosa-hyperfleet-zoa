package scopes

import "testing"

func TestNeedsApproval_WhenAuto_ItShouldNotRequireApproval(t *testing.T) {
	if NeedsApproval(Authorization{Approval: "auto"}) {
		t.Fatal("auto should not need approval")
	}
}

func TestNeedsApproval_WhenStrict_ItShouldRequireApproval(t *testing.T) {
	if !NeedsApproval(Authorization{Approval: "strict"}) {
		t.Fatal("strict should need approval")
	}
}
