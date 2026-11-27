package handler

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/store"
)

func TestFlattenHeaders_WhenMultipleHeaders_ItShouldJoinValues(t *testing.T) {
	h := http.Header{
		"Content-Type": []string{"application/json"},
		"X-Custom":     []string{"a", "b"},
	}
	result := flattenHeaders(h)
	if result["Content-Type"] != "application/json" {
		t.Errorf("expected 'application/json', got %q", result["Content-Type"])
	}
	if result["X-Custom"] != "a,b" {
		t.Errorf("expected 'a,b', got %q", result["X-Custom"])
	}
}

func TestFlattenHeaders_WhenEmpty_ItShouldReturnEmptyMap(t *testing.T) {
	result := flattenHeaders(http.Header{})
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

func TestHandleEvent_WhenUnrecognizedEvent_ItShouldReturnError(t *testing.T) {
	l := New(Deps{})
	raw := json.RawMessage(`{"unknown_field": "value"}`)
	_, err := l.HandleEvent(context.Background(), raw)
	if err == nil {
		t.Fatal("expected error for unrecognized event")
	}
	if err.Error() != "unrecognized event type" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestHandleEvent_WhenInvalidJSON_ItShouldReturnError(t *testing.T) {
	l := New(Deps{})
	raw := json.RawMessage(`not json`)
	_, err := l.HandleEvent(context.Background(), raw)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestHandleEvent_WhenHTTPEventWithNoHandler_ItShouldReturn503(t *testing.T) {
	l := New(Deps{})
	event := map[string]interface{}{
		"requestContext": map[string]interface{}{
			"http": map[string]string{"method": "GET", "path": "/version"},
		},
	}
	raw, _ := json.Marshal(event)
	result, err := l.HandleEvent(context.Background(), raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Result should be an APIGatewayV2HTTPResponse with 503
	data, _ := json.Marshal(result)
	if !contains(string(data), "503") {
		t.Errorf("expected 503 in response, got %s", data)
	}
}

func TestHandleEvent_WhenExecutionEventMissingID_ItShouldReturnError(t *testing.T) {
	l := New(Deps{
		Cfg: (&mockConfig{mode: "worker"}).toConfig(),
	})
	event := map[string]interface{}{
		"route":        "execute",
		"execution_id": "",
	}
	raw, _ := json.Marshal(event)
	_, err := l.HandleEvent(context.Background(), raw)
	if err == nil {
		t.Fatal("expected error for missing execution_id")
	}
}

func TestHandleEvent_WhenScheduledEventInAPIMode_ItShouldSkip(t *testing.T) {
	l := New(Deps{
		Cfg: (&mockConfig{mode: "api"}).toConfig(),
	})
	event := map[string]interface{}{
		"route": "reconciler",
	}
	raw, _ := json.Marshal(event)
	result, err := l.HandleEvent(context.Background(), raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, _ := json.Marshal(result)
	if !contains(string(data), "skipped") {
		t.Errorf("expected skipped response, got %s", data)
	}
}

func TestHandleEvent_WhenExecutionEventInAPIMode_ItShouldSkip(t *testing.T) {
	l := New(Deps{
		Cfg: (&mockConfig{mode: "api"}).toConfig(),
	})
	event := map[string]interface{}{
		"route":        "execute",
		"execution_id": "exec-123",
	}
	raw, _ := json.Marshal(event)
	result, err := l.HandleEvent(context.Background(), raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, _ := json.Marshal(result)
	if !contains(string(data), "skipped") {
		t.Errorf("expected skipped response, got %s", data)
	}
}

func TestHttpRequestFromEvent_WhenValidEvent_ItShouldConstructRequest(t *testing.T) {
	// Test directly with a minimal event that has required fields
	event := newMockHTTPEvent("GET", "/api/v0/trusted-actions", "action=get_pods")
	req, err := httpRequestFromEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Method != "GET" {
		t.Errorf("expected GET, got %s", req.Method)
	}
	if req.URL.Path != "/api/v0/trusted-actions" {
		t.Errorf("expected path '/api/v0/trusted-actions', got %q", req.URL.Path)
	}
	if req.URL.RawQuery != "action=get_pods" {
		t.Errorf("expected query 'action=get_pods', got %q", req.URL.RawQuery)
	}
}

func TestHttpRequestFromEvent_WhenHeaders_ItShouldCopyThem(t *testing.T) {
	event := newMockHTTPEvent("POST", "/api/v0/trusted-actions/run", "")
	event.Headers = map[string]string{
		"content-type":    "application/json",
		"x-account-id":    "123456789012",
		"x-forwarded-for": "1.2.3.4",
	}
	req, err := httpRequestFromEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Header.Get("Content-Type") != "application/json" {
		t.Errorf("expected content-type header")
	}
	if req.Header.Get("X-Account-Id") != "123456789012" {
		t.Errorf("expected x-account-id header")
	}
}

func TestResponseWriter_WhenWrite_ItShouldDefaultTo200(t *testing.T) {
	rw := &responseWriter{headers: make(http.Header)}
	_, err := rw.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rw.statusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", rw.statusCode)
	}
	if rw.body.String() != "hello" {
		t.Errorf("expected 'hello', got %q", rw.body.String())
	}
}

func TestResponseWriter_WhenWriteHeader_ItShouldSetStatusCode(t *testing.T) {
	rw := &responseWriter{headers: make(http.Header)}
	rw.WriteHeader(http.StatusNotFound)
	if rw.statusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rw.statusCode)
	}
}

func TestResponseWriter_WhenHeader_ItShouldReturnHeaders(t *testing.T) {
	rw := &responseWriter{headers: make(http.Header)}
	rw.Header().Set("Content-Type", "application/json")
	if rw.headers.Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type header")
	}
}

func TestHandleEvent_WhenScheduledEventWithUnknownRoute_ItShouldReturnError(t *testing.T) {
	l := New(Deps{
		Cfg:    (&mockConfig{mode: "worker"}).toConfig(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	event := map[string]interface{}{
		"route": "unknown-route",
	}
	raw, _ := json.Marshal(event)
	_, err := l.HandleEvent(context.Background(), raw)
	if err == nil {
		t.Fatal("expected error for unknown route")
	}
}

func TestHandleEvent_WhenDetailTypePresent_ItShouldRouteAsScheduled(t *testing.T) {
	l := New(Deps{
		Cfg:    (&mockConfig{mode: "api"}).toConfig(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	event := map[string]interface{}{
		"detail-type": "Scheduled Event",
		"detail":      map[string]string{"route": "reconciler"},
	}
	raw, _ := json.Marshal(event)
	result, err := l.HandleEvent(context.Background(), raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, _ := json.Marshal(result)
	if !contains(string(data), "skipped") {
		t.Errorf("expected skipped (api mode), got %s", data)
	}
}

func TestHandleEvent_WhenExecutionEventWithNonexistentExec_ItShouldReturnError(t *testing.T) {
	mockStore := &mockExecStore{getResult: nil}
	l := New(Deps{
		Cfg:       (&mockConfig{mode: "worker"}).toConfig(),
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		ExecStore: mockStore,
	})
	event := map[string]interface{}{
		"route":        "execute",
		"execution_id": "nonexistent-123",
	}
	raw, _ := json.Marshal(event)
	_, err := l.HandleEvent(context.Background(), raw)
	if err == nil {
		t.Fatal("expected error for nonexistent execution")
	}
}

func TestHandleEvent_WhenExecutionEventWithUnregisteredAction_ItShouldFail(t *testing.T) {
	mockStore := &mockExecStore{
		getResult: &store.Execution{
			ID:            "exec-bad-action",
			Action:        "nonexistent_action_xyz",
			Status:        store.StatusDispatched,
			ExecutionMode: "sync",
		},
	}
	l := New(Deps{
		Cfg:       (&mockConfig{mode: "worker"}).toConfig(),
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		ExecStore: mockStore,
	})
	event := map[string]interface{}{
		"route":        "execute",
		"execution_id": "exec-bad-action",
	}
	raw, _ := json.Marshal(event)
	_, err := l.HandleEvent(context.Background(), raw)
	if err == nil {
		t.Fatal("expected error for unregistered action")
	}
}

func TestHandleEvent_WhenHTTPEventWithBody_ItShouldParseCorrectly(t *testing.T) {
	l := New(Deps{
		Cfg:    (&mockConfig{mode: "api"}).toConfig(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	event := map[string]interface{}{
		"requestContext": map[string]interface{}{
			"http": map[string]string{"method": "POST", "path": "/api/v0/trusted-actions/get_pods/run"},
		},
		"rawPath":        "/api/v0/trusted-actions/get_pods/run",
		"rawQueryString": "",
		"headers":        map[string]string{"content-type": "application/json"},
		"body":           `{"params":{"namespace":"grafana"}}`,
	}
	raw, _ := json.Marshal(event)
	result, err := l.HandleEvent(context.Background(), raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, _ := json.Marshal(result)
	// Without a handler, should get 503 mode_mismatch (handler is nil)
	if !contains(string(data), "503") {
		t.Errorf("expected 503 (no handler), got %s", data)
	}
}

func TestHandleEvent_WhenExecutionAlreadySucceeded_ItShouldSkipDuplicate(t *testing.T) {
	mockStore := &mockExecStore{
		getResult: &store.Execution{
			ID:            "exec-already-done",
			Action:        "get_resource",
			Status:        store.StatusSucceeded,
			ExecutionMode: "sync",
		},
	}
	l := New(Deps{
		Cfg:       (&mockConfig{mode: "worker"}).toConfig(),
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		ExecStore: mockStore,
	})
	event := map[string]interface{}{
		"route":        "execute",
		"execution_id": "exec-already-done",
	}
	raw, _ := json.Marshal(event)
	result, err := l.HandleEvent(context.Background(), raw)
	if err != nil {
		t.Fatalf("expected nil error for idempotent skip, got: %v", err)
	}
	data, _ := json.Marshal(result)
	if !contains(string(data), "ok") {
		t.Errorf("expected ok status for skipped duplicate, got %s", data)
	}
}

func TestHandleEvent_WhenExecutionAlreadyFailed_ItShouldSkipDuplicate(t *testing.T) {
	mockStore := &mockExecStore{
		getResult: &store.Execution{
			ID:            "exec-already-failed",
			Action:        "delete_pod",
			Status:        store.StatusFailed,
			ExecutionMode: "sync",
		},
	}
	l := New(Deps{
		Cfg:       (&mockConfig{mode: "worker"}).toConfig(),
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		ExecStore: mockStore,
	})
	event := map[string]interface{}{
		"route":        "execute",
		"execution_id": "exec-already-failed",
	}
	raw, _ := json.Marshal(event)
	result, err := l.HandleEvent(context.Background(), raw)
	if err != nil {
		t.Fatalf("expected nil error for idempotent skip, got: %v", err)
	}
	data, _ := json.Marshal(result)
	if !contains(string(data), "ok") {
		t.Errorf("expected ok status for skipped duplicate, got %s", data)
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) && (indexOfString(s, substr) >= 0))
}

func indexOfString(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
