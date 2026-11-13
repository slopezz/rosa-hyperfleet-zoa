package api

import (
	"context"
	"fmt"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/breakglass"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/store"
)

// BreakglassAutoApprover is the synthetic approver when the scope catalog skips peer workflow (approval "none").
const BreakglassAutoApprover = "auto"

const breakglassAutoApproveNote = "break-glass auto-approve (approval workflow epic pending)"

// BreakglassSkipsPeerApproval is true when the scope metadata does not require peer approve before activate
// (same idea as Trusted Actions with authorization.approval "none"). No Lambda env override.
func BreakglassSkipsPeerApproval(auth breakglass.Authorization) bool {
	return !breakglass.NeedsApproval(auth)
}

// ApproveBreakglassOnCreateWhenNoPeerWorkflow runs ApproveBreakglassRequest immediately after create when the
// scope catalog allows it — same transition as laptop approve, synthetic approver for audit.
func ApproveBreakglassOnCreateWhenNoPeerWorkflow(
	ctx context.Context,
	bgStore store.BreakglassStore,
	req *store.BreakglassRequest,
	def *breakglass.Definition,
) (*store.BreakglassRequest, error) {
	if req == nil || def == nil {
		return req, nil
	}
	if !BreakglassSkipsPeerApproval(def.Metadata.Authorization) {
		return req, nil
	}
	note := ""
	if def.Metadata.Authorization.RequireDecisionNote {
		note = breakglassAutoApproveNote
	}
	if err := ApproveBreakglassRequest(ctx, bgStore, req, BreakglassAutoApprover, note); err != nil {
		return nil, fmt.Errorf("approve break-glass on create (no peer workflow): %w", err)
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
