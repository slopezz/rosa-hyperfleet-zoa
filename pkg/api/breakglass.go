package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/version"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/scopes"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/store"
)

func (h *Handler) breakglassEnabled(w http.ResponseWriter) bool {
	if h.cfg.BreakglassEnabled() && h.breakglassStore != nil {
		return true
	}
	writeError(w, http.StatusServiceUnavailable, "breakglass_disabled", "break-glass is not enabled for this target")
	return false
}

func (h *Handler) handleListBreakglassScopes(w http.ResponseWriter, r *http.Request) {
	if !h.breakglassEnabled(w) {
		return
	}
	items := scopes.List()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": items,
		"count": len(items),
	})
	h.recordAudit(r, http.StatusOK, AuditActionBreakglassScopes, "")
}

func (h *Handler) handleDescribeBreakglassScope(w http.ResponseWriter, r *http.Request) {
	if !h.breakglassEnabled(w) {
		return
	}
	scopeID := r.PathValue("scope")
	def, ok := scopes.Get(scopeID)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "unknown break-glass scope")
		return
	}
	writeJSON(w, http.StatusOK, def.Metadata)
}

type breakglassCreateRequest struct {
	Scope  string `json:"scope"`
	Reason string `json:"reason"`
}

func (h *Handler) handleCreateBreakglassRequest(w http.ResponseWriter, r *http.Request) {
	if !h.breakglassEnabled(w) {
		return
	}
	var body breakglassCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	body.Scope = strings.TrimSpace(body.Scope)
	body.Reason = strings.TrimSpace(body.Reason)
	if body.Scope == "" || body.Reason == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "scope and reason are required")
		return
	}
	def, ok := scopes.Get(body.Scope)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "unknown break-glass scope")
		return
	}
	operator, signerARN, sessionID := h.resolveIdentity(r)
	if operator == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "could not resolve operator identity")
		return
	}
	now := time.Now()
	requestExpires := now.Add(time.Duration(def.Metadata.Authorization.DefaultPendingHours) * time.Hour)
	grantExpires := now.Add(time.Duration(def.Metadata.Authorization.DefaultGrantHours) * time.Hour)

	status := store.StatusPendingApproval
	approvedBy := ""
	approvedAt := ""
	grantAt := ""
	if !scopes.NeedsApproval(def.Metadata.Authorization) {
		status = store.StatusApproved
		approvedBy = "auto"
		approvedAt = now.Format(time.RFC3339Nano)
		grantAt = grantExpires.Format(time.RFC3339Nano)
	}

	taskID, _ := extractTaskIDFromSigner(signerARN)
	req := &store.BreakglassRequest{
		RequestID:        uuid.NewString(),
		Scope:            body.Scope,
		Reason:           body.Reason,
		Operator:         operator,
		SignerARN:        signerARN,
		SessionID:        sessionID,
		CreatorTaskID:    taskID,
		TargetCluster:    h.cfg.TargetCluster,
		TargetType:       h.cfg.TargetType,
		AccountID:        authenticatedAccountID(r),
		Status:           status,
		Approval:         def.Metadata.Authorization.Approval,
		Revision:         version.Get().GitCommit,
		CreatedAt:        now.Format(time.RFC3339Nano),
		RequestExpiresAt: requestExpires.Format(time.RFC3339Nano),
		GrantExpiresAt:   grantAt,
		ApprovedBy:       approvedBy,
		ApprovedAt:       approvedAt,
	}
	if status == store.StatusApproved {
		req.TargetStatusKey = store.BreakglassTargetStatusKey(status, req.GrantExpiresAt)
	}

	if err := h.breakglassStore.Create(r.Context(), req); err != nil {
		h.logger.Error("create breakglass request", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to create break-glass request")
		return
	}
	writeJSON(w, http.StatusCreated, req)
	h.recordAudit(r, http.StatusCreated, AuditActionBreakglassRequest, req.RequestID, withReason(body.Reason))
}

