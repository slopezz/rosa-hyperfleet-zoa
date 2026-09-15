// Package handler provides the Lambda event router. It owns all event
// dispatch logic, keeping cmd/zoa-lambda/main.go as pure DI wiring.
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"

	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/actions"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/api"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/config"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/executor"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/metrics"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/scheduler"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/store"
)

// ExecutionEvent is the payload for self-invoked TA execution.
type ExecutionEvent struct {
	Route       string `json:"route"`
	ExecutionID string `json:"execution_id"`
}

// Lambda aggregates all Lambda dependencies and provides the unified event handler.
type Lambda struct {
	cfg        *config.Config
	handler    *api.Handler
	reconciler *scheduler.Reconciler
	exec       *executor.Executor
	execStore  store.ExecutionStore
	logger     *slog.Logger
}

// Deps holds the dependencies injected by main.go.
type Deps struct {
	Cfg        *config.Config
	Handler    *api.Handler
	Reconciler *scheduler.Reconciler
	Executor   *executor.Executor
	ExecStore  store.ExecutionStore
	Logger     *slog.Logger
}

func New(d Deps) *Lambda {
	return &Lambda{
		cfg:        d.Cfg,
		handler:    d.Handler,
		reconciler: d.Reconciler,
		exec:       d.Executor,
		execStore:  d.ExecStore,
		logger:     d.Logger,
	}
}

// HandleEvent is the Lambda entry point. It routes incoming events based on
// their structure:
//   - "route"="execute"  → self-invoked TA execution (worker mode)
//   - "detail-type" present → EventBridge scheduled event (worker mode)
//   - "requestContext" present → Function URL / API Gateway HTTP event (api mode)
func (l *Lambda) HandleEvent(ctx context.Context, rawEvent json.RawMessage) (interface{}, error) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(rawEvent, &probe); err != nil {
		return nil, fmt.Errorf("unmarshaling event: %w", err)
	}

	if routeRaw, hasRoute := probe["route"]; hasRoute {
		var route string
		if err := json.Unmarshal(routeRaw, &route); err == nil {
			if route == "execute" {
				return l.handleExecutionEvent(ctx, rawEvent)
			}
			// EventBridge Scheduler sends {"route":"reconciler"} / {"route":"gc"}
			return l.handleScheduledEvent(ctx, rawEvent)
		}
	}

	// EventBridge Rules (if using full event format with detail-type)
	if _, hasDetailType := probe["detail-type"]; hasDetailType {
		return l.handleScheduledEvent(ctx, rawEvent)
	}

	if _, hasRequestContext := probe["requestContext"]; hasRequestContext {
		return l.handleHTTPEvent(ctx, rawEvent)
	}

	return nil, fmt.Errorf("unrecognized event type")
}

func (l *Lambda) handleHTTPEvent(ctx context.Context, rawEvent json.RawMessage) (*events.APIGatewayV2HTTPResponse, error) {
	if l.handler == nil {
		return &events.APIGatewayV2HTTPResponse{
			StatusCode: http.StatusServiceUnavailable,
			Body:       `{"code":"mode_mismatch","reason":"HTTP events not handled in this mode"}`,
		}, nil
	}

	var event events.APIGatewayV2HTTPRequest
	if err := json.Unmarshal(rawEvent, &event); err != nil {
		return &events.APIGatewayV2HTTPResponse{
			StatusCode: http.StatusBadRequest,
			Body:       `{"code":"invalid_event","reason":"failed to parse HTTP event"}`,
		}, nil
	}

	req, err := httpRequestFromEvent(ctx, &event)
	if err != nil {
		return &events.APIGatewayV2HTTPResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       `{"code":"internal_error","reason":"failed to construct request"}`,
		}, nil
	}

	rw := &responseWriter{headers: make(http.Header)}
	l.handler.ServeHTTP(rw, req)

	return &events.APIGatewayV2HTTPResponse{
		StatusCode: rw.statusCode,
		Headers:    flattenHeaders(rw.headers),
		Body:       rw.body.String(),
	}, nil
}

func (l *Lambda) handleScheduledEvent(ctx context.Context, rawEvent json.RawMessage) (interface{}, error) {
	if !l.cfg.IsWorkerMode() {
		return map[string]string{"status": "skipped", "reason": "scheduled events only run in worker mode"}, nil
	}

	var event struct {
		Route  string `json:"route"`
		Detail struct {
			Route string `json:"route"`
			Task  string `json:"task"`
		} `json:"detail"`
	}
	if err := json.Unmarshal(rawEvent, &event); err != nil {
		return nil, fmt.Errorf("unmarshaling scheduled event: %w", err)
	}

	route := event.Route
	if route == "" {
		route = event.Detail.Route
	}
	if route == "" {
		route = event.Detail.Task
	}

	deadline := time.Duration(l.cfg.ReconcilerDeadlineSeconds) * time.Second
	ctx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()

	l.logger.Info("worker scheduled event", "route", route, "deadline_seconds", l.cfg.ReconcilerDeadlineSeconds)

	var runErr error
	switch route {
	case "reconciler", "reconcile":
		runErr = l.reconciler.Run(ctx)
	case "gc", "garbage-collection":
		runErr = l.reconciler.RunGC(ctx)
	default:
		return nil, fmt.Errorf("unknown worker route: %q", route)
	}

	if runErr != nil {
		return nil, fmt.Errorf("worker route %q error: %w", route, runErr)
	}
	return map[string]string{"status": "ok", "route": route}, nil
}

