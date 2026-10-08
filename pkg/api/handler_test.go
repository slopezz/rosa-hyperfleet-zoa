package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"

	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/actions"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/config"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/executor"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/store"
)

// --- Test mocks ---

type mockS3 struct {
	objects map[string]string // key → content
}

func (m *mockS3) GetObject(_ context.Context, params *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if m == nil || m.objects == nil {
		return nil, fmt.Errorf("NoSuchKey: key not found")
	}
	content, ok := m.objects[*params.Key]
	if !ok {
		return nil, fmt.Errorf("NoSuchKey: key %q not found", *params.Key)
	}
	size := int64(len(content))
	return &s3.GetObjectOutput{
		Body:          io.NopCloser(strings.NewReader(content)),
		ContentLength: &size,
	}, nil
}

type mockExecStore struct {
	executions     []*store.Execution
	created        []*store.Execution
	activeCount    int
	recentByTarget []*store.Execution
}

func (m *mockExecStore) Create(_ context.Context, e *store.Execution) error {
	m.created = append(m.created, e)
	return nil
}
func (m *mockExecStore) Get(_ context.Context, id string) (*store.Execution, error) {
	for _, e := range m.executions {
		if e.ID == id {
			return e, nil
		}
	}
	return nil, nil
}
func (m *mockExecStore) List(_ context.Context, _ string, _ int, _ *store.ListFilter) ([]*store.Execution, error) {
	return m.executions, nil
}
func (m *mockExecStore) ListAll(_ context.Context, _ int, _ *store.ListFilter) ([]*store.Execution, error) {
	return m.executions, nil
}
func (m *mockExecStore) TransitionStatus(_ context.Context, _ string, _, _ store.Status) error {
	return nil
}
func (m *mockExecStore) TransitionWithMetadata(_ context.Context, _ string, _, _ store.Status, _ map[string]interface{}) error {
	return nil
}
func (m *mockExecStore) QueryByStatus(_ context.Context, _ store.Status) ([]*store.Execution, error) {
	return nil, nil
}
func (m *mockExecStore) QueryByStatusAndClass(_ context.Context, _ store.Status, _ string) ([]*store.Execution, error) {
	return nil, nil
}
func (m *mockExecStore) QueryTerminal(_ context.Context, _ time.Duration) ([]*store.Execution, error) {
	return nil, nil
}
func (m *mockExecStore) MarkCleaned(_ context.Context, _ string) error { return nil }
func (m *mockExecStore) ListByTargetAndAction(_ context.Context, _, _ string, _ time.Time) ([]*store.Execution, error) {
	return m.recentByTarget, nil
}
func (m *mockExecStore) CountActiveByTarget(_ context.Context, _ string) (int, error) {
	return m.activeCount, nil
}
func (m *mockExecStore) QueryByTargetAndStatus(_ context.Context, _ string, _ store.Status) ([]*store.Execution, error) {
	return nil, nil
}
func (m *mockExecStore) QueryTerminalByTarget(_ context.Context, _ string, _ time.Duration) ([]*store.Execution, error) {
	return nil, nil
}

type mockAuditStore struct{}

func (m *mockAuditStore) Record(_ context.Context, _ *store.AuditEntry) error { return nil }
func (m *mockAuditStore) List(_ context.Context, _ string, _ *store.AuditFilter) ([]*store.AuditEntry, error) {
	return nil, nil
}
func (m *mockAuditStore) ListAll(_ context.Context, _ *store.AuditFilter) ([]*store.AuditEntry, error) {
	return nil, nil
}

func init() {
	// Register test actions
	actions.Register(&testAction{
		meta: actions.ActionMetadata{
			Name:              "test-read",
			Scope:             "kube-api",
			Type:              "read",
			ExecutionMode:     "sync",
			TimeoutSeconds:    60,
			DeploymentTargets: []string{actions.DeploymentTargetRC, actions.DeploymentTargetMC},
			Parameters: []actions.ParameterDef{
				{Name: "namespace", Required: true},
			},
		},
	})
	actions.Register(&testAction{
		meta: actions.ActionMetadata{
			Name:                 "test-write",
			Scope:                "kube-api",
			Type:                 "write",
			ExecutionMode:        "sync",
			TimeoutSeconds:       60,
			WriteCooldownSeconds: 300,
			DeploymentTargets:    []string{actions.DeploymentTargetRC, actions.DeploymentTargetMC},
			Parameters: []actions.ParameterDef{
				{Name: "namespace", Required: true},
				{Name: "name", Required: true},
			},
		},
	})
	actions.Register(&testAction{
		meta: actions.ActionMetadata{
			Name:              "test-write-dryrun",
			Scope:             "kube-api",
			Type:              "read",
			ExecutionMode:     "sync",
			TimeoutSeconds:    60,
			DryRunAction:      "test-read",
			DeploymentTargets: []string{actions.DeploymentTargetRC, actions.DeploymentTargetMC},
			Parameters: []actions.ParameterDef{
				{Name: "namespace", Required: true},
				{Name: "name", Required: true},
			},
		},
	})
}

