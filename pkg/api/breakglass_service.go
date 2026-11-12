package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/scopes"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/store"
)

var (
	errBreakglassNotPending     = errors.New("breakglass: not pending approval")
	errBreakglassRequestExpired = errors.New("breakglass: request expired")
	errBreakglassNoApprover     = errors.New("breakglass: missing approver")
	errBreakglassSelfApprove    = errors.New("breakglass: self approval")
	errBreakglassNoteRequired   = errors.New("breakglass: note required")
)

// ApproveBreakglassRequest transitions pending → approved with grant expiry.
func ApproveBreakglassRequest(ctx context.Context, bgStore store.BreakglassStore, req *store.BreakglassRequest, approver, note string) error {
	if req.Status != store.StatusPendingApproval {
		return errBreakglassNotPending
	}
	if req.RequestExpiresAt != "" {
		exp, err := time.Parse(time.RFC3339Nano, req.RequestExpiresAt)
		if err == nil && time.Now().After(exp) {
			return errBreakglassRequestExpired
		}
	}
	if approver == "" {
		return errBreakglassNoApprover
	}
	if req.Operator != "" && req.Operator == approver {
		return errBreakglassSelfApprove
	}
	def, _ := scopes.Get(req.Scope)
	if def != nil && def.Metadata.Authorization.RequireDecisionNote && note == "" {
		return errBreakglassNoteRequired
	}
	grantHours := 4
	if def != nil && def.Metadata.Authorization.DefaultGrantHours > 0 {
		grantHours = def.Metadata.Authorization.DefaultGrantHours
	}
	now := time.Now()
	grantExpires := now.Add(time.Duration(grantHours) * time.Hour)
	updates := map[string]interface{}{
		"approvedBy":     approver,
		"approvedAt":     now.Format(time.RFC3339Nano),
		"grantExpiresAt": grantExpires.Format(time.RFC3339Nano),
		"decisionNote":   note,
	}
	return bgStore.Transition(ctx, req.RequestID, store.StatusPendingApproval, store.StatusApproved, updates)
}

// RejectBreakglassRequest transitions pending → rejected.
func RejectBreakglassRequest(ctx context.Context, bgStore store.BreakglassStore, req *store.BreakglassRequest, approver, note string) error {
	if req.Status != store.StatusPendingApproval {
		return errBreakglassNotPending
	}
	if approver == "" {
		return errBreakglassNoApprover
	}
	def, _ := scopes.Get(req.Scope)
	if def != nil && def.Metadata.Authorization.RequireDecisionNote && note == "" {
		return errBreakglassNoteRequired
	}
	now := time.Now()
	updates := map[string]interface{}{
		"rejectedBy":   approver,
		"rejectedAt":   now.Format(time.RFC3339Nano),
		"decisionNote": note,
	}
	return bgStore.Transition(ctx, req.RequestID, store.StatusPendingApproval, store.StatusRejected, updates)
}

func writeBreakglassDecisionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errBreakglassNotPending):
		writeError(w, http.StatusConflict, "invalid_status", "request is not pending approval")
	case errors.Is(err, errBreakglassRequestExpired):
		writeError(w, http.StatusGone, "request_expired", "pending request expired")
	case errors.Is(err, errBreakglassNoApprover):
		writeError(w, http.StatusUnauthorized, "unauthorized", "could not resolve approver identity")
	case errors.Is(err, errBreakglassSelfApprove):
		writeError(w, http.StatusForbidden, "self_approval", "requester cannot approve own request")
	case errors.Is(err, errBreakglassNoteRequired):
		writeError(w, http.StatusBadRequest, "note_required", "decision note is required for this scope")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to update break-glass request")
	}
}
