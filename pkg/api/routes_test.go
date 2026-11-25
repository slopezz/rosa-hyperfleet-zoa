package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/config"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/executor"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/store"
)

// --- handleListActions ---

func TestHandleListActions_WhenCalled_ItShouldReturnAllRegistered(t *testing.T) {
	h := testHandler(&mockExecStore{})

	rr := doRequest(h, "GET", "/api/v0/trusted-actions", nil, defaultHeaders())

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)

	items, ok := resp["items"].([]interface{})
	if !ok {
		t.Fatalf("expected items array, got %T", resp["items"])
	}
	if len(items) == 0 {
		t.Error("expected at least one action in the list")
	}
}

// --- handleDescribeAction ---

func TestHandleDescribeAction_WhenActionExists_ItShouldReturnMetadata(t *testing.T) {
	h := testHandler(&mockExecStore{})

	rr := doRequest(h, "GET", "/api/v0/trusted-actions/test-read", nil, defaultHeaders())

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)

	if resp["name"] != "test-read" {
		t.Errorf("expected name=test-read, got %v", resp["name"])
	}
	if resp["scope"] != "kube" {
		t.Errorf("expected scope=kube, got %v", resp["scope"])
	}
}

func TestHandleDescribeAction_WhenActionNotFound_ItShouldReturn404(t *testing.T) {
	h := testHandler(&mockExecStore{})

	rr := doRequest(h, "GET", "/api/v0/trusted-actions/nonexistent-action", nil, defaultHeaders())

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --- handleListExecutions ---

func TestHandleListExecutions_WhenExecutionsExist_ItShouldReturnThem(t *testing.T) {
	execStore := &mockExecStore{
		executions: []*store.Execution{
			{ID: "e1", Action: "test-read", Status: store.StatusSucceeded},
			{ID: "e2", Action: "test-write", Status: store.StatusFailed},
		},
	}
	h := testHandler(execStore)

	rr := doRequest(h, "GET", "/api/v0/trusted-actions/runs", nil, defaultHeaders())

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)

	items, ok := resp["items"].([]interface{})
	if !ok {
		t.Fatalf("expected items array, got %T", resp["items"])
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
	count, ok := resp["count"].(float64)
	if !ok || int(count) != 2 {
		t.Errorf("expected count=2, got %v", resp["count"])
	}
}

func TestHandleListExecutions_WhenMissingAuthenticatedIAM_ItShouldReturn401(t *testing.T) {
	h := testHandler(&mockExecStore{})

	rr := doRequest(h, "GET", "/api/v0/trusted-actions/runs", nil, map[string]string{
		"__skip_iam": "true",
	})

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleListExecutions_WhenStoreError_ItShouldReturn500(t *testing.T) {
	execStore := &errorExecStore{err: fmt.Errorf("connection refused")}
	h := testHandler(execStore)

	rr := doRequest(h, "GET", "/api/v0/trusted-actions/runs", nil, defaultHeaders())

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --- handleAudit ---

func TestHandleAudit_WhenEntriesExist_ItShouldReturnThem(t *testing.T) {
	auditStore := &mockAuditStoreWithData{
		entries: []*store.AuditEntry{
			{AccountID: "acc1", Method: "POST", Path: "/api/v0/trusted-actions/test-read/run"},
		},
	}
	h := testHandlerWithAudit(&mockExecStore{}, auditStore)

	rr := doRequest(h, "GET", "/api/v0/audit", nil, defaultHeaders())

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	items, ok := resp["items"].([]interface{})
	if !ok || len(items) != 1 {
		t.Errorf("expected 1 audit entry, got %v", resp["items"])
	}
}

func TestHandleAudit_WhenNoAccountID_ItShouldStillReturn200(t *testing.T) {
	h := testHandlerWithAudit(&mockExecStore{}, &mockAuditStoreWithData{})

	rr := doRequest(h, "GET", "/api/v0/audit", nil, map[string]string{})

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --- handleVersion ---

func TestHandleVersion_WhenCalled_ItShouldReturnVersion(t *testing.T) {
	h := testHandler(&mockExecStore{})

	rr := doRequest(h, "GET", "/version", nil, nil)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if _, ok := resp["version"]; !ok {
		t.Error("expected version field in response")
	}
}

// --- helpers ---

type errorExecStore struct {
	mockExecStore
	err error
}

func (e *errorExecStore) List(_ context.Context, _ string, _ int, _ *store.ListFilter) ([]*store.Execution, error) {
	return nil, e.err
}
func (e *errorExecStore) ListAll(_ context.Context, _ int, _ *store.ListFilter) ([]*store.Execution, error) {
	return nil, e.err
}

type mockAuditStoreWithData struct {
	entries []*store.AuditEntry
}

func (m *mockAuditStoreWithData) Record(_ context.Context, _ *store.AuditEntry) error { return nil }
func (m *mockAuditStoreWithData) List(_ context.Context, _ string, _ *store.AuditFilter) ([]*store.AuditEntry, error) {
	return m.entries, nil
}
func (m *mockAuditStoreWithData) ListAll(_ context.Context, _ *store.AuditFilter) ([]*store.AuditEntry, error) {
	return m.entries, nil
}

func testHandlerWithAudit(execStore store.ExecutionStore, auditStore store.AuditStore) *Handler {
	cfg := &config.Config{
		HandlerMode:              "api",
		ArtifactBucket:           "test-bucket",
		WriteCooldownSeconds:     300,
		MaxConcurrentPerTarget:   5,
		TargetCluster:            "test-cluster",
		UploaderRoleARN:          "arn:aws:iam::123:role/uploader",
		JobImage:                 "test:latest",
		ExecutionDeadlineSeconds: 295,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	exec := executor.New(nil, nil, nil, nil, executor.ExecutorConfig{
		ArtifactBucket:  "test-bucket",
		UploaderRoleARN: "arn:aws:iam::123:role/uploader",
		Region:          "us-east-1",
		JobImage:        "test:latest",
	}, logger)
	return New(cfg, execStore, auditStore, exec, nil, logger)
}

// --- Filter tests ---

type filterCapturingExecStore struct {
	mockExecStore
	capturedFilter *store.ListFilter
	capturedLimit  int
}

func (m *filterCapturingExecStore) List(_ context.Context, _ string, limit int, filter *store.ListFilter) ([]*store.Execution, error) {
	m.capturedFilter = filter
	m.capturedLimit = limit
	return m.executions, nil
}
func (m *filterCapturingExecStore) ListAll(_ context.Context, limit int, filter *store.ListFilter) ([]*store.Execution, error) {
	m.capturedFilter = filter
	m.capturedLimit = limit
	return m.executions, nil
}

type filterCapturingAuditStore struct {
	entries        []*store.AuditEntry
	capturedFilter *store.AuditFilter
}

func (m *filterCapturingAuditStore) Record(_ context.Context, _ *store.AuditEntry) error { return nil }
func (m *filterCapturingAuditStore) List(_ context.Context, _ string, filter *store.AuditFilter) ([]*store.AuditEntry, error) {
	m.capturedFilter = filter
	return m.entries, nil
}
func (m *filterCapturingAuditStore) ListAll(_ context.Context, filter *store.AuditFilter) ([]*store.AuditEntry, error) {
	m.capturedFilter = filter
	return m.entries, nil
}

func TestHandleListExecutions_WhenFiltersProvided_ItShouldPassFilterToStore(t *testing.T) {
	execStore := &filterCapturingExecStore{}
	h := testHandler(execStore)

	rr := doRequest(h, "GET", "/api/v0/trusted-actions/runs?status=failed&type=write&scope=kube&execution_mode=async&action=rollout_restart&since=1h&limit=10", nil, defaultHeaders())

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	f := execStore.capturedFilter
	if f == nil {
		t.Fatal("expected filter to be passed to store")
		return
	}
	if f.Status == nil || *f.Status != store.StatusFailed {
		t.Errorf("expected status=failed, got %v", f.Status)
	}
	if f.Type == nil || *f.Type != "write" {
		t.Errorf("expected type=write, got %v", f.Type)
	}
	if f.Scope == nil || *f.Scope != "kube" {
		t.Errorf("expected scope=kube, got %v", f.Scope)
	}
	if f.ExecutionMode == nil || *f.ExecutionMode != "async" {
		t.Errorf("expected execution_mode=async, got %v", f.ExecutionMode)
	}
	if f.Action == nil || *f.Action != "rollout_restart" {
		t.Errorf("expected action=rollout_restart, got %v", f.Action)
	}
	if f.Since == nil {
		t.Error("expected since to be set")
	}
	if f.Limit != 10 {
		t.Errorf("expected limit=10, got %d", f.Limit)
	}
}

func TestHandleListExecutions_WhenTargetProvided_ItShouldPassTargetFilter(t *testing.T) {
	execStore := &filterCapturingExecStore{}
	h := testHandler(execStore)

	rr := doRequest(h, "GET", "/api/v0/trusted-actions/runs?target=eph-dev-mc01", nil, defaultHeaders())

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	f := execStore.capturedFilter
	if f == nil {
		t.Fatal("expected filter to be passed to store")
		return
	}
	if f.Target == nil || *f.Target != "eph-dev-mc01" {
		t.Errorf("expected target=eph-dev-mc01, got %v", f.Target)
	}
}

func TestHandleAudit_WhenTargetProvided_ItShouldPassTargetFilter(t *testing.T) {
	auditStore := &filterCapturingAuditStore{}
	h := testHandlerWithAudit(&mockExecStore{}, auditStore)

	rr := doRequest(h, "GET", "/api/v0/audit?target=eph-dev-rc", nil, defaultHeaders())

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	f := auditStore.capturedFilter
	if f == nil {
		t.Fatal("expected filter to be passed to store")
		return
	}
	if f.Target == nil || *f.Target != "eph-dev-rc" {
		t.Errorf("expected target=eph-dev-rc, got %v", f.Target)
	}
}

func TestHandleListExecutions_WhenLimitExceeds100_ItShouldCap(t *testing.T) {
	execStore := &filterCapturingExecStore{}
	h := testHandler(execStore)

	doRequest(h, "GET", "/api/v0/trusted-actions/runs?limit=500", nil, defaultHeaders())

	if execStore.capturedLimit != 100 {
		t.Errorf("expected limit capped to 100, got %d", execStore.capturedLimit)
	}
}

func TestHandleAudit_WhenFiltersProvided_ItShouldPassFilterToStore(t *testing.T) {
	auditStore := &filterCapturingAuditStore{}
	h := testHandlerWithAudit(&mockExecStore{}, auditStore)

	rr := doRequest(h, "GET", "/api/v0/audit?method=POST&since=24h&limit=15", nil, defaultHeaders())

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	f := auditStore.capturedFilter
	if f == nil {
		t.Fatal("expected filter to be passed to store")
		return
	}
	if f.Method == nil || *f.Method != "POST" {
		t.Errorf("expected method=POST, got %v", f.Method)
	}
	if f.Since == nil {
		t.Error("expected since to be set")
	}
	if f.Limit != 15 {
		t.Errorf("expected limit=15, got %d", f.Limit)
	}
}

func TestHandleAudit_WhenLimitExceeds200_ItShouldCap(t *testing.T) {
	auditStore := &filterCapturingAuditStore{}
	h := testHandlerWithAudit(&mockExecStore{}, auditStore)

	doRequest(h, "GET", "/api/v0/audit?limit=999", nil, defaultHeaders())

	if auditStore.capturedFilter == nil {
		t.Fatal("expected filter to be set")
	}
	if auditStore.capturedFilter.Limit != 200 {
		t.Errorf("expected limit capped to 200, got %d", auditStore.capturedFilter.Limit)
	}
}

// --- Audit recording tests ---

type auditCapturingStore struct {
	entries  []*store.AuditEntry
	recorded []*store.AuditEntry
}

func (m *auditCapturingStore) Record(_ context.Context, e *store.AuditEntry) error {
	m.recorded = append(m.recorded, e)
	return nil
}
func (m *auditCapturingStore) List(_ context.Context, _ string, _ *store.AuditFilter) ([]*store.AuditEntry, error) {
	return m.entries, nil
}
func (m *auditCapturingStore) ListAll(_ context.Context, _ *store.AuditFilter) ([]*store.AuditEntry, error) {
	return m.entries, nil
}

func TestHandleGetExecution_WhenFound_ItShouldRecordAudit(t *testing.T) {
	execStore := &mockExecStore{
		executions: []*store.Execution{
			{ID: "exec-123", Action: "get_resource", Status: store.StatusSucceeded},
		},
	}
	auditStore := &auditCapturingStore{}
	h := testHandlerWithAudit(execStore, auditStore)

	rr := doRequest(h, "GET", "/api/v0/trusted-actions/runs/exec-123", nil, defaultHeaders())

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if len(auditStore.recorded) == 0 {
		t.Fatal("expected audit entry to be recorded for GET /runs/{id}")
	}
	entry := auditStore.recorded[0]
	if entry.Method != "GET" {
		t.Errorf("expected method=GET, got %s", entry.Method)
	}
	if entry.ExecutionID != "exec-123" {
		t.Errorf("expected execution_id=exec-123, got %s", entry.ExecutionID)
	}
	if entry.Action != AuditActionRunGet {
		t.Errorf("expected action=%s, got %s", AuditActionRunGet, entry.Action)
	}
	if entry.StatusCode != http.StatusOK {
		t.Errorf("expected status_code=200, got %d", entry.StatusCode)
	}
}

func TestHandleListExecutions_WhenCalled_ItShouldRecordAudit(t *testing.T) {
	auditStore := &auditCapturingStore{}
	h := testHandlerWithAudit(&mockExecStore{}, auditStore)

	rr := doRequest(h, "GET", "/api/v0/trusted-actions/runs", nil, defaultHeaders())

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if len(auditStore.recorded) == 0 {
		t.Fatal("expected audit entry to be recorded for GET /runs")
	}
	if auditStore.recorded[0].Method != "GET" {
		t.Errorf("expected method=GET, got %s", auditStore.recorded[0].Method)
	}
	if auditStore.recorded[0].Action != AuditActionRunsList {
		t.Errorf("expected action=%s, got %s", AuditActionRunsList, auditStore.recorded[0].Action)
	}
}

func TestHandleAudit_WhenCalled_ItShouldRecordAudit(t *testing.T) {
	auditStore := &auditCapturingStore{}
	h := testHandlerWithAudit(&mockExecStore{}, auditStore)

	rr := doRequest(h, "GET", "/api/v0/audit", nil, defaultHeaders())

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if len(auditStore.recorded) == 0 {
		t.Fatal("expected audit entry to be recorded for GET /audit")
	}
	if auditStore.recorded[0].Method != "GET" {
		t.Errorf("expected method=GET, got %s", auditStore.recorded[0].Method)
	}
	if auditStore.recorded[0].Path != "/api/v0/audit" {
		t.Errorf("expected recorded path /api/v0/audit, got %q", auditStore.recorded[0].Path)
	}
	if auditStore.recorded[0].Action != AuditActionQuery {
		t.Errorf("expected action=%s, got %s", AuditActionQuery, auditStore.recorded[0].Action)
	}
}

// --- parseTimeValue ---

func TestParseTimeValue_WhenValidDurations_ItShouldReturnCorrectTime(t *testing.T) {
	cases := []struct {
		input  string
		minAgo int64
		maxAgo int64
	}{
		{"1h", 3500, 3700},
		{"24h", 86300, 86500},
		{"7d", 604700, 604900},
		{"30m", 1750, 1850},
		{"60s", 55, 65},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			result, err := parseTimeValue(tc.input)
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.input, err)
			}
			agoSec := int64(time.Since(result).Seconds())
			if agoSec < tc.minAgo || agoSec > tc.maxAgo {
				t.Errorf("expected %q to be %d-%ds ago, got %ds", tc.input, tc.minAgo, tc.maxAgo, agoSec)
			}
		})
	}
}

func TestParseTimeValue_WhenShortDate_ItShouldReturnStartOfDay(t *testing.T) {
	result, err := parseTimeValue("2026-08-25")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	if !result.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, result)
	}
}