type testAction struct {
	meta actions.ActionMetadata
}

func (t *testAction) Metadata() actions.ActionMetadata                             { return t.meta }
func (t *testAction) Validate(_ context.Context, _ *actions.ExecutionParams) error { return nil }
func (t *testAction) Execute(_ context.Context, _ *actions.ExecutionParams) (*actions.ActionResult, error) {
	return &actions.ActionResult{Success: true}, nil
}

type auditCapturingStoreHandler struct {
	recorded []*store.AuditEntry
}

func (m *auditCapturingStoreHandler) Record(_ context.Context, e *store.AuditEntry) error {
	m.recorded = append(m.recorded, e)
	return nil
}
func (m *auditCapturingStoreHandler) ListAll(_ context.Context, _ *store.AuditFilter) ([]*store.AuditEntry, error) {
	return nil, nil
}
func (m *auditCapturingStoreHandler) List(_ context.Context, _ string, _ *store.AuditFilter) ([]*store.AuditEntry, error) {
	return nil, nil
}

func testHandler(execStore store.ExecutionStore) *Handler {
	return testHandlerWithS3(execStore, nil)
}

func testHandlerWithCustomAudit(execStore store.ExecutionStore, auditStore store.AuditStore) *Handler {
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

func testHandlerWithS3(execStore store.ExecutionStore, s3Mock S3Getter) *Handler {
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
	return New(cfg, execStore, &mockAuditStore{}, exec, s3Mock, logger)
}

func doRequest(h *Handler, method, path string, body interface{}, headers map[string]string) *httptest.ResponseRecorder {
	var reqBody *bytes.Buffer
	if body != nil {
		data, _ := json.Marshal(body)
		reqBody = bytes.NewBuffer(data)
	} else {
		reqBody = &bytes.Buffer{}
	}

	req := httptest.NewRequest(method, path, reqBody)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func defaultHeaders() map[string]string {
	return map[string]string{
		"X-Account-ID": "account-123",
		"X-Operator":   "sre@redhat.com",
	}
}

// --- Tests ---

func TestHandleCreate_WhenWriteCooldown_ItShouldReturn429(t *testing.T) {
	// Cooldown is per (target, action, params) — mock must have matching params
	execStore := &mockExecStore{
		recentByTarget: []*store.Execution{
			{
				ID:     "recent-exec",
				Action: "test-write",
				Status: store.StatusSucceeded,
				Params: map[string]string{"namespace": "default", "name": "pod-1"},
			},
		},
	}
	h := testHandler(execStore)

	body := createRequest{
		Reason: "JIRA-123",
		Params: map[string]string{"namespace": "default", "name": "pod-1"},
	}

	rr := doRequest(h, "POST", "/api/v0/trusted-actions/test-write/run", body, defaultHeaders())

	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["code"] != "write_cooldown" {
		t.Errorf("expected write_cooldown code, got %q", resp["code"])
	}
}

func TestHandleCreate_WhenForceBypassesCooldown_ItShouldCreateExecution(t *testing.T) {
	execStore := &mockExecStore{
		recentByTarget: []*store.Execution{
			{ID: "recent-exec", Action: "test-write", Status: store.StatusSucceeded},
		},
	}
	h := testHandler(execStore)

	body := createRequest{
		Reason: "JIRA-123",
		Params: map[string]string{"namespace": "default", "name": "pod-1"},
		Force:  true,
	}

	rr := doRequest(h, "POST", "/api/v0/trusted-actions/test-write/run", body, defaultHeaders())

	// Force should bypass cooldown — execution should be created
	// (response may not be 200 since executor has nil internals, but it must NOT be 429)
	if rr.Code == http.StatusTooManyRequests {
		t.Errorf("force=true should bypass cooldown, but got 429: %s", rr.Body.String())
	}
	if len(execStore.created) == 0 {
		t.Fatal("force=true should bypass cooldown and create the execution")
	}
}

func TestHandleCreate_WhenDifferentParams_ItShouldNotTriggerCooldown(t *testing.T) {
	// Cooldown is per (target, action, params) — different params means different target workload
	execStore := &mockExecStore{
		recentByTarget: []*store.Execution{
			{
				ID:     "recent-exec",
				Action: "test-write",
				Status: store.StatusSucceeded,
				Params: map[string]string{"namespace": "default", "name": "pod-1"},
			},
		},
	}
	h := testHandler(execStore)

	// Same action, different params — should NOT be blocked by cooldown
	body := createRequest{
		Reason: "JIRA-123",
		Params: map[string]string{"namespace": "default", "name": "pod-2"}, // different name
	}

	rr := doRequest(h, "POST", "/api/v0/trusted-actions/test-write/run", body, defaultHeaders())

	// Should NOT get 429 — different target workload
	if rr.Code == http.StatusTooManyRequests {
		t.Errorf("different params should not trigger cooldown, but got 429: %s", rr.Body.String())
	}
}

func TestHandleCreate_WhenDryRunRecent_ItShouldNotTriggerCooldown(t *testing.T) {
	// Dry-run executions don't count towards cooldown (no real mutation)
	execStore := &mockExecStore{
		recentByTarget: []*store.Execution{
			{
				ID:     "recent-exec",
				Action: "test-write",
				Status: store.StatusSucceeded,
				Params: map[string]string{"namespace": "default", "name": "pod-1"},
				DryRun: true, // dry-run execution
			},
		},
	}
	h := testHandler(execStore)

	// Same params, but recent was dry-run — should NOT be blocked
	body := createRequest{
		Reason: "JIRA-123",
		Params: map[string]string{"namespace": "default", "name": "pod-1"},
	}

	rr := doRequest(h, "POST", "/api/v0/trusted-actions/test-write/run", body, defaultHeaders())

	// Should NOT get 429 — dry-runs don't count
	if rr.Code == http.StatusTooManyRequests {
		t.Errorf("dry-run executions should not trigger cooldown, but got 429: %s", rr.Body.String())
	}
}

func TestHandleCreate_WhenMaxConcurrentExceeded_ItShouldReturn429(t *testing.T) {
	execStore := &mockExecStore{
		activeCount: 10, // exceeds MaxConcurrentPerTarget=5
	}
	h := testHandler(execStore)

	body := createRequest{
		Reason: "JIRA-123",
		Params: map[string]string{"namespace": "default"},
	}

	rr := doRequest(h, "POST", "/api/v0/trusted-actions/test-read/run", body, defaultHeaders())

	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["code"] != "max_concurrent" {
		t.Errorf("expected max_concurrent code, got %q", resp["code"])
	}
}

func TestHandleCreate_WhenCooldownRejected_ItShouldRecordAuditWith429(t *testing.T) {
	// Cooldown is per (target, action, params) — mock must have matching params
	execStore := &mockExecStore{
		recentByTarget: []*store.Execution{
			{
				ID:     "recent-exec",
				Action: "test-write",
				Status: store.StatusSucceeded,
				Params: map[string]string{"namespace": "default", "name": "pod-1"},
			},
		},
	}
	auditCapture := &auditCapturingStoreHandler{}
	h := testHandlerWithCustomAudit(execStore, auditCapture)

	body := createRequest{
		Reason: "JIRA-123",
		Params: map[string]string{"namespace": "default", "name": "pod-1"},
	}
	rr := doRequest(h, "POST", "/api/v0/trusted-actions/test-write/run", body, defaultHeaders())

	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rr.Code)
	}
	if len(auditCapture.recorded) == 0 {
		t.Fatal("expected audit entry for rejected POST")
	}
	if auditCapture.recorded[0].StatusCode != http.StatusTooManyRequests {
		t.Errorf("expected status_code=429, got %d", auditCapture.recorded[0].StatusCode)
	}
	if auditCapture.recorded[0].Action != "test-write" {
		t.Errorf("expected action=test-write, got %s", auditCapture.recorded[0].Action)
	}
	if auditCapture.recorded[0].Reason != "JIRA-123" {
		t.Errorf("expected reason=JIRA-123, got %s", auditCapture.recorded[0].Reason)
	}
}

