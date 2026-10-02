package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/version"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/config"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/execcreds"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/store"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/targetroles"
)

// ECSAPI abstracts ECS operations for testability.
type ECSAPI interface {
	RunTask(ctx context.Context, input *RunTaskInput) (*RunTaskOutput, error)
	StopTask(ctx context.Context, input *StopTaskInput) error
}

// ECSClientFactory resolves an ECS client for a boundary target (RC local or MC cross-account).
type ECSClientFactory func(ctx context.Context, target *store.Target) (ECSAPI, error)

// RunTaskInput holds parameters for running an ECS task.
type RunTaskInput struct {
	Cluster        string
	TaskDefinition string
	Subnets        []string
	SecurityGroup  string
	Environment    map[string]string
	Tags           map[string]string
}

// RunTaskOutput holds the result of running an ECS task.
type RunTaskOutput struct {
	TaskArn string
	TaskID  string
}

// StopTaskInput holds parameters for stopping an ECS task.
type StopTaskInput struct {
	Cluster string
	TaskArn string
	Reason  string
}

// ExecCredentialVendor vends ECS Exec credentials scoped to one boundary task.
type ExecCredentialVendor interface {
	VendForTask(ctx context.Context, execRoleARN, username, clusterARN, taskARN string) (*execcreds.APICredentials, error)
}

// AccessHandler handles HTTP requests for the access Lambda mode.
type AccessHandler struct {
	cfg          *config.Config
	sessionStore store.SessionStore
	targetStore  store.TargetStore
	auditStore   store.AuditStore
	ecsClient    ECSAPI
	ecsFactory   ECSClientFactory
	execVendor   ExecCredentialVendor
	logger       *slog.Logger
	mux          *http.ServeMux
}

// AccessDeps holds the dependencies for creating an AccessHandler.
type AccessDeps struct {
	Cfg          *config.Config
	SessionStore store.SessionStore
	TargetStore  store.TargetStore
	AuditStore   store.AuditStore
	ECSClient    ECSAPI
	ECSFactory   ECSClientFactory
	ExecVendor   ExecCredentialVendor
	Logger       *slog.Logger
}

// NewAccessHandler creates a new AccessHandler and registers its routes.
func NewAccessHandler(deps AccessDeps) *AccessHandler {
	h := &AccessHandler{
		cfg:          deps.Cfg,
		sessionStore: deps.SessionStore,
		targetStore:  deps.TargetStore,
		auditStore:   deps.AuditStore,
		ecsClient:    deps.ECSClient,
		ecsFactory:   deps.ECSFactory,
		execVendor:   deps.ExecVendor,
		logger:       deps.Logger,
		mux:          http.NewServeMux(),
	}
	h.registerRoutes()
	return h
}

