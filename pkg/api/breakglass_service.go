package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/breakglass"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/store"
)

var (
	errBreakglassNotPending     = errors.New("breakglass: not pending approval")
	errBreakglassRequestExpired = errors.New("breakglass: request expired")
	errBreakglassNoApprover     = errors.New("breakglass: missing approver")
	errBreakglassSelfApprove    = errors.New("breakglass: self approval")
	errBreakglassNoteRequired   = errors.New("breakglass: note required")
)

// ApproveBreakglassRequest transitions pending → active with grant expiry (approval=approved).
func ApproveBreakglassRequest(ctx context.Context, bgStore store.BreakglassStore, req *store.BreakglassRequest, approver, note string) error {
	if req.Status != store.StatusPending {
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
	def, _ := breakglass.Get(req.Profile)
	if def != nil && def.Metadata.Authorization.RequireDecisionNote && note == "" && approver != BreakglassAutoApprover {
		return errBreakglassNoteRequired
	}
	grantHours := 4
	if def != nil && def.Metadata.Authorization.DefaultGrantHours > 0 {
		grantHours = def.Metadata.Authorization.DefaultGrantHours
	}
	now := time.Now()
	grantExpires := now.Add(time.Duration(grantHours) * time.Hour)
	updates := map[string]interface{}{
		"approval":       store.WorkflowApprovalApproved,
		"grantExpiresAt": grantExpires.Format(time.RFC3339Nano),
		"decisionNote":   note,
	}
	if approver != BreakglassAutoApprover {
		decidedAt := now.Format(time.RFC3339Nano)
		updates["decidedBy"] = approver
		updates["decidedAt"] = decidedAt
	}
	return bgStore.Transition(ctx, req.RequestID, req.Status, store.StatusActive, updates)
}

// RejectBreakglassRequest transitions pending → denied (approval=rejected).
func RejectBreakglassRequest(ctx context.Context, bgStore store.BreakglassStore, req *store.BreakglassRequest, approver, note string) error {
	if req.Status != store.StatusPending {
		return errBreakglassNotPending
	}
	if approver == "" {
		return errBreakglassNoApprover
	}
	def, _ := breakglass.Get(req.Profile)
	if def != nil && def.Metadata.Authorization.RequireDecisionNote && note == "" {
		return errBreakglassNoteRequired
	}
	now := time.Now()
	decidedAt := now.Format(time.RFC3339Nano)
	updates := map[string]interface{}{
		"approval":     store.WorkflowApprovalRejected,
		"decidedBy":    approver,
		"decidedAt":    decidedAt,
		"decisionNote": note,
	}
	return bgStore.Transition(ctx, req.RequestID, req.Status, store.StatusDenied, updates)
}

func breakglassGrantStillValid(req *store.BreakglassRequest) bool {
	if req == nil || req.Status != store.StatusActive || req.Approval != store.WorkflowApprovalApproved {
		return false
	}
	if req.GrantExpiresAt == "" {
		return true
	}
	exp, err := time.Parse(time.RFC3339Nano, req.GrantExpiresAt)
	if err != nil {
		return false
	}
	return time.Now().Before(exp)
}

func findActiveBreakglassGrant(ctx context.Context, bgStore store.BreakglassStore, operator, profile, target string) (*store.BreakglassRequest, error) {
	items, err := bgStore.QueryByTargetAndStatus(ctx, target, store.StatusActive)
	if err != nil {
		return nil, err
	}
	for _, req := range items {
		if req.Operator != operator || req.Profile != profile {
			continue
		}
		if breakglassGrantStillValid(req) {
			return req, nil
		}
	}
	return nil, nil
}

func writeBreakglassGrantAlreadyActive(w http.ResponseWriter, existing *store.BreakglassRequest) {
	id := existing.RequestID
	msg := fmt.Sprintf("request active id %s; activate with: zoa breakglass activate %s", id, id)
	writeJSON(w, http.StatusConflict, map[string]interface{}{
		"kind":             "Error",
		"code":             "grant_already_active",
		"reason":           msg,
		"request_id":       id,
		"grant_expires_at": existing.GrantExpiresAt,
	})
}

func writeBreakglassDecisionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errBreakglassNotPending):
		writeError(w, http.StatusConflict, "invalid_status", "request is not pending")
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