func TestHandleCreate_WhenDispatched_ItShouldRecordReasonInAudit(t *testing.T) {
	execStore := &mockExecStore{}
	auditCapture := &auditCapturingStoreHandler{}
	h := testHandlerWithCustomAudit(execStore, auditCapture)

	body := createRequest{
		Reason:        "ROSAENG-2024",
		Params:        map[string]string{"namespace": "default"},
		ExecutionMode: "async",
	}
	// Async dispatch may fail due to nil K8s client in test, but audit is recorded
	// at dispatch time (before execution attempt).
	_ = doRequest(h, "POST", "/api/v0/trusted-actions/test-read/run", body, defaultHeaders())

	if len(auditCapture.recorded) == 0 {
		t.Fatal("expected audit entry for dispatch")
	}
	if auditCapture.recorded[0].Reason != "ROSAENG-2024" {
		t.Errorf("expected reason=ROSAENG-2024, got %q", auditCapture.recorded[0].Reason)
	}
	if auditCapture.recorded[0].StatusCode != http.StatusAccepted {
		t.Errorf("expected audit status_code=202 (recorded at dispatch), got %d", auditCapture.recorded[0].StatusCode)
	}
	if auditCapture.recorded[0].Action != "test-read" {
		t.Errorf("expected action=test-read, got %q", auditCapture.recorded[0].Action)
	}
}