func (h *Handler) handleGetBreakglassRequest(w http.ResponseWriter, r *http.Request) {
	if !h.breakglassEnabled(w) {
		return
	}
	id := r.PathValue("id")
	req, err := h.breakglassStore.Get(r.Context(), id)
	if err != nil {
		h.logger.Error("get breakglass request", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to get request")
		return
	}
	if req == nil {
		writeError(w, http.StatusNotFound, "not_found", "request not found")
		return
	}
	writeJSON(w, http.StatusOK, req)
}

func (h *Handler) handleListBreakglassRequests(w http.ResponseWriter, r *http.Request) {
	if !h.breakglassEnabled(w) {
		return
	}
	operator, _, _ := h.resolveIdentity(r)
	if operator == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "could not resolve operator identity")
		return
	}
	items, err := h.breakglassStore.ListByOperator(r.Context(), operator, &store.BreakglassFilter{Limit: 50})
	if err != nil {
		h.logger.Error("list breakglass requests", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to list requests")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": items, "count": len(items)})
}

func (h *Handler) handleConnectBreakglassRequest(w http.ResponseWriter, r *http.Request) {
	if !h.breakglassEnabled(w) {
		return
	}
	id := r.PathValue("id")
	req, err := h.breakglassStore.Get(r.Context(), id)
	if err != nil || req == nil {
		writeError(w, http.StatusNotFound, "not_found", "request not found")
		return
	}
	if req.Status != store.StatusApproved {
		writeError(w, http.StatusConflict, "not_approved", "request is not approved")
		return
	}
	operator, signerARN, _ := h.resolveIdentity(r)
	taskID, _ := extractTaskIDFromSigner(signerARN)
	if taskID == "" || req.CreatorTaskID == "" || taskID != req.CreatorTaskID {
		writeError(w, http.StatusForbidden, "task_mismatch", "connect is only allowed from the boundary task that created the request")
		return
	}
	if req.Operator != "" && operator != "" && req.Operator != operator {
		writeError(w, http.StatusForbidden, "operator_mismatch", "operator does not match request")
		return
	}
	if req.GrantExpiresAt != "" {
		exp, err := time.Parse(time.RFC3339Nano, req.GrantExpiresAt)
		if err == nil && time.Now().After(exp) {
			writeError(w, http.StatusGone, "grant_expired", "grant has expired")
			return
		}
	}
	resp := map[string]interface{}{
		"request_id":       req.RequestID,
		"scope":            req.Scope,
		"grant_expires_at": req.GrantExpiresAt,
		"status":           "ready",
		"kind":             "breakglass",
	}
	def, scopeOK := scopes.Get(req.Scope)
	if scopeOK && h.executor != nil {
		remaining := time.Until(mustParseRFC3339(req.GrantExpiresAt))
		switch def.Metadata.Plane {
		case scopes.PlaneKube:
			cluster, err := h.executor.BreakglassKubeConnect(r.Context(), req.RequestID, req.Scope)
			if err != nil {
				h.logger.Error("breakglass connect kube", "error", err)
				writeError(w, http.StatusInternalServerError, "provision_failed", "failed to provision break-glass kube access")
				return
			}
			resp["kube"] = cluster
			if r.URL.Query().Get("credentials") == "true" {
				if token, exp, err := h.executor.IssueBreakglassKubeToken(r.Context(), req.RequestID, req.Scope, remaining); err == nil {
					resp["kube_token_expires_at"] = exp.Format(time.RFC3339Nano)
					resp["kube_token"] = token
				}
			}
		case scopes.PlaneAWS:
			creds, err := h.executor.AssumeBreakglassRole(r.Context(), req.RequestID, req.Scope, remaining)
			if err != nil {
				h.logger.Error("breakglass connect aws", "error", err)
				writeError(w, http.StatusInternalServerError, "provision_failed", "failed to provision break-glass AWS access")
				return
			}
			awsResp := map[string]interface{}{
				"region":     creds.Region,
				"role_arn":   creds.RoleARN,
				"expiration": creds.Expiration.Format(time.RFC3339Nano),
			}
			if r.URL.Query().Get("credentials") == "true" {
				awsResp["access_key_id"] = creds.AccessKeyID
				awsResp["secret_access_key"] = creds.SecretAccessKey
				awsResp["session_token"] = creds.SessionToken
			}
			resp["aws"] = awsResp
		}
	}
	writeJSON(w, http.StatusOK, resp)
	h.recordAudit(r, http.StatusOK, AuditActionBreakglassConnect, id, withReason(req.Scope))
}

func mustParseRFC3339(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Now().Add(4 * time.Hour)
	}
	return t
}

func extractTaskIDFromSigner(signerARN string) (string, bool) {
	sessionName, _, err := ExtractSREIdentity(signerARN)
	if err != nil {
		return "", false
	}
	if !looksLikeECSTaskID(sessionName) {
		return "", false
	}
	return sessionName, true
}
