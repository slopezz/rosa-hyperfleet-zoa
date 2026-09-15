package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/google/uuid"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/version"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/actions"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/executor"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/metrics"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/store"
)

var jiraPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]+-\d+$`)

type createRequest struct {
	Jira           string            `json:"jira"`
	Params         map[string]string `json:"params,omitempty"`
	Force          bool              `json:"force"`
	DryRun         bool              `json:"dry_run"`
	ExecutionMode  string            `json:"execution_mode,omitempty"`  // "sync" or "async" — override TA default
	TimeoutSeconds int               `json:"timeout_seconds,omitempty"` // override TA's default timeout (bounded by server max)
}

type createResponse struct {
	ID              string          `json:"id"`
	Action          string          `json:"action"`
	RequestedAction string          `json:"requested_action,omitempty"`
	TargetCluster   string          `json:"target_cluster"`
	Operator        string          `json:"operator"`
	Status          string          `json:"status"`
	ExecutionMode   string          `json:"execution_mode"`
	Scope           string          `json:"scope"`
	Type            string          `json:"type"`
	DryRun          bool            `json:"dry_run"`
	Force           bool            `json:"force"`
	DurationMs      *int64          `json:"duration_ms,omitempty"`
	Output          json.RawMessage `json:"output,omitempty"`
	Logs            string          `json:"logs,omitempty"`
}

func (h *Handler) handleCreate(w http.ResponseWriter, r *http.Request, actionName string) {
	ctx := r.Context()
	// TODO: Replace with identity resolution from ECS task ARN once rosa-boundary is integrated
	operator := r.Header.Get("X-Operator")
	accountID := r.Header.Get("X-Account-ID")

	if accountID == "" {
		writeError(w, http.StatusBadRequest, "missing_account", "X-Account-ID header required")
		return
	}

	action, ok := actions.Get(actionName)
	if !ok {
		h.recordAudit(r, http.StatusNotFound, actionName, "")
		metrics.EmitRejection(h.cfg.TargetCluster, metrics.RejectionActionNotFound)
		writeError(w, http.StatusNotFound, "action_not_found", fmt.Sprintf("action %q not found", actionName))
		return
	}

	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.recordAudit(r, http.StatusBadRequest, actionName, "")
		metrics.EmitRejection(h.cfg.TargetCluster, metrics.RejectionValidationFailed)
		writeError(w, http.StatusBadRequest, "invalid_body", "failed to parse request body")
		return
	}

	if req.Jira == "" {
		h.recordAudit(r, http.StatusBadRequest, actionName, "", withForce(req.Force), withDryRun(req.DryRun))
		metrics.EmitRejection(h.cfg.TargetCluster, metrics.RejectionValidationFailed)
		writeError(w, http.StatusBadRequest, "missing_jira", "jira ticket is required for all executions")
		return
	}
	if !jiraPattern.MatchString(req.Jira) {
		h.recordAudit(r, http.StatusBadRequest, actionName, "", withJira(req.Jira), withForce(req.Force), withDryRun(req.DryRun))
		metrics.EmitRejection(h.cfg.TargetCluster, metrics.RejectionValidationFailed)
		writeError(w, http.StatusBadRequest, "invalid_jira", fmt.Sprintf("jira ticket %q must match format PROJECT-123", req.Jira))
		return
	}

	meta := action.Metadata()

	if err := validateParams(meta, req.Params); err != nil {
		h.recordAudit(r, http.StatusBadRequest, actionName, "", withJira(req.Jira), withForce(req.Force), withDryRun(req.DryRun))
		metrics.EmitRejection(h.cfg.TargetCluster, metrics.RejectionValidationFailed)
		writeError(w, http.StatusBadRequest, "invalid_params", err.Error())
		return
	}

	executedAction := actionName
	if req.DryRun && meta.DryRunAction != "" {
		for k, v := range meta.DryRunExtraParams {
			if _, exists := req.Params[k]; !exists {
				if req.Params == nil {
					req.Params = make(map[string]string)
				}
				req.Params[k] = v
			}
		}
		executedAction = meta.DryRunAction
		dryAction, dryOk := actions.Get(executedAction)
		if !dryOk {
			writeError(w, http.StatusInternalServerError, "dry_run_misconfigured", fmt.Sprintf("dry-run action %q not found", executedAction))
			return
		}
		action = dryAction
		meta = action.Metadata()
	}

	if err := h.executor.ValidateAction(ctx, action, req.Params); err != nil {
		h.recordAudit(r, http.StatusBadRequest, actionName, "", withJira(req.Jira), withForce(req.Force), withDryRun(req.DryRun))
		metrics.EmitRejection(h.cfg.TargetCluster, metrics.RejectionValidationFailed)
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}

	// Write cooldown check (prevents duplicate SRE requests at UX level)
	// Cooldown is per (target, action, params) — different params means different target workload.
	// - Dry-run executions don't trigger or count towards cooldown (no real mutation)
	// - Force bypasses cooldown but doesn't affect what triggers cooldown
	if meta.Type == "write" && !req.Force && !req.DryRun {
		cooldown := h.cfg.WriteCooldownSeconds
		if meta.WriteCooldownSeconds > 0 {
			cooldown = meta.WriteCooldownSeconds
		}
		since := time.Now().Add(-time.Duration(cooldown) * time.Second)
		recent, err := h.executionStore.ListByTargetAndAction(ctx, h.cfg.TargetCluster, actionName, since)
		if err != nil {
			h.logger.Error("failed to check write cooldown", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to check write cooldown")
			return
		}
		// Filter to executions with matching params (same target workload)
		// Exclude dry-runs (they don't count as real executions)
		matching := filterMatchingParams(recent, req.Params)
		if len(matching) > 0 {
			h.recordAudit(r, http.StatusTooManyRequests, actionName, "", withJira(req.Jira), withForce(req.Force), withDryRun(req.DryRun))
			metrics.EmitRejection(h.cfg.TargetCluster, metrics.RejectionWriteCooldown)
			writeError(w, http.StatusTooManyRequests, "write_cooldown",
				fmt.Sprintf("action %q with these params was executed within the last %ds; use force=true to override", actionName, cooldown))
			return
		}
	}

	// Max concurrent check (prevents overloading a single cluster)
	if !req.Force {
		activeCount, err := h.executionStore.CountActiveByTarget(ctx, h.cfg.TargetCluster)
		if err != nil {
			h.logger.Error("failed to count active executions", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to check concurrent limit")
			return
		}
		if activeCount >= h.cfg.MaxConcurrentPerTarget {
			h.recordAudit(r, http.StatusTooManyRequests, actionName, "", withJira(req.Jira), withForce(req.Force), withDryRun(req.DryRun))
			metrics.EmitRejection(h.cfg.TargetCluster, metrics.RejectionMaxConcurrent)
			writeError(w, http.StatusTooManyRequests, "max_concurrent",
				fmt.Sprintf("target %q has %d active executions (max %d); use force=true to override", h.cfg.TargetCluster, activeCount, h.cfg.MaxConcurrentPerTarget))
			return
		}
	}

	// Determine execution mode: CLI override > TA default
	execMode := meta.ExecutionMode
	if req.ExecutionMode != "" {
		if req.ExecutionMode != "sync" && req.ExecutionMode != "async" {
			writeError(w, http.StatusBadRequest, "invalid_execution_mode",
				fmt.Sprintf("execution_mode must be 'sync' or 'async', got %q", req.ExecutionMode))
			return
		}
		if req.ExecutionMode != meta.ExecutionMode {
			if meta.DisallowExecutionModeOverride {
				h.recordAudit(r, http.StatusBadRequest, actionName, "", withJira(req.Jira), withForce(req.Force), withDryRun(req.DryRun))
				writeError(w, http.StatusBadRequest, "execution_mode_locked",
					fmt.Sprintf("action %q requires execution_mode=%q and cannot be overridden", actionName, meta.ExecutionMode))
				return
			}
			h.logger.Info("execution mode overridden",
				"action", actionName,
				"default", meta.ExecutionMode,
				"requested", req.ExecutionMode,
				"operator", operator,
			)
		}
		execMode = req.ExecutionMode
	}
	if execMode == "" {
		execMode = "sync"
	}

	// Resolve timeout: CLI override > TA default, bounded by server max
	timeoutSeconds := meta.TimeoutSeconds
	if req.TimeoutSeconds > 0 {
		if req.TimeoutSeconds > h.cfg.ExecutionDeadlineSeconds {
			writeError(w, http.StatusBadRequest, "timeout_exceeded",
				fmt.Sprintf("requested timeout %ds exceeds server maximum %ds", req.TimeoutSeconds, h.cfg.ExecutionDeadlineSeconds))
			return
		}
		timeoutSeconds = req.TimeoutSeconds
	}

	executionID := uuid.New().String()
	now := time.Now().Format(time.RFC3339Nano)

	var requestedAction string
	if executedAction != actionName {
		requestedAction = actionName
	}

	exec := &store.Execution{
		ID:              executionID,
		Action:          executedAction,
		RequestedAction: requestedAction,
		AccountID:       accountID,
		TargetCluster:   h.cfg.TargetCluster,
		Status:          store.StatusDispatched,
		ExecutionMode:   execMode,
		Scope:           meta.Scope,
		Type:            meta.Type,
		DryRun:          req.DryRun,
		Force:           req.Force,
		Jira:            req.Jira,
		Operator:        operator,
		Params:          req.Params,
		Revision:        version.GitCommit,
		TimeoutSeconds:  timeoutSeconds,
		CreatedAt:       now,
		DispatchedAt:    now,
	}

	if err := h.executionStore.Create(ctx, exec); err != nil {
		h.logger.Error("failed to create execution record", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to create execution")
		return
	}

	h.recordAudit(r, http.StatusAccepted, executedAction, executionID, withJira(req.Jira), withForce(req.Force), withDryRun(req.DryRun))

	if execMode == "sync" {
		h.executeSyncAndRespond(w, ctx, exec, action, req.Params, req.Force)
		return
	}

	// Async: dispatch resources and fire-and-forget (reconciler will manage lifecycle)
	if err := h.executor.DispatchAsync(ctx, exec, action); err != nil {
		h.logger.Error("async dispatch failed", "error", err, "execution_id", executionID)
		_ = h.executionStore.TransitionWithMetadata(ctx, executionID, store.StatusDispatched, store.StatusFailed,
			map[string]interface{}{
				"completedAt": time.Now().Format(time.RFC3339Nano),
				"durationMs":  int64(0),
			})
		emitTerminalExecution(h.cfg.TargetCluster, exec, store.StatusFailed, 0)
		writeJSON(w, http.StatusOK, createResponse{
			ID:              executionID,
			Action:          executedAction,
			RequestedAction: requestedAction,
			TargetCluster:   h.cfg.TargetCluster,
			Operator:        operator,
			Status:          string(store.StatusFailed),
			ExecutionMode:   execMode,
			Scope:           meta.Scope,
			Type:            meta.Type,
			DryRun:          req.DryRun,
			Force:           req.Force,
		})
		return
	}

	writeJSON(w, http.StatusAccepted, createResponse{
		ID:              executionID,
		Action:          executedAction,
		RequestedAction: requestedAction,
		TargetCluster:   h.cfg.TargetCluster,
		Operator:        operator,
		Status:          string(store.StatusDispatched),
		ExecutionMode:   execMode,
		Scope:           meta.Scope,
		Type:            meta.Type,
		DryRun:          req.DryRun,
		Force:           req.Force,
	})
}

func (h *Handler) executeSyncAndRespond(w http.ResponseWriter, ctx context.Context, exec *store.Execution, action actions.Action, params map[string]string, force bool) {
	timeout := time.Duration(exec.TimeoutSeconds) * time.Second
	if timeout == 0 || timeout > time.Duration(h.cfg.ExecutionDeadlineSeconds)*time.Second {
		timeout = time.Duration(h.cfg.ExecutionDeadlineSeconds) * time.Second
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, execErr := h.executor.ExecuteSync(execCtx, exec.ID, action, params, &executor.SyncContext{
		Operator:      exec.Operator,
		TargetCluster: exec.TargetCluster,
		Force:         force,
	})

	startTime, _ := time.Parse(time.RFC3339Nano, exec.CreatedAt)
	durationMs := time.Since(startTime).Milliseconds()

	finalStatus := store.StatusSucceeded
	if execCtx.Err() == context.DeadlineExceeded {
		finalStatus = store.StatusTimedOut
	} else if execErr != nil || (result != nil && !result.Success) {
		finalStatus = store.StatusFailed
	}

	updates := map[string]interface{}{
		"durationMs": durationMs,
	}
	if result != nil {
		updates["outputBytes"] = result.OutputBytes
		updates["logBytes"] = result.LogBytes
		updates["outputFormat"] = "json"
	}

	if err := h.executionStore.TransitionWithMetadata(ctx, exec.ID, store.StatusDispatched, finalStatus, updates); err != nil {
		h.logger.Error("failed to transition execution status after sync completion",
			"execution_id", exec.ID, "target_status", string(finalStatus), "error", err)
	}

	emitTerminalExecution(h.cfg.TargetCluster, exec, finalStatus, durationMs)

	resp := createResponse{
		ID:              exec.ID,
		Action:          exec.Action,
		RequestedAction: exec.RequestedAction,
		TargetCluster:   h.cfg.TargetCluster,
		Operator:        exec.Operator,
		Status:          string(finalStatus),
		ExecutionMode:   exec.ExecutionMode,
		Scope:           exec.Scope,
		Type:            exec.Type,
		DryRun:          exec.DryRun,
		Force:           exec.Force,
		DurationMs:      &durationMs,
	}
	if result != nil {
		if finalStatus == store.StatusSucceeded {
			resp.Output = result.Output
		} else {
			resp.Logs = result.Logs
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func validateParams(meta actions.ActionMetadata, params map[string]string) error {
	defined := make(map[string]bool)
	for _, p := range meta.Parameters {
		defined[p.Name] = true
		if p.Required {
			if _, ok := params[p.Name]; !ok {
				return fmt.Errorf("required parameter %q is missing", p.Name)
			}
		}
	}

	for k := range params {
		if !defined[k] {
			return fmt.Errorf("unknown parameter %q", k)
		}
	}

	return nil
}

// filterMatchingParams filters executions to those with matching params.
// - Excludes dry-run executions (they don't count towards cooldown)
// - Matches all params exactly (cooldown is per target workload, not just action)
func filterMatchingParams(executions []*store.Execution, params map[string]string) []*store.Execution {
	var matching []*store.Execution
	for _, e := range executions {
		// Dry-runs don't count towards cooldown (no real mutation happened)
		if e.DryRun {
			continue
		}
		// Check if params match (same target workload)
		if paramsMatch(e.Params, params) {
			matching = append(matching, e)
		}
	}
	return matching
}

// paramsMatch returns true if the two param maps have the same keys and values.
func emitTerminalExecution(cluster string, exec *store.Execution, status store.Status, durationMs int64) {
	if !status.IsTerminal() {
		return
	}
	metrics.EmitExecution(cluster, exec.Action, string(status), exec.ExecutionMode, exec.Scope, exec.Type, durationMs)
}

func paramsMatch(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