func TestHandleCreate_WhenForceBypassesMaxConcurrent_ItShouldCreateExecution(t *testing.T) {
	execStore := &mockExecStore{
		activeCount: 10, // exceeds MaxConcurrentPerTarget=5
	}
	h := testHandler(execStore)

	body := createRequest{
		Reason:        "JIRA-123",
		Params:        map[string]string{"namespace": "default"},
		Force:         true,
		ExecutionMode: "async",
	}

	rr := doRequest(h, "POST", "/api/v0/trusted-actions/test-read/run", body, defaultHeaders())

	if rr.Code == http.StatusTooManyRequests {
		t.Errorf("force=true should bypass max concurrent, but got 429: %s", rr.Body.String())
	}
	if len(execStore.created) == 0 {
		t.Fatal("force=true should bypass max concurrent and create the execution")
	}
}

func TestHandleCreate_WhenDryRun_ItShouldUseDryRunAction(t *testing.T) {
	execStore := &mockExecStore{}
	h := testHandler(execStore)

	body := createRequest{
		Reason:        "JIRA-123",
		Params:        map[string]string{"namespace": "default", "name": "pod-1"},
		DryRun:        true,
		ExecutionMode: "async", // avoid needing real executor
	}

	_ = doRequest(h, "POST", "/api/v0/trusted-actions/test-write-dryrun/run", body, defaultHeaders())

	if len(execStore.created) != 1 {
		t.Fatalf("expected 1 execution created, got %d", len(execStore.created))
	}
	if execStore.created[0].Action != "test-read" {
		t.Errorf("dry run should execute the dry-run action (test-read), got %q", execStore.created[0].Action)
	}
	if !execStore.created[0].DryRun {
		t.Error("execution should have DryRun=true")
	}
}

func TestHandleCreate_WhenForced_ItShouldRecordForceInAuditAndExecution(t *testing.T) {
	execStore := &mockExecStore{}
	auditCapture := &auditCapturingStoreHandler{}
	h := testHandlerWithCustomAudit(execStore, auditCapture)

	body := createRequest{
		Reason:        "DEMO-021",
		Params:        map[string]string{"namespace": "default"},
		Force:         true,
		ExecutionMode: "async",
	}
	_ = doRequest(h, "POST", "/api/v0/trusted-actions/test-read/run", body, defaultHeaders())

	if len(execStore.created) == 0 {
		t.Fatal("expected execution to be created")
	}
	if !execStore.created[0].Force {
		t.Error("execution should have Force=true")
	}

	if len(auditCapture.recorded) == 0 {
		t.Fatal("expected audit entry")
	}
	if !auditCapture.recorded[0].Force {
		t.Error("audit entry should have Force=true")
	}
}

func TestHandleCreate_WhenDryRunForced_ItShouldRecordBothInAudit(t *testing.T) {
	execStore := &mockExecStore{}
	auditCapture := &auditCapturingStoreHandler{}
	h := testHandlerWithCustomAudit(execStore, auditCapture)

	body := createRequest{
		Reason:        "DEMO-022",
		Params:        map[string]string{"namespace": "default", "name": "pod-1"},
		DryRun:        true,
		Force:         true,
		ExecutionMode: "async",
	}
	_ = doRequest(h, "POST", "/api/v0/trusted-actions/test-write-dryrun/run", body, defaultHeaders())

	if len(auditCapture.recorded) == 0 {
		t.Fatal("expected audit entry")
	}
	entry := auditCapture.recorded[0]
	if !entry.Force {
		t.Error("audit entry should have Force=true")
	}
	if !entry.DryRun {
		t.Error("audit entry should have DryRun=true")
	}

	if len(execStore.created) == 0 {
		t.Fatal("expected execution to be created")
	}
	exec := execStore.created[0]
	if !exec.Force {
		t.Error("execution should have Force=true")
	}
	if !exec.DryRun {
		t.Error("execution should have DryRun=true")
	}
	if exec.RequestedAction != "test-write-dryrun" {
		t.Errorf("expected requested_action=test-write-dryrun, got %q", exec.RequestedAction)
	}
	if exec.Action != "test-read" {
		t.Errorf("expected action=test-read (dry-run redirect), got %q", exec.Action)
	}
}

