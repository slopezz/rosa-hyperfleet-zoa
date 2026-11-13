package cli

import (
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
)

func TestBreakglassLifecycleStatus_WhenPending_ItShouldEchoStatus(t *testing.T) {
	req := &client.BreakglassRequest{Status: "pending"}
	if got := breakglassLifecycleStatus(req); got != "pending" {
		t.Fatalf("got %q", got)
	}
}

func TestBreakglassWorkflowApproval_WhenApproved_ItShouldNotFoldActor(t *testing.T) {
	req := &client.BreakglassRequest{Status: "approved", Approval: "approved", DecidedBy: "bob"}
	if got := breakglassWorkflowApproval(req); got != "approved" {
		t.Fatalf("got %q", got)
	}
	if got := breakglassDecidedBy(req); got != "bob" {
		t.Fatalf("got %q", got)
	}
}

func TestBreakglassDecidedAt_WhenRejected_ItShouldPreferUnifiedField(t *testing.T) {
	req := &client.BreakglassRequest{
		Status:    "rejected",
		Approval:  "rejected",
		DecidedAt: "2026-11-13T12:00:00Z",
	}
	if got := breakglassDecidedAt(req); got != "2026-11-13T12:00:00Z" {
		t.Fatalf("got %q", got)
	}
}

func TestBreakglassValidUntil_WhenPending_ItShouldUseRequestExpires(t *testing.T) {
	req := &client.BreakglassRequest{
		Status:           "pending",
		RequestExpiresAt: "2026-11-14T10:00:00Z",
		GrantExpiresAt:   "2026-11-14T14:00:00Z",
	}
	if got := breakglassValidUntil(req); got != "2026-11-14T10:00:00Z" {
		t.Fatalf("got %q", got)
	}
}
