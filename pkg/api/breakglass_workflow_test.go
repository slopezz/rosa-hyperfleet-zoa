package api

import (
	"context"
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/breakglass"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/store"
)

func TestShouldAutoApproveBreakglass_WhenNone_ItShouldAutoApprove(t *testing.T) {
	if !ShouldAutoApproveBreakglass(breakglass.Authorization{Approval: "none"}) {
		t.Fatal("expected auto when approval is none")
	}
}

func TestShouldAutoApproveBreakglass_WhenWorkflowRequired_ItShouldStayPending(t *testing.T) {
	if ShouldAutoApproveBreakglass(breakglass.Authorization{Approval: "manual"}) {
		t.Fatal("expected pending when approval workflow required")
	}
}

func TestMaybeAutoApproveBreakglassAfterCreate_ItShouldTransitionViaApproveWorkflow(t *testing.T) {
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
	out, err := MaybeAutoApproveBreakglassAfterCreate(context.Background(), st, req, def)
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