func TestHandleCreate_WhenReadAction_ItShouldSkipCooldownCheck(t *testing.T) {
	execStore := &mockExecStore{
		recentByTarget: []*store.Execution{
			{ID: "recent", Action: "test-read", Status: store.StatusSucceeded},
		},
	}
	h := testHandler(execStore)

	body := createRequest{
		Reason: "JIRA-123",
		Params: map[string]string{"namespace": "default"},
	}

	rr := doRequest(h, "POST", "/api/v0/trusted-actions/test-read/run", body, defaultHeaders())

	// Read actions don't have cooldown (only write actions do)
	if rr.Code == http.StatusTooManyRequests {
		t.Error("read actions should not be subject to write cooldown")
	}
}

func TestHandleCreate_WhenInvalidExecutionMode_ItShouldReturn400(t *testing.T) {
	execStore := &mockExecStore{}
	h := testHandler(execStore)

	body := createRequest{
		Reason:        "JIRA-123",
		Params:        map[string]string{"namespace": "default"},
		ExecutionMode: "invalid",
	}

	rr := doRequest(h, "POST", "/api/v0/trusted-actions/test-read/run", body, defaultHeaders())

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["code"] != "invalid_execution_mode" {
		t.Errorf("expected invalid_execution_mode code, got %q", resp["code"])
	}
}

func TestHandleCreate_WhenMustGatherGatherInvalidForDeployment_ItShouldRejectBeforeDispatch(t *testing.T) {
	actions.SetDeploymentTarget("mc")
	t.Cleanup(func() { actions.SetDeploymentTarget("") })

	execStore := &mockExecStore{}
	kubeClient := fake.NewClientset()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	exec := executor.New(kubeClient, &rest.Config{Host: "https://localhost:6443"}, nil, nil, executor.ExecutorConfig{
		DeploymentTarget: "mc",
		Region:           "us-east-1",
	}, logger)
	cfg := &config.Config{
		HandlerMode:              "api",
		ArtifactBucket:           "test-bucket",
		WriteCooldownSeconds:     300,
		MaxConcurrentPerTarget:   5,
		TargetCluster:            "test-cluster",
		ExecutionDeadlineSeconds: 295,
	}
	h := New(cfg, execStore, &mockAuditStore{}, exec, nil, logger)

	body := createRequest{
		Reason: "RO-123",
		Params: map[string]string{"gather": "rc"},
	}

	rr := doRequest(h, "POST", "/api/v0/trusted-actions/must_gather/run", body, defaultHeaders())

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["code"] != "validation_failed" {
		t.Errorf("expected validation_failed code, got %q", resp["code"])
	}
	if !strings.Contains(resp["reason"], "not allowed on mc ZOA endpoint") {
		t.Errorf("expected deployment gather error, got %q", resp["reason"])
	}
	if len(execStore.created) != 0 {
		t.Fatalf("expected no execution record, got %d", len(execStore.created))
	}
}

func TestHandleCreate_WhenMustGatherSyncOverride_ItShouldReturn400(t *testing.T) {
	execStore := &mockExecStore{}
	kubeClient := fake.NewClientset()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	exec := executor.New(kubeClient, &rest.Config{Host: "https://localhost:6443"}, nil, nil, executor.ExecutorConfig{
		Region: "us-east-1",
	}, logger)
	cfg := &config.Config{
		HandlerMode:              "api",
		ArtifactBucket:           "test-bucket",
		WriteCooldownSeconds:     300,
		MaxConcurrentPerTarget:   5,
		TargetCluster:            "test-cluster",
		ExecutionDeadlineSeconds: 295,
	}
	h := New(cfg, execStore, &mockAuditStore{}, exec, nil, logger)

	body := createRequest{
		Reason:        "JIRA-123",
		Params:        map[string]string{"gather": "mc", "skip_must_gather_image": "true"},
		ExecutionMode: "sync",
	}

	rr := doRequest(h, "POST", "/api/v0/trusted-actions/must_gather/run", body, defaultHeaders())

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["code"] != "execution_mode_locked" {
		t.Errorf("expected execution_mode_locked code, got %q", resp["code"])
	}
}