func (l *Lambda) handleExecutionEvent(ctx context.Context, rawEvent json.RawMessage) (interface{}, error) {
	if !l.cfg.IsWorkerMode() {
		return map[string]string{"status": "skipped", "reason": "execution events only run in worker mode"}, nil
	}

	var event ExecutionEvent
	if err := json.Unmarshal(rawEvent, &event); err != nil {
		return nil, fmt.Errorf("unmarshaling execution event: %w", err)
	}

	if event.ExecutionID == "" {
		return nil, fmt.Errorf("execution_id is required in execution event")
	}

	deadline := time.Duration(l.cfg.ExecutionDeadlineSeconds) * time.Second
	ctx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()

	l.logger.Info("worker TA execution", "execution_id", event.ExecutionID, "deadline_seconds", l.cfg.ExecutionDeadlineSeconds)

	execErr := l.runDispatchedExecution(ctx, event.ExecutionID)

	if execErr != nil {
		return nil, fmt.Errorf("execution %s failed: %w", event.ExecutionID, execErr)
	}
	return map[string]string{"status": "ok", "execution_id": event.ExecutionID}, nil
}

func (l *Lambda) runDispatchedExecution(ctx context.Context, executionID string) error {
	exec, err := l.execStore.Get(ctx, executionID)
	if err != nil {
		return fmt.Errorf("loading execution from store: %w", err)
	}
	if exec == nil {
		return fmt.Errorf("execution %s not found in store", executionID)
	}

	// Idempotency guard: Lambda async invocations retry on failure (up to 2x).
	// Without this check, a retry after a successful execution but failed state
	// transition would re-execute the TA, causing duplicate side effects.
	if exec.Status != store.StatusDispatched {
		l.logger.Info("execution already progressed past dispatched, skipping duplicate invocation",
			"execution_id", executionID, "current_status", exec.Status)
		return nil
	}

	action, ok := actions.Get(exec.Action)
	if !ok {
		_ = l.execStore.TransitionWithMetadata(ctx, executionID, store.StatusDispatched, store.StatusFailed,
			map[string]interface{}{
				"completedAt": time.Now().Format(time.RFC3339Nano),
				"durationMs":  int64(0),
			})
		emitWorkerTerminalExecution(l.cfg.TargetCluster, exec, store.StatusFailed, 0)
		return fmt.Errorf("action %q not registered", exec.Action)
	}

	if exec.ExecutionMode == "async" {
		if err := l.exec.DispatchAsync(ctx, exec, action); err != nil {
			_ = l.execStore.TransitionWithMetadata(ctx, executionID, store.StatusDispatched, store.StatusFailed,
				map[string]interface{}{
					"completedAt": time.Now().Format(time.RFC3339Nano),
					"durationMs":  int64(0),
				})
			emitWorkerTerminalExecution(l.cfg.TargetCluster, exec, store.StatusFailed, 0)
			return fmt.Errorf("async dispatch failed: %w", err)
		}
		return nil
	}

	result, execErr := l.exec.ExecuteSync(ctx, executionID, action, exec.Params, &executor.SyncContext{
		Operator:      exec.Operator,
		TargetCluster: exec.TargetCluster,
	})

	completedAt := time.Now().Format(time.RFC3339Nano)
	startTime, _ := time.Parse(time.RFC3339Nano, exec.DispatchedAt)
	durationMs := time.Since(startTime).Milliseconds()

	finalStatus := store.StatusSucceeded
	if ctx.Err() == context.DeadlineExceeded {
		finalStatus = store.StatusTimedOut
	} else if execErr != nil || (result != nil && !result.Success) {
		finalStatus = store.StatusFailed
	}

	updates := map[string]interface{}{
		"completedAt": completedAt,
		"durationMs":  durationMs,
	}

	if err := l.execStore.TransitionWithMetadata(ctx, executionID, store.StatusDispatched, finalStatus, updates); err != nil {
		l.logger.Error("failed to persist terminal state — Lambda retry will re-check via idempotency guard",
			"execution_id", executionID, "target_status", finalStatus, "error", err)
		return fmt.Errorf("persisting execution result: %w", err)
	}
	emitWorkerTerminalExecution(l.cfg.TargetCluster, exec, finalStatus, durationMs)
	return execErr
}

func emitWorkerTerminalExecution(cluster string, exec *store.Execution, status store.Status, durationMs int64) {
	if exec == nil || !status.IsTerminal() {
		return
	}
	metrics.EmitExecution(cluster, exec.Action, string(status), exec.ExecutionMode, exec.Scope, exec.Type, durationMs)
}

func httpRequestFromEvent(ctx context.Context, event *events.APIGatewayV2HTTPRequest) (*http.Request, error) {
	path := event.RawPath
	if path == "" {
		path = event.RequestContext.HTTP.Path
	}

	// Append query string — Function URL passes it separately from path
	if event.RawQueryString != "" {
		path += "?" + event.RawQueryString
	}

	method := event.RequestContext.HTTP.Method
	body := strings.NewReader(event.Body)

	req, err := http.NewRequestWithContext(ctx, method, path, body)
	if err != nil {
		return nil, err
	}

	for k, v := range event.Headers {
		req.Header.Set(k, v)
	}

	if ip := event.RequestContext.HTTP.SourceIP; ip != "" {
		req.Header.Set("X-Source-IP", ip)
	}
	if ua := event.RequestContext.HTTP.UserAgent; ua != "" && req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", ua)
	}
	if rid := event.RequestContext.RequestID; rid != "" {
		req.Header.Set("X-Request-ID", rid)
	}

	return req, nil
}

func flattenHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, v := range h {
		out[k] = strings.Join(v, ",")
	}
	return out
}
