package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/version"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/breakglass"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/catalog"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/reason"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/store"
)

func (h *Handler) handleListBreakglassProfiles(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	scopeFilter := strings.TrimSpace(q.Get("scope"))
	typeFilter := strings.TrimSpace(q.Get("type"))
	items := breakglass.List()
	filtered := make([]breakglass.ProfileMetadata, 0, len(items))
	for _, m := range items {
		if !catalog.MatchCredentialScope(m.Scope, scopeFilter) {
			continue
		}
		if !catalog.MatchPrivilegeType(m.Type, typeFilter) {
			continue
		}
		filtered = append(filtered, m)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": filtered,
		"count": len(filtered),
	})
}

func (h *Handler) handleDescribeBreakglassProfile(w http.ResponseWriter, r *http.Request) {
	profileName := r.PathValue("name")
	def, ok := breakglass.Get(profileName)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "unknown break-glass profile")
		return
	}
	targetType := h.cfg.TargetType
	if targetType == "" {
		targetType = breakglass.DefaultDescribeTargetType(def.Metadata)
	}
	writeJSON(w, http.StatusOK, breakglass.DescribeProfile(def, targetType))
}

type breakglassCreateRequest struct {
	Profile string `json:"profile"`
	Reason  string `json:"reason"`
}

func (h *Handler) handleCreateBreakglassRequest(w http.ResponseWriter, r *http.Request) {
	var body breakglassCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	body.Profile = strings.TrimSpace(body.Profile)
	if body.Profile == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "profile is required")
		return
	}
	reasonValue, err := reason.Require(body.Reason)
	if err != nil {
		code := "missing_reason"
		if strings.TrimSpace(body.Reason) != "" {
			code = "invalid_reason"
		}
		writeError(w, http.StatusBadRequest, code, err.Error())
		return
	}
	def, ok := breakglass.Get(body.Profile)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "unknown break-glass profile")
		return
	}
	operator, signerARN, sessionID := h.resolveIdentity(r)
	if operator == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "could not resolve operator identity")
		return
	}
	if existing, err := findActiveBreakglassGrant(r.Context(), h.breakglassStore, operator, body.Profile, h.cfg.TargetCluster); err != nil {
		h.logger.Error("find active breakglass grant", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to create break-glass request")
		return
	} else if existing != nil {
		writeBreakglassGrantAlreadyActive(w, existing)
		return
	}
	now := time.Now()
	requestExpires := now.Add(time.Duration(def.Metadata.Authorization.DefaultRequestHours) * time.Hour)

	taskID, _ := extractTaskIDFromSigner(signerARN)
	req := &store.BreakglassRequest{
		RequestID:        uuid.NewString(),
		Profile:          body.Profile,
		Reason:           reasonValue,
		Operator:         operator,
		ReqSignerARN:     signerARN,
		ReqSessionID:     sessionID,
		ReqTaskID:        taskID,
		TargetCluster:    h.cfg.TargetCluster,
		TargetType:       h.cfg.TargetType,
		AccountID:        authenticatedAccountID(r),
		Status:           store.StatusPending,
		Approval:         store.WorkflowApprovalPending,
		Revision:         version.Get().GitCommit,
		CreatedAt:        now.Format(time.RFC3339Nano),
		RequestExpiresAt: requestExpires.Format(time.RFC3339Nano),
	}

	if err := h.breakglassStore.Create(r.Context(), req); err != nil {
		h.logger.Error("create breakglass request", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to create break-glass request")
		return
	}
	h.recordAudit(r, http.StatusCreated, AuditActionBreakglassRequest, req.RequestID,
		withReason(reasonValue), withAuditPath(breakglassAuditPath(body.Profile, "requests")))

	resp, err := ApproveBreakglassOnCreateWhenNoPeerWorkflow(r.Context(), h.breakglassStore, req, def)
	if err != nil {
		h.logger.Error("approve breakglass on create", "error", err, "request_id", req.RequestID)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to approve break-glass request")
		return
	}
	if resp != nil && resp.Status == store.StatusActive {
		h.recordAudit(r, http.StatusOK, AuditActionBreakglassApprove, resp.RequestID,
			withReason(reasonValue), withAuditPath(breakglassAuditPath(resp.Profile, "approve")))
		req = resp
	}

	writeJSON(w, http.StatusCreated, req)
}

func (h *Handler) handleGetBreakglassRequest(w http.ResponseWriter, r *http.Request) {
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
	q := r.URL.Query()
	filter := &store.BreakglassFilter{
		Limit: 50,
	}
	// Same as GET /trusted-actions/runs: optional target filter; default lists all
	// targets in the deployment table (RC + MC rows share BREAKGLASS_TABLE).
	if v := q.Get("target"); v != "" {
		filter.Target = &v
	}
	if v := q.Get("profile"); v != "" {
		filter.Profile = &v
	}
	if v := q.Get("status"); v != "" && v != "all" {
		s := store.Status(v)
		filter.Status = &s
	}
	if v := q.Get("approval"); v != "" && v != "all" {
		filter.Approval = &v
	}
	if v := q.Get("decided_by"); v != "" {
		filter.DecidedBy = &v
	}
	if v := q.Get("since"); v != "" {
		if t, err := parseTimeValue(v); err == nil {
			filter.Since = &t
		}
	}
	if v := q.Get("decided_since"); v != "" {
		if t, err := parseTimeValue(v); err == nil {
			filter.DecidedSince = &t
		}
	}
	if v := q.Get("until"); v != "" {
		if t, err := parseUntilTimeValue(v); err == nil {
			filter.Before = &t
		}
	}
	if v := q.Get("limit"); v != "" {
		if n, err := fmt.Sscanf(v, "%d", &filter.Limit); n == 1 && err == nil && filter.Limit > 200 {
			filter.Limit = 200
		}
	}

	mine := q.Get("mine") == "true"
	fleetView := !mine
	if fleetView {
		if v := q.Get("operator"); v != "" {
			filter.Operator = &v
		}
	} else {
		operator, _, _ := h.resolveIdentity(r)
		if operator == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized", "could not resolve operator identity")
			return
		}
		if q.Get("operator") != "" {
			writeError(w, http.StatusBadRequest, "invalid_query", "operator filter is not allowed with mine=true")
			return
		}
		filter.Operator = &operator
	}

	items, err := h.breakglassStore.ListAll(r.Context(), filter)
	if err != nil {
		h.logger.Error("list breakglass requests", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to list requests")
		return
	}
	if fleetView {
		h.recordAudit(r, http.StatusOK, AuditActionBreakglassHistory, "", withReason(q.Get("operator")))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": items, "count": len(items)})
}

func (h *Handler) handleActivateBreakglassRequest(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	req, err := h.breakglassStore.Get(r.Context(), id)
	if err != nil || req == nil {
		writeError(w, http.StatusNotFound, "not_found", "request not found")
		return
	}
	if req.Status != store.StatusActive {
		writeError(w, http.StatusConflict, "grant_not_active", "request grant is not active")
		return
	}
	if req.Approval != store.WorkflowApprovalApproved {
		writeError(w, http.StatusConflict, "not_approved", "request approval is not approved")
		return
	}
	operator, signerARN, sessionID := h.resolveIdentity(r)
	taskID, _ := extractTaskIDFromSigner(signerARN)
	if req.Operator != "" && operator != "" && req.Operator != operator {
		writeError(w, http.StatusForbidden, "operator_mismatch", "operator does not match request")
		return
	}
	if req.TargetCluster != "" && h.cfg.TargetCluster != "" && req.TargetCluster != h.cfg.TargetCluster {
		writeError(w, http.StatusForbidden, "target_mismatch",
			fmt.Sprintf("grant is for target %q; activate from a boundary session for that target (this deployment is %q)",
				req.TargetCluster, h.cfg.TargetCluster))
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
		"id":               req.RequestID,
		"request_id":       req.RequestID,
		"profile":          req.Profile,
		"target_cluster":   req.TargetCluster,
		"grant_expires_at": req.GrantExpiresAt,
		"status":           "ready",
		"kind":             "breakglass",
	}
	def, scopeOK := breakglass.Get(req.Profile)
	if scopeOK && h.executor != nil {
		remaining := time.Until(mustParseRFC3339(req.GrantExpiresAt))
		if _, err := breakglass.CredentialDurationForGrant(def.Metadata.Scope, remaining); err != nil {
			if writeBreakglassGrantTooShort(w, err) {
				return
			}
		}
		switch def.Metadata.Scope {
		case breakglass.ScopeKube:
			cluster, err := h.executor.BreakglassKubeActivate(r.Context(), req.RequestID, req.Profile)
			if err != nil {
				h.logger.Error("breakglass activate kube", "error", err)
				writeError(w, http.StatusInternalServerError, "provision_failed", "failed to provision break-glass kube access")
				return
			}
			resp["kube"] = cluster
			token, exp, err := h.executor.IssueBreakglassKubeToken(r.Context(), req.RequestID, req.Profile, remaining)
			if err != nil {
				if writeBreakglassGrantTooShort(w, err) {
					return
				}
				h.logger.Error("breakglass issue kube token", "error", err, "request_id", req.RequestID)
				writeError(w, http.StatusInternalServerError, "provision_failed", "failed to issue break-glass kube token")
				return
			}
			resp["kube_token_expires_at"] = exp.Format(time.RFC3339Nano)
			resp["kube_token"] = token
		case breakglass.ScopeAWS:
			creds, err := h.executor.AssumeBreakglassRole(r.Context(), req.RequestID, req.Profile, remaining)
			if err != nil {
				if writeBreakglassGrantTooShort(w, err) {
					return
				}
				h.logger.Error("breakglass activate aws", "error", err)
				writeError(w, http.StatusInternalServerError, "provision_failed", "failed to provision break-glass AWS access")
				return
			}
			resp["aws"] = map[string]interface{}{
				"region":            creds.Region,
				"role_arn":          creds.RoleARN,
				"expiration":        creds.Expiration.Format(time.RFC3339Nano),
				"access_key_id":     creds.AccessKeyID,
				"secret_access_key": creds.SecretAccessKey,
				"session_token":     creds.SessionToken,
			}
		}
	}
	ev := store.ActivateEvent{
		At:        time.Now().Format(time.RFC3339Nano),
		TaskID:    taskID,
		SessionID: sessionID,
	}
	if err := h.breakglassStore.AppendActivateEvent(r.Context(), id, ev); err != nil {
		h.logger.Error("breakglass record activate", "error", err, "request_id", id)
	}
	writeJSON(w, http.StatusOK, resp)
	h.recordAudit(r, http.StatusOK, AuditActionBreakglassActivate, id,
		withReason(req.Reason), withAuditPath(breakglassAuditPath(req.Profile, "activate")))
}

func mustParseRFC3339(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Now().Add(4 * time.Hour)
	}
	return t
}

func writeBreakglassGrantTooShort(w http.ResponseWriter, err error) bool {
	var short *breakglass.GrantTooShortError
	if errors.As(err, &short) {
		writeError(w, http.StatusConflict, "grant_too_short", breakglass.ActivateGrantTooShortMessage(short.Scope, short.Remaining))
		return true
	}
	return false
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