func TestHandleCreate_WhenAsyncOverride_ItShouldSetExecutionModeAsync(t *testing.T) {
	execStore := &mockExecStore{}
	h := testHandler(execStore)

	body := createRequest{
		Reason:        "JIRA-123",
		Params:        map[string]string{"namespace": "default"},
		ExecutionMode: "async",
	}

	rr := doRequest(h, "POST", "/api/v0/trusted-actions/test-read/run", body, defaultHeaders())

	// Async will fail because executor is nil, but the execution record should have async class
	if len(execStore.created) != 1 {
		t.Fatalf("expected 1 execution created, got %d", len(execStore.created))
	}
	if execStore.created[0].ExecutionMode != "async" {
		t.Errorf("expected execution_mode=async, got %q", execStore.created[0].ExecutionMode)
	}
	_ = rr
}

func TestHandleCreate_WhenMissingAccountID_ItShouldReturn400(t *testing.T) {
	h := testHandler(&mockExecStore{})

	body := createRequest{
		Params: map[string]string{"namespace": "default"},
	}

	rr := doRequest(h, "POST", "/api/v0/trusted-actions/test-read/run", body, map[string]string{
		"X-Operator": "sre@redhat.com",
	})

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestHandleCreate_WhenMissingReason_ItShouldReturn400(t *testing.T) {
	h := testHandler(&mockExecStore{})

	body := createRequest{
		Params: map[string]string{"namespace": "default"},
	}

	rr := doRequest(h, "POST", "/api/v0/trusted-actions/test-read/run", body, defaultHeaders())

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing reason, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleCreate_WhenUnknownAction_ItShouldReturn404(t *testing.T) {
	h := testHandler(&mockExecStore{})

	body := createRequest{
		Params: map[string]string{},
	}

	rr := doRequest(h, "POST", "/api/v0/trusted-actions/nonexistent/run", body, defaultHeaders())

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleCreate_WhenMissingRequiredParam_ItShouldReturn400(t *testing.T) {
	h := testHandler(&mockExecStore{})

	body := createRequest{
		Params: map[string]string{}, // missing "namespace"
	}

	rr := doRequest(h, "POST", "/api/v0/trusted-actions/test-read/run", body, defaultHeaders())

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --- GET /runs/{id} with ?include= tests ---

func TestHandleGetExecution_WhenIncludeOutput_ItShouldEmbedS3Content(t *testing.T) {
	execStore := &mockExecStore{
		executions: []*store.Execution{
			{
				ID:          "exec-001",
				Action:      "get_resource",
				Status:      store.StatusSucceeded,
				DurationMs:  2000,
				OutputBytes: 42,
			},
		},
	}
	s3Mock := &mockS3{
		objects: map[string]string{
			"executions/exec-001/output.json": `{"success":true,"output":[{"name":"pod-1"}]}`,
		},
	}
	h := testHandlerWithS3(execStore, s3Mock)

	rr := doRequest(h, "GET", "/api/v0/trusted-actions/runs/exec-001?include=output", nil, defaultHeaders())

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)

	outputRaw := resp["output"]
	if outputRaw == nil {
		t.Fatalf("expected non-nil output field")
	}
	outputJSON, err := json.Marshal(outputRaw)
	if err != nil {
		t.Fatalf("failed to marshal output: %v", err)
	}
	if !strings.Contains(string(outputJSON), "pod-1") {
		t.Errorf("expected output to contain pod-1, got %q", string(outputJSON))
	}
}

func TestHandleGetExecution_WhenIncludeLogs_ItShouldEmbedS3Logs(t *testing.T) {
	execStore := &mockExecStore{
		executions: []*store.Execution{
			{
				ID:       "exec-002",
				Action:   "get_resource",
				Status:   store.StatusFailed,
				LogBytes: 100,
			},
		},
	}
	s3Mock := &mockS3{
		objects: map[string]string{
			"executions/exec-002/execution.log": "2026-08-12T10:00:00Z ERROR: validation failed: unsupported resource",
		},
	}
	h := testHandlerWithS3(execStore, s3Mock)

	rr := doRequest(h, "GET", "/api/v0/trusted-actions/runs/exec-002?include=logs", nil, defaultHeaders())

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)

	logs, ok := resp["logs"].(string)
	if !ok || logs == "" {
		t.Errorf("expected non-empty logs field, got %v", resp["logs"])
	}
	if !strings.Contains(logs, "validation failed") {
		t.Errorf("expected logs to contain error message, got %q", logs)
	}
}

func TestHandleGetExecution_WhenIncludeAll_ItShouldEmbedBoth(t *testing.T) {
	execStore := &mockExecStore{
		executions: []*store.Execution{
			{
				ID:          "exec-003",
				Action:      "get_resource",
				Status:      store.StatusSucceeded,
				OutputBytes: 10,
				LogBytes:    20,
			},
		},
	}
	s3Mock := &mockS3{
		objects: map[string]string{
			"executions/exec-003/output.json":   `{"success":true}`,
			"executions/exec-003/execution.log": "INFO: done",
		},
	}
	h := testHandlerWithS3(execStore, s3Mock)

	rr := doRequest(h, "GET", "/api/v0/trusted-actions/runs/exec-003?include=output,logs", nil, defaultHeaders())

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)

	if resp["output"] == nil || resp["output"] == "" {
		t.Error("expected output to be populated")
	}
	if resp["logs"] == nil || resp["logs"] == "" {
		t.Error("expected logs to be populated")
	}
}

func TestHandleGetExecution_WhenPending_ItShouldNotFetchS3(t *testing.T) {
	execStore := &mockExecStore{
		executions: []*store.Execution{
			{
				ID:     "exec-004",
				Action: "get_resource",
				Status: store.StatusDispatched,
			},
		},
	}
	s3Mock := &mockS3{
		objects: map[string]string{
			"executions/exec-004/output.json": `should not appear`,
		},
	}
	h := testHandlerWithS3(execStore, s3Mock)

	rr := doRequest(h, "GET", "/api/v0/trusted-actions/runs/exec-004?include=output", nil, defaultHeaders())

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)

	if output, ok := resp["output"].(string); ok && output != "" {
		t.Errorf("should not embed S3 content for non-terminal execution, got %q", output)
	}
}

