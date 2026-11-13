package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/store"
)

type approvalBody struct {
	Comment string `json:"comment"`
	Note    string `json:"note"`
}

func (h *Handler) handleApproveBreakglassOrExecution(w http.ResponseWriter, r *http.Request) {
	id := resolveResourceID(strings.TrimSpace(r.PathValue("id")))
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "id is required")
		return
	}
	if h.breakglassStore != nil {
		if bg, err := h.breakglassStore.Get(r.Context(), id); err == nil && bg != nil {
			h.handleApproveBreakglass(w, r, bg)
			return
		}
	}
	if h.executionStore != nil {
		if exec, err := h.executionStore.Get(r.Context(), id); err == nil && exec != nil {
			writeError(w, http.StatusNotImplemented, "not_implemented", "execution approval not yet enabled")
			return
		}
	}
	writeError(w, http.StatusNotFound, "not_found", "request not found")
}

func (h *Handler) handleRejectBreakglassOrExecution(w http.ResponseWriter, r *http.Request) {
	id := resolveResourceID(strings.TrimSpace(r.PathValue("id")))
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "id is required")
		return
	}
	if h.breakglassStore != nil {
		if bg, err := h.breakglassStore.Get(r.Context(), id); err == nil && bg != nil {
			h.handleRejectBreakglass(w, r, bg)
			return
		}
	}
	if h.executionStore != nil {
		if exec, err := h.executionStore.Get(r.Context(), id); err == nil && exec != nil {
			writeError(w, http.StatusNotImplemented, "not_implemented", "execution rejection not yet enabled")
			return
		}
	}
	writeError(w, http.StatusNotFound, "not_found", "request not found")
}

func (h *Handler) handleApproveBreakglass(w http.ResponseWriter, r *http.Request, req *store.BreakglassRequest) {
	if !h.breakglassEnabled(w) {
		return
	}
	approver, _, _ := h.resolveIdentity(r)
	note := decodeApprovalNote(r)
	if err := ApproveBreakglassRequest(r.Context(), h.breakglassStore, req, approver, note); err != nil {
		h.logger.Error("approve breakglass", "error", err)
		writeBreakglassDecisionError(w, err)
		return
	}
	updated, err := h.breakglassStore.Get(r.Context(), req.RequestID)
	if err != nil || updated == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load request after approve")
		return
	}
	writeJSON(w, http.StatusOK, updated)
	h.recordAudit(r, http.StatusOK, AuditActionBreakglassApprove, req.RequestID, withReason(note))
}

func (h *Handler) handleRejectBreakglass(w http.ResponseWriter, r *http.Request, req *store.BreakglassRequest) {
	if !h.breakglassEnabled(w) {
		return
	}
	approver, _, _ := h.resolveIdentity(r)
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
	h.recordAudit(r, http.StatusOK, AuditActionBreakglassReject, req.RequestID, withReason(note))
}

func decodeApprovalNote(r *http.Request) string {
	var body approvalBody
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Note != "" {
		return body.Note
	}
	return body.Comment
}