func (h *AccessHandler) registerRoutes() {
	h.mux.HandleFunc("GET "+PathHealth, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	h.mux.HandleFunc("GET "+PathVersion, func(w http.ResponseWriter, _ *http.Request) {
		info := version.Get()
		writeJSON(w, http.StatusOK, struct {
			version.Info
			Mode string `json:"mode"`
		}{Info: info, Mode: "access"})
	})

	h.mux.HandleFunc("POST "+PathSessionsStart, h.handleSessionStart)
	h.mux.HandleFunc("GET "+PathSessions, h.handleSessionList)
	h.mux.HandleFunc("POST "+PathSessionsStop+"{id}", h.handleSessionStop)
	h.mux.HandleFunc("POST "+PathSessionsJoin+"{id}", h.handleSessionJoin)

	h.mux.HandleFunc("GET "+PathTargets, h.handleTargetList)

	h.mux.HandleFunc("POST "+PathApprove+"{id}", h.handleApproveStub)
	h.mux.HandleFunc("POST "+PathReject+"{id}", h.handleRejectStub)
}

func (h *AccessHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

type sessionStartRequest struct {
	DeploymentName string `json:"deployment_name"`
	Target         string `json:"target"`
	TimeoutHours   int    `json:"timeout_hours,omitempty"`
}

type sessionStartResponse struct {
	SessionID string              `json:"session_id"`
	Status    store.SessionStatus `json:"status"`
	Target    string              `json:"target"`
	TaskArn   string              `json:"task_arn,omitempty"`
	Deadline  string              `json:"deadline"`
	Region    string              `json:"region,omitempty"`
}

func (h *AccessHandler) handleSessionStart(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	operatorARN := r.Header.Get("X-Operator")

	username, _, err := ExtractSREIdentity(operatorARN)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_identity", fmt.Sprintf("cannot extract SRE identity: %v", err))
		return
	}

	var req sessionStartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}

	if req.Target == "" {
		writeError(w, http.StatusBadRequest, "missing_target", "target is required")
		return
	}

	// Look up target from DynamoDB for VPC/subnet/task definition info
	target, err := h.targetStore.Get(ctx, req.Target)
	if err != nil {
		h.logger.Error("failed to get target", "target", req.Target, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to look up target")
		return
	}
	if target == nil {
		writeError(w, http.StatusNotFound, "target_not_found", fmt.Sprintf("target %q not found", req.Target))
		return
	}

	timeoutHours := 4
	if req.TimeoutHours > 0 && req.TimeoutHours <= 8 {
		timeoutHours = req.TimeoutHours
	}

	now := time.Now()
	deadline := now.Add(time.Duration(timeoutHours) * time.Hour)
	sessionID := uuid.New().String()

	session := &store.Session{
		SessionID:      sessionID,
		Operator:       username,
		OperatorARN:    operatorARN,
		TargetCluster:  req.Target,
		Region:         target.Region,
		Status:         store.SessionStatusCreating,
		CreatedAt:      now.Format(time.RFC3339Nano),
		Deadline:       deadline.Format(time.RFC3339Nano),
		VpcId:          target.VpcId,
		DeploymentName: req.DeploymentName,
	}

	if err := h.sessionStore.Put(ctx, session); err != nil {
		h.logger.Error("failed to create session", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to create session")
		return
	}

	h.recordAccessAudit(r, http.StatusOK, "session_start", sessionID)

	writeJSON(w, http.StatusOK, sessionStartResponse{
		SessionID: sessionID,
		Status:    session.Status,
		Target:    req.Target,
		Deadline:  session.Deadline,
		Region:    session.Region,
	})
}

func (h *AccessHandler) ensureBoundaryTask(ctx context.Context, session *store.Session) error {
	if session.TaskArn != "" {
		return nil
	}
	if h.ecsClient == nil {
		return fmt.Errorf("ecs client not configured")
	}

	target, err := h.targetStore.Get(ctx, session.TargetCluster)
	if err != nil {
		return fmt.Errorf("looking up target %q: %w", session.TargetCluster, err)
	}
	if target == nil {
		return fmt.Errorf("target %q not found", session.TargetCluster)
	}

	ecs, err := h.ecsForTarget(ctx, target)
	if err != nil {
		return err
	}

	subnets := splitCSV(target.SubnetIds)
	taskOutput, err := ecs.RunTask(ctx, &RunTaskInput{
		Cluster:        target.EcsClusterArn,
		TaskDefinition: target.TaskDefinitionArn,
		Subnets:        subnets,
		SecurityGroup:  target.SecurityGroupId,
		Environment: map[string]string{
			"ZOA_API_URL":    target.FunctionUrl,
			"ZOA_TARGET":     session.TargetCluster,
			"ZOA_DEPLOYMENT": session.DeploymentName,
		},
		Tags: map[string]string{
			"Component":  "zoa",
			"sre":        session.Operator,
			"sessionId":  session.SessionID,
			"deployment": session.DeploymentName,
			"target":     session.TargetCluster,
		},
	})
	if err != nil {
		_ = h.sessionStore.UpdateStatus(ctx, session.SessionID, session.Status, store.SessionStatusFailed,
			map[string]interface{}{"stopReason": store.StopReasonProvisionFailed})
		return fmt.Errorf("starting boundary task: %w", err)
	}

	from := session.Status
	if from == "" {
		from = store.SessionStatusCreating
	}
	if err := h.sessionStore.UpdateStatus(ctx, session.SessionID, from, store.SessionStatusActive,
		map[string]interface{}{
			"taskArn":    taskOutput.TaskArn,
			"taskId":     taskOutput.TaskID,
			"ecsCluster": target.EcsClusterArn,
		}); err != nil {
		return fmt.Errorf("updating session after RunTask: %w", err)
	}

	session.Status = store.SessionStatusActive
	session.TaskArn = taskOutput.TaskArn
	session.TaskID = taskOutput.TaskID
	session.EcsCluster = target.EcsClusterArn
	return nil
}

func (h *AccessHandler) handleSessionList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	operatorARN := r.Header.Get("X-Operator")

	filter := &store.SessionFilter{}
	if v := q.Get("status"); v != "" && v != "all" {
		s := store.SessionStatus(v)
		filter.Status = &s
	}
	if v := q.Get("target"); v != "" {
		filter.Target = &v
	}

	// scope=mine: `zoa session list` — caller's sessions only; no cross-operator listing.
	if q.Get("scope") == "mine" {
		if q.Get("operator") != "" {
			writeError(w, http.StatusBadRequest, "invalid_query", "operator filter is not allowed with scope=mine")
			return
		}
		username, _, err := ExtractSREIdentity(operatorARN)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_identity", "cannot extract SRE identity")
			return
		}
		if username == "" {
			writeError(w, http.StatusBadRequest, "invalid_identity", "cannot extract SRE identity")
			return
		}
		filter.Operator = &username
	} else if v := q.Get("operator"); v != "" {
		filter.Operator = &v
	}

	// Time filters — same as `zoa runs` (since/until with flexible formats).
	if v := q.Get("since"); v != "" {
		if t, err := parseTimeValue(v); err == nil {
			filter.Since = &t
		}
	}
	if v := q.Get("until"); v != "" {
		if t, err := parseUntilTimeValue(v); err == nil {
			filter.Before = &t
		}
	}

	limit := 50
	if v := q.Get("limit"); v != "" {
		if n, err := fmt.Sscanf(v, "%d", &limit); n == 1 && err == nil {
			if limit > 200 {
				limit = 200
			}
		}
	}
	filter.Limit = limit

	// Use ListAll (date-bucket-index) — same as `zoa runs`. CLI defaults to
	// --since 24h to keep queries bounded.
	sessions, err := h.sessionStore.ListAll(ctx, filter)
	if err != nil {
		h.logger.Error("failed to list sessions", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to list sessions")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"items": sessions, "count": len(sessions)})
}

func (h *AccessHandler) handleSessionStop(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := r.PathValue("id")
	operatorARN := r.Header.Get("X-Operator")

	username, _, err := ExtractSREIdentity(operatorARN)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_identity", "cannot extract SRE identity")
		return
	}

	session, err := h.sessionStore.Get(ctx, sessionID)
	if err != nil {
		h.logger.Error("failed to get session", "session_id", sessionID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to get session")
		return
	}
	if session == nil {
		writeError(w, http.StatusNotFound, "not_found", "session not found")
		return
	}

	if session.Operator != username {
		writeError(w, http.StatusForbidden, "forbidden", "only the session owner can stop it")
		return
	}

	switch session.Status {
	case store.SessionStatusActive, store.SessionStatusCreating:
	default:
		writeError(w, http.StatusConflict, "invalid_status", fmt.Sprintf("session is %s, not stoppable", session.Status))
		return
	}

	if session.TaskArn != "" {
		target, tErr := h.targetForSession(ctx, session)
		if tErr != nil {
			h.logger.Error("cannot stop session: target lookup failed", "session_id", sessionID, "error", tErr)
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to resolve session target")
			return
		}
		ecs, eErr := h.ecsForTarget(ctx, target)
		if eErr != nil {
			h.logger.Error("cannot stop session: ECS client", "session_id", sessionID, "error", eErr)
			writeError(w, http.StatusServiceUnavailable, "task_stop_failed", "cannot stop boundary task")
			return
		}
		if err := ecs.StopTask(ctx, &StopTaskInput{
			Cluster: session.EcsCluster,
			TaskArn: session.TaskArn,
			Reason:  store.StopReasonOperatorStop,
		}); err != nil {
			h.logger.Error("failed to stop ECS task", "session_id", sessionID, "task_arn", session.TaskArn, "error", err)
			writeError(w, http.StatusBadGateway, "task_stop_failed", "failed to stop boundary ECS task; session is still active")
			return
		}
	}

	err = h.sessionStore.UpdateStatus(ctx, sessionID, session.Status, store.SessionStatusTerminated,
		map[string]interface{}{"stopReason": store.StopReasonOperatorStop})
	if err != nil {
		h.logger.Error("failed to update session status", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to update session")
		return
	}

	h.recordAccessAudit(r, http.StatusOK, "session_stop", sessionID)

	writeJSON(w, http.StatusOK, map[string]string{"status": "terminated", "session_id": sessionID})
}

type sessionJoinResponse struct {
	SessionID       string                    `json:"session_id"`
	Cluster         string                    `json:"cluster"`
	TaskArn         string                    `json:"task_arn"`
	Region          string                    `json:"region"`
	ContainerName   string                    `json:"container_name"`
	ExecCommand     string                    `json:"exec_command"`
	ExecCredentials *execcreds.APICredentials `json:"exec_credentials"`
}

func (h *AccessHandler) handleSessionJoin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := r.PathValue("id")
	operatorARN := r.Header.Get("X-Operator")

	username, _, err := ExtractSREIdentity(operatorARN)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_identity", "cannot extract SRE identity")
		return
	}

	session, err := h.sessionStore.Get(ctx, sessionID)
	if err != nil {
		h.logger.Error("failed to get session", "session_id", sessionID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to get session")
		return
	}
	if session == nil {
		writeError(w, http.StatusNotFound, "not_found", "session not found")
		return
	}

	if session.Operator != username {
		writeError(w, http.StatusForbidden, "forbidden", "only the session owner can join")
		return
	}

	switch session.Status {
	case store.SessionStatusActive, store.SessionStatusCreating:
	default:
		writeError(w, http.StatusConflict, "invalid_status", fmt.Sprintf("session is %s, not joinable", session.Status))
		return
	}

	if err := h.ensureBoundaryTask(ctx, session); err != nil {
		h.logger.Error("failed to ensure boundary task", "session_id", sessionID, "error", err)
		writeError(w, http.StatusInternalServerError, "ecs_error", "failed to start boundary container")
		return
	}

	if session.Status != store.SessionStatusActive {
		writeError(w, http.StatusConflict, "invalid_status", fmt.Sprintf("session is %s, not active", session.Status))
		return
	}

	if h.execVendor == nil {
		h.logger.Error("exec credential vendor not configured")
		writeError(w, http.StatusInternalServerError, "internal_error", "exec credentials not configured")
		return
	}

	target, tErr := h.targetForSession(ctx, session)
	if tErr != nil {
		h.logger.Error("failed to resolve target for exec", "session_id", sessionID, "error", tErr)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to resolve session target")
		return
	}
	execRoleARN := h.execScopedRoleARN(target)
	if execRoleARN == "" {
		h.logger.Error("exec scoped role ARN not configured")
		writeError(w, http.StatusInternalServerError, "internal_error", "exec credentials not configured")
		return
	}

	execCreds, err := h.execVendor.VendForTask(ctx, execRoleARN, username, session.EcsCluster, session.TaskArn)
	if err != nil {
		h.logger.Error("failed to vend exec credentials", "session_id", sessionID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to vend exec credentials")
		return
	}

	h.recordAccessAudit(r, http.StatusOK, "session_join", sessionID)

	writeJSON(w, http.StatusOK, sessionJoinResponse{
		SessionID:       sessionID,
		Cluster:         session.EcsCluster,
		TaskArn:         session.TaskArn,
		Region:          session.Region,
		ContainerName:   "zoa-boundary",
		ExecCommand:     h.cfg.BoundaryECSExecCommand,
		ExecCredentials: execCreds,
	})
}

func (h *AccessHandler) handleTargetList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	targets, err := h.targetStore.List(ctx)
	if err != nil {
		h.logger.Error("failed to list targets", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to list targets")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"items": targets, "count": len(targets)})
}

func (h *AccessHandler) handleApproveStub(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h.recordAccessAudit(r, http.StatusNotImplemented, "approve", id)
	writeError(w, http.StatusNotImplemented, "not_implemented", "approval workflow not yet enabled")
}

func (h *AccessHandler) handleRejectStub(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h.recordAccessAudit(r, http.StatusNotImplemented, "reject", id)
	writeError(w, http.StatusNotImplemented, "not_implemented", "approval workflow not yet enabled")
}

func (h *AccessHandler) recordAccessAudit(r *http.Request, statusCode int, action, sessionID string) {
	if h.auditStore == nil {
		return
	}
	operator := r.Header.Get("X-Operator")
	accountID := r.Header.Get("X-Account-ID")
	entry := &store.AuditEntry{
		AccountID:   accountID,
		Timestamp:   time.Now().Format(time.RFC3339Nano),
		Method:      r.Method,
		Path:        r.URL.Path,
		StatusCode:  statusCode,
		Operator:    operator,
		Action:      action,
		SourceIP:    r.Header.Get("X-Source-IP"),
		RequestID:   r.Header.Get("X-Request-ID"),
		UserAgent:   r.Header.Get("User-Agent"),
		ExecutionID: sessionID,
	}
	if err := h.auditStore.Record(r.Context(), entry); err != nil {
		h.logger.Error("failed to record audit entry", "error", err)
	}
}

func (h *AccessHandler) ecsForTarget(ctx context.Context, target *store.Target) (ECSAPI, error) {
	if h.ecsFactory != nil {
		return h.ecsFactory(ctx, target)
	}
	if h.ecsClient == nil {
		return nil, fmt.Errorf("ecs client not configured")
	}
	return h.ecsClient, nil
}

func (h *AccessHandler) execScopedRoleARN(target *store.Target) string {
	if target != nil && target.AccountId != "" && target.TargetID != "" {
		return targetroles.ExecScopedRoleARN(target.AccountId, target.TargetID)
	}
	if h.cfg != nil && h.cfg.ExecScopedRoleARN != "" {
		return h.cfg.ExecScopedRoleARN
	}
	return ""
}

func (h *AccessHandler) targetForSession(ctx context.Context, session *store.Session) (*store.Target, error) {
	if session == nil || session.TargetCluster == "" {
		return nil, fmt.Errorf("session has no target")
	}
	target, err := h.targetStore.Get(ctx, session.TargetCluster)
	if err != nil {
		return nil, fmt.Errorf("looking up target %q: %w", session.TargetCluster, err)
	}
	if target == nil {
		return nil, fmt.Errorf("target %q not found", session.TargetCluster)
	}
	return target, nil
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	for _, part := range splitString(s, ',') {
		trimmed := trimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func splitString(s string, sep byte) []string {
	var result []string
	start := 0
	for i := range len(s) {
		if s[i] == sep {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