func TestHandleGetExecution_WhenTerminal_ItShouldIncludeDuration(t *testing.T) {
	execStore := &mockExecStore{
		executions: []*store.Execution{
			{
				ID:         "exec-005",
				Action:     "get_resource",
				Status:     store.StatusSucceeded,
				DurationMs: 258, // sub-second execution
			},
		},
	}
	h := testHandlerWithS3(execStore, nil)

	rr := doRequest(h, "GET", "/api/v0/trusted-actions/runs/exec-005", nil, defaultHeaders())

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)

	dur, ok := resp["duration_ms"].(float64)
	if !ok {
		t.Fatalf("expected duration_ms as number in response, got %T: %v", resp["duration_ms"], resp["duration_ms"])
	}
	if dur != 258 {
		t.Errorf("expected duration_ms=258, got %v", dur)
	}
}

func TestHandleGetExecution_WhenNotFound_ItShouldReturn404(t *testing.T) {
	h := testHandlerWithS3(&mockExecStore{}, nil)

	rr := doRequest(h, "GET", "/api/v0/trusted-actions/runs/nonexistent", nil, defaultHeaders())

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleCreate_WhenTimeoutExceedsServerMax_ItShouldReturn400(t *testing.T) {
	execStore := &mockExecStore{}
	h := testHandler(execStore)

	body := createRequest{
		Reason:         "JIRA-123",
		Params:         map[string]string{"namespace": "default"},
		TimeoutSeconds: 999,
	}

	rr := doRequest(h, "POST", "/api/v0/trusted-actions/test-read/run", body, defaultHeaders())

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["code"] != "timeout_exceeded" {
		t.Errorf("expected timeout_exceeded code, got %q", resp["code"])
	}
}

func TestHandleCreate_WhenTimeoutWithinLimit_ItShouldSetCustomTimeout(t *testing.T) {
	execStore := &mockExecStore{}
	h := testHandler(execStore)

	body := createRequest{
		Reason:         "JIRA-123",
		Params:         map[string]string{"namespace": "default"},
		TimeoutSeconds: 60,
	}

	rr := doRequest(h, "POST", "/api/v0/trusted-actions/test-read/run", body, defaultHeaders())

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	if len(execStore.created) != 1 {
		t.Fatalf("expected 1 execution created, got %d", len(execStore.created))
	}
	if execStore.created[0].TimeoutSeconds != 60 {
		t.Errorf("expected timeout_seconds=60, got %d", execStore.created[0].TimeoutSeconds)
	}
}

// --- Inline output tests ---

type mockExecutorS3 struct {
	putCalls []s3.PutObjectInput
}

func (m *mockExecutorS3) PutObject(_ context.Context, params *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	m.putCalls = append(m.putCalls, *params)
	return &s3.PutObjectOutput{}, nil
}
func (m *mockExecutorS3) HeadObject(_ context.Context, _ *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	return &s3.HeadObjectOutput{}, nil
}

func testHandlerWithWorkingExecutor(execStore store.ExecutionStore) *Handler {
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
	kubeClient := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	s3Mock := &mockExecutorS3{}
	exec := executor.New(kubeClient, nil, s3Mock, nil, executor.ExecutorConfig{
		ArtifactBucket:  "test-bucket",
		UploaderRoleARN: "arn:aws:iam::123:role/uploader",
		Region:          "us-east-1",
		JobImage:        "test:latest",
	}, logger)
	return New(cfg, execStore, &mockAuditStore{}, exec, nil, logger)
}

func TestHandleCreate_WhenSyncSucceeds_ItShouldReturnOutputInline(t *testing.T) {
	execStore := &mockExecStore{}
	h := testHandlerWithWorkingExecutor(execStore)

	body := createRequest{
		Reason: "JIRA-123",
		Params: map[string]string{"namespace": "default"},
	}

	rr := doRequest(h, "POST", "/api/v0/trusted-actions/test-read/run", body, defaultHeaders())

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)

	if resp["status"] == nil {
		t.Fatal("response missing status field")
	}
	status := resp["status"].(string)

	if resp["execution_mode"] != "sync" {
		t.Errorf("expected execution_mode=sync, got %v", resp["execution_mode"])
	}

	if resp["duration_ms"] == nil {
		t.Error("sync response should include duration_ms inline")
	}

	switch status {
	case "succeeded":
		if resp["output"] == nil {
			t.Error("succeeded sync response should include output inline")
		}
	case "failed":
		if resp["logs"] == nil || resp["logs"] == "" {
			t.Error("failed sync response should include logs inline")
		}
	}
}

