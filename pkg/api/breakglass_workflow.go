package api

import (
	"context"
	"fmt"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/breakglass"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/store"
)

// BreakglassAutoApprover is the synthetic approver for deployment-wide auto-approve (pending → approved transition).
const BreakglassAutoApprover = "auto"

const breakglassAutoApproveNote = "break-glass auto-approve (approval workflow epic pending)"

// ShouldAutoApproveBreakglass reports whether create should invoke ApproveBreakglassRequest immediately.
// True when scope authorization is approval "none" (no peer workflow). Does not consult Lambda env overrides.
func ShouldAutoApproveBreakglass(auth breakglass.Authorization) bool {
	return !breakglass.NeedsApproval(auth)
}

// MaybeAutoApproveBreakglassAfterCreate runs the same ApproveBreakglassRequest path used for peer approve
// when the resolved scope does not require a workflow (approval none).
func MaybeAutoApproveBreakglassAfterCreate(
	ctx context.Context,
	bgStore store.BreakglassStore,
	req *store.BreakglassRequest,
	def *breakglass.Definition,
) (*store.BreakglassRequest, error) {
	if req == nil || def == nil {
		return req, nil
	}
	if !ShouldAutoApproveBreakglass(def.Metadata.Authorization) {
		return req, nil
	}
	note := ""
	if def.Metadata.Authorization.RequireDecisionNote {
		note = breakglassAutoApproveNote
	}
	if err := ApproveBreakglassRequest(ctx, bgStore, req, BreakglassAutoApprover, note); err != nil {
		return nil, fmt.Errorf("auto-approve break-glass: %w", err)
	}
	updated, err := bgStore.Get(ctx, req.RequestID)
	if err != nil {
		return nil, err
	}
	if updated == nil {
		return req, nil
	}
	return updated, nil
}
