package breakglass

import "testing"

func TestNeedsApproval_WhenNone_ItShouldNotRequireApproval(t *testing.T) {
	if NeedsApproval(Authorization{Approval: "none"}) {
		t.Fatal("none should not need approval")
	}
}

func TestNeedsApproval_WhenManual_ItShouldRequireApproval(t *testing.T) {
	if !NeedsApproval(Authorization{Approval: "manual"}) {
		t.Fatal("non-none should need approval (future workflow)")
	}
}