func TestHandleCreate_WhenSyncFails_ItShouldReturnLogsInline(t *testing.T) {
	execStore := &mockExecStore{}
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
	s3Mock := &mockExecutorS3{}
	exec := executor.New(nil, nil, s3Mock, nil, executor.ExecutorConfig{
		ArtifactBucket:  "test-bucket",
		UploaderRoleARN: "arn:aws:iam::123:role/uploader",
		Region:          "us-east-1",
		JobImage:        "test:latest",
	}, logger)
	h := New(cfg, execStore, &mockAuditStore{}, exec, nil, logger)

	body := createRequest{
		Reason: "JIRA-456",
		Params: map[string]string{"namespace": "default"},
	}

	rr := doRequest(h, "POST", "/api/v0/trusted-actions/test-read/run", body, defaultHeaders())

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)

	status := resp["status"].(string)
	if status != "failed" {
		t.Skipf("expected failed status (nil K8s client), got %q", status)
	}

	if resp["logs"] == nil || resp["logs"] == "" {
		t.Error("failed sync response should include execution logs inline")
	}
}

func TestHandleCreate_WhenAsync_ItShouldNotIncludeInlineOutput(t *testing.T) {
	execStore := &mockExecStore{}
	h := testHandlerWithWorkingExecutor(execStore)

	body := createRequest{
		Reason:        "JIRA-789",
		Params:        map[string]string{"namespace": "default"},
		ExecutionMode: "async",
	}

	rr := doRequest(h, "POST", "/api/v0/trusted-actions/test-read/run", body, defaultHeaders())

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)

	if resp["output"] != nil {
		t.Error("async response should NOT include inline output")
	}
	if resp["logs"] != nil && resp["logs"] != "" {
		t.Error("async response should NOT include inline logs")
	}
}

func TestHandleGetExecution_WhenTarGzWithoutManifest_ItShouldReturnDownloadHint(t *testing.T) {
	execStore := &mockExecStore{
		executions: []*store.Execution{
			{
				ID:           "exec-mg-002",
				Action:       "must_gather",
				Status:       store.StatusSucceeded,
				OutputFormat: "tar.gz",
				OutputBytes:  99999999,
			},
		},
	}
	h := testHandlerWithS3(execStore, &mockS3{objects: map[string]string{}})

	rr := doRequest(h, "GET", "/api/v0/trusted-actions/runs/exec-mg-002?include=output", nil, defaultHeaders())

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)

	outputStr, ok := resp["output"].(string)
	if !ok || !strings.Contains(outputStr, "zoa download") {
		t.Errorf("expected download hint, got %v", resp["output"])
	}
}

func TestHandleGetExecution_WhenNoS3Client_ItShouldStillReturnMetadata(t *testing.T) {
	execStore := &mockExecStore{
		executions: []*store.Execution{
			{
				ID:     "exec-006",
				Action: "get_resource",
				Status: store.StatusSucceeded,
			},
		},
	}
	h := testHandlerWithS3(execStore, nil)

	rr := doRequest(h, "GET", "/api/v0/trusted-actions/runs/exec-006?include=output", nil, defaultHeaders())

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["id"] != "exec-006" {
		t.Errorf("expected id=exec-006, got %v", resp["id"])
	}
}
