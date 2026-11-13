package api

import "net/http"

func (h *AccessHandler) handleApprove(w http.ResponseWriter, r *http.Request) {
	deployment, rawID := resolveCompoundResource(r.PathValue("id"))
	if rawID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "id is required")
		return
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
		if h.tryApproveExecution(w, rawID) {
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
	updated, err := h.breakglassStore.Get(r.Context(), req.RequestID)
	if err != nil || updated == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load request after approve")
		return
	}
	writeJSON(w, http.StatusOK, updated)
	h.recordAccessAudit(r, http.StatusOK, AuditActionBreakglassApprove, req.RequestID, note, req.TargetCluster)
}

func (h *AccessHandler) handleReject(w http.ResponseWriter, r *http.Request) {
	deployment, rawID := resolveCompoundResource(r.PathValue("id"))
	if rawID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "id is required")
		return
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
	updated, err := h.breakglassStore.Get(r.Context(), req.RequestID)
	if err != nil || updated == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load request after reject")
		return
	}
	writeJSON(w, http.StatusOK, updated)
	h.recordAccessAudit(r, http.StatusOK, AuditActionBreakglassReject, req.RequestID, note, req.TargetCluster)
}

// tryApproveExecution attempts TA manual approval when enabled (ROSAENG epic); not wired on Access yet.
func (h *AccessHandler) tryApproveExecution(_ http.ResponseWriter, _ string) bool {
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