func TestParseTimeValue_WhenRFC3339_ItShouldReturnExactTime(t *testing.T) {
	result, err := parseTimeValue("2026-08-25T14:30:00Z")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Date(2026, 8, 25, 14, 30, 0, 0, time.UTC)
	if !result.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, result)
	}
}

func TestParseTimeValue_WhenRFC3339WithOffset_ItShouldReturnExactTime(t *testing.T) {
	result, err := parseTimeValue("2026-08-25T16:30:00+02:00")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Date(2026, 8, 25, 14, 30, 0, 0, time.UTC)
	if !result.UTC().Equal(expected) {
		t.Errorf("expected %v UTC, got %v UTC", expected, result.UTC())
	}
}

func TestParseTimeValue_WhenInvalid_ItShouldReturnError(t *testing.T) {
	cases := []string{"", "h", "abc", "1x", "1"}
	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			_, err := parseTimeValue(input)
			if err == nil {
				t.Errorf("expected error for %q, got nil", input)
			}
		})
	}
}

// --- parseUntilTimeValue ---

func TestParseUntilTimeValue_WhenShortDate_ItShouldReturnEndOfDay(t *testing.T) {
	result, err := parseUntilTimeValue("2026-08-25")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	if !result.Equal(expected) {
		t.Errorf("expected %v (end of 2026-08-25), got %v", expected, result)
	}
}

func TestParseUntilTimeValue_WhenRFC3339_ItShouldReturnExactTime(t *testing.T) {
	result, err := parseUntilTimeValue("2026-08-25T14:30:00Z")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Date(2026, 8, 25, 14, 30, 0, 0, time.UTC)
	if !result.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, result)
	}
}

func TestParseUntilTimeValue_WhenDuration_ItShouldSubtractFromNow(t *testing.T) {
	result, err := parseUntilTimeValue("1h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	agoSec := int64(time.Since(result).Seconds())
	if agoSec < 3500 || agoSec > 3700 {
		t.Errorf("expected ~3600s ago, got %ds", agoSec)
	}
}

func TestParseUntilTimeValue_WhenInvalid_ItShouldReturnError(t *testing.T) {
	cases := []string{"", "h", "abc", "1x", "1"}
	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			_, err := parseUntilTimeValue(input)
			if err == nil {
				t.Errorf("expected error for %q, got nil", input)
			}
		})
	}
}
