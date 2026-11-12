package api

import (
	"net/http"
	"strings"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/store"
)

func (h *AccessHandler) handleApprove(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "id is required")
		return
	}
	rawID := id
	deployment := ""
	if dep, rid, err := ParseCompoundResourceID(id); err == nil {
		deployment = dep
		rawID = rid
	}
	if h.breakglassStore == nil {
		writeError(w, http.StatusServiceUnavailable, "breakglass_disabled", "break-glass is not enabled")
		return
	}
	req, err := h.breakglassStore.Get(r.Context(), rawID)
	if err != nil {
		h.logger.Error("access approve get breakglass", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load request")
		return
	}
	if req == nil {
		if h.tryApproveExecutionStub(w, rawID) {
			return
		}
		writeError(w, http.StatusNotFound, "not_found", "request not found")
		return
	}
	if deployment != "" && h.cfg != nil && h.cfg.DeploymentName != "" && deployment != h.cfg.DeploymentName {
		writeError(w, http.StatusBadRequest, "deployment_mismatch", "compound id deployment does not match this Access deployment")
		return
	}
	approver := accessOperator(r, h)
	note := decodeApprovalNote(r)
	if err := ApproveBreakglassRequest(r.Context(), h.breakglassStore, req, approver, note); err != nil {
		h.logger.Error("access approve breakglass", "error", err)
		writeBreakglassDecisionError(w, err)
		return
	}
	req.Status = store.StatusApproved
	req.ApprovedBy = approver
	writeJSON(w, http.StatusOK, req)
	h.recordAccessAudit(r, http.StatusOK, AuditActionBreakglassApprove, req.RequestID, note, req.TargetCluster)
}

func (h *AccessHandler) handleReject(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	rawID := id
	deployment := ""
	if dep, rid, err := ParseCompoundResourceID(id); err == nil {
		deployment = dep
		rawID = rid
	}
	if h.breakglassStore == nil {
		writeError(w, http.StatusServiceUnavailable, "breakglass_disabled", "break-glass is not enabled")
		return
	}
	req, err := h.breakglassStore.Get(r.Context(), rawID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load request")
		return
	}
	if req == nil {
		writeError(w, http.StatusNotFound, "not_found", "request not found")
		return
	}
	if deployment != "" && h.cfg != nil && h.cfg.DeploymentName != "" && deployment != h.cfg.DeploymentName {
		writeError(w, http.StatusBadRequest, "deployment_mismatch", "compound id deployment does not match this Access deployment")
		return
	}
	approver := accessOperator(r, h)
	note := decodeApprovalNote(r)
	if err := RejectBreakglassRequest(r.Context(), h.breakglassStore, req, approver, note); err != nil {
		writeBreakglassDecisionError(w, err)
		return
	}
	req.Status = store.StatusRejected
	writeJSON(w, http.StatusOK, req)
	h.recordAccessAudit(r, http.StatusOK, AuditActionBreakglassReject, req.RequestID, note, req.TargetCluster)
}

func (h *AccessHandler) tryApproveExecutionStub(w http.ResponseWriter, id string) bool {
	_ = id
	return false
}

func accessOperator(r *http.Request, h *AccessHandler) string {
	signerARN := authenticatedCallerARN(r)
	if signerARN == "" {
		return ""
	}
	if result, err := ResolveIdentity(r.Context(), signerARN, "", h.sessionStore); err == nil && result.Operator != "" {
		return result.Operator
	}
	if username, _, err := ExtractSREIdentity(signerARN); err == nil {
		return username
	}
	return signerARN
}
