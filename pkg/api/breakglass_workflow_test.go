package api

import (
	"context"
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/breakglass"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/store"
)

func TestBreakglassSkipsPeerApproval_WhenNone_ItShouldSkipPeerWorkflow(t *testing.T) {
	if !BreakglassSkipsPeerApproval(breakglass.Authorization{Approval: "none"}) {
		t.Fatal("expected skip peer when approval is none")
	}
}

func TestBreakglassSkipsPeerApproval_WhenWorkflowRequired_ItShouldRequirePeer(t *testing.T) {
	if BreakglassSkipsPeerApproval(breakglass.Authorization{Approval: "manual"}) {
		t.Fatal("expected peer workflow when approval is not none")
	}
}

func TestApproveBreakglassOnCreateWhenNoPeerWorkflow_ItShouldTransitionViaApproveWorkflow(t *testing.T) {
	req := &store.BreakglassRequest{
		RequestID:        "req-1",
		Scope:            "kube-read",
		Status:           store.StatusPendingApproval,
		Operator:         "alice",
		RequestExpiresAt: "2099-01-01T00:00:00Z",
	}
	st := &mockBreakglassServiceStore{req: req}
	def := &breakglass.Definition{
		Metadata: breakglass.ScopeMetadata{
			Authorization: breakglass.Authorization{
				Approval:            "none",
				DefaultGrantHours:   4,
				DefaultPendingHours: 24,
			},
		},
	}
	out, err := ApproveBreakglassOnCreateWhenNoPeerWorkflow(context.Background(), st, req, def)
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != store.StatusApproved {
		t.Fatalf("expected approved, got %s", out.Status)
	}
	if out.ApprovedBy != BreakglassAutoApprover {
		t.Fatalf("expected auto approver, got %q", out.ApprovedBy)
	}
}

func TestApproveBreakglassRequest_WhenAutoApproverOnStrictNoteScope_ItShouldAllowEmptyPeerNote(t *testing.T) {
	req := &store.BreakglassRequest{
		RequestID:        "req-2",
		Scope:            "kube-elevated",
		Status:           store.StatusPendingApproval,
		Operator:         "alice",
		RequestExpiresAt: "2099-01-01T00:00:00Z",
	}
	store := &mockBreakglassServiceStore{req: req}
	err := ApproveBreakglassRequest(context.Background(), store, req, BreakglassAutoApprover, breakglassAutoApproveNote)
	if err != nil {
		t.Fatalf("auto approve with synthetic note: %v", err)
	}
}
