package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/breakglass"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/config"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/executor"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/store"
)

const testActivateTarget = "eph-mc01"

type mockBreakglassActivateStore struct {
	mockBreakglassServiceStore
	appendEvents []store.ActivateEvent
}

func (m *mockBreakglassActivateStore) AppendActivateEvent(_ context.Context, _ string, ev store.ActivateEvent) error {
	m.appendEvents = append(m.appendEvents, ev)
	return nil
}

func approvedActiveGrant(grantID, profile, operator string, grantTTL time.Duration) *store.BreakglassRequest {
	return &store.BreakglassRequest{
		RequestID:      grantID,
		Profile:        profile,
		TargetCluster:  testActivateTarget,
		Operator:       operator,
		Status:         store.StatusActive,
		Approval:       store.WorkflowApprovalApproved,
		GrantExpiresAt: time.Now().Add(grantTTL).Format(time.RFC3339Nano),
	}
}

func newBreakglassActivateHandler(bg store.BreakglassStore, exec *executor.Executor) *Handler {
	cfg := &config.Config{
		HandlerMode:            "api",
		TargetCluster:          testActivateTarget,
		WriteCooldownSeconds:   300,
		MaxConcurrentPerTarget: 5,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(cfg, &mockExecStore{}, &mockSessionStore{}, bg, &mockAuditStore{}, exec, nil, logger)
}

func postActivate(h *Handler, grantID string, headers map[string]string) *httptest.ResponseRecorder {
	path := PathBreakglassRequests + "/" + grantID + "/activate"
	return doRequest(h, http.MethodPost, path, nil, headers)
}

func errorCode(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	return body["code"]
}

func TestHandleActivateBreakglassRequest_WhenGrantTargetDiffersFromDeployment_ItShouldReturn403(t *testing.T) {
	grantID := "411e01c6-125c-439a-9e2f-df47b3bc1448"
	bgStore := &mockBreakglassServiceStore{req: &store.BreakglassRequest{
		RequestID:      grantID,
		Profile:        "aws_elevated",
		TargetCluster:  "eph-regional",
		Status:         store.StatusActive,
		Approval:       store.WorkflowApprovalApproved,
		GrantExpiresAt: time.Now().Add(time.Hour).Format(time.RFC3339Nano),
	}}
	cfg := &config.Config{
		HandlerMode:    "api",
		TargetCluster:  "eph-mc01",
		WriteCooldownSeconds: 300,
		MaxConcurrentPerTarget: 5,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := New(cfg, &mockExecStore{}, &mockSessionStore{}, bgStore, &mockAuditStore{}, nil, nil, logger)

	path := PathBreakglassRequests + "/" + grantID + "/activate"
	rr := doRequest(h, http.MethodPost, path, nil, defaultHeaders())
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", rr.Code, rr.Body.String())
	}
	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != "target_mismatch" {
		t.Fatalf("code = %q, want target_mismatch", body["code"])
	}
}

func TestHandleActivateBreakglassRequest_WhenGrantTargetMatchesDeployment_ItShouldNotReturnTargetMismatch(t *testing.T) {
	grantID := "grant-1"
	bgStore := &mockBreakglassServiceStore{req: &store.BreakglassRequest{
		RequestID:      grantID,
		Profile:        "unknown_scope",
		TargetCluster:  "eph-mc01",
		Status:         store.StatusActive,
		Approval:       store.WorkflowApprovalApproved,
		GrantExpiresAt: time.Now().Add(time.Hour).Format(time.RFC3339Nano),
	}}
	cfg := &config.Config{
		HandlerMode:            "api",
		TargetCluster:          "eph-mc01",
		WriteCooldownSeconds:   300,
		MaxConcurrentPerTarget: 5,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	exec := executor.New(nil, nil, nil, nil, executor.ExecutorConfig{Region: "us-east-1"}, logger)
	h := New(cfg, &mockExecStore{}, &mockSessionStore{}, bgStore, &mockAuditStore{}, exec, nil, logger)

	path := PathBreakglassRequests + "/" + grantID + "/activate"
	rr := doRequest(h, http.MethodPost, path, nil, defaultHeaders())
	if rr.Code == http.StatusForbidden {
		var body map[string]string
		_ = json.NewDecoder(rr.Body).Decode(&body)
		if body["code"] == "target_mismatch" {
			t.Fatal("unexpected target_mismatch when grant matches deployment")
		}
	}
}

func TestWriteBreakglassGrantTooShort_WhenRemainingBelowKubeMin_ItShouldReturn409(t *testing.T) {
	w := httptest.NewRecorder()
	err := &breakglass.GrantTooShortError{
		Scope:         breakglass.ScopeKube,
		Remaining:     2 * time.Minute,
		MinCredential: 10 * time.Minute,
	}
	if !writeBreakglassGrantTooShort(w, err) {
		t.Fatal("expected grant too short handler")
	}
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", w.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != "grant_too_short" {
		t.Fatalf("code = %q", body["code"])
	}
	if body["reason"] == "" {
		t.Fatal("expected non-empty reason")
	}
}

func TestWriteBreakglassGrantTooShort_WhenUnrelatedError_ItShouldReturnFalse(t *testing.T) {
	w := httptest.NewRecorder()
	if writeBreakglassGrantTooShort(w, breakglass.ErrGrantTooShortForCredential) {
		t.Fatal("expected false without GrantTooShortError")
	}
}

func TestHandleActivateBreakglassRequest_WhenRequestMissing_ItShouldReturn404(t *testing.T) {
	h := newBreakglassActivateHandler(&mockBreakglassServiceStore{req: nil}, nil)
	rr := postActivate(h, "missing-id", defaultHeaders())
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
	if got := errorCode(t, rr); got != "not_found" {
		t.Fatalf("code = %q, want not_found", got)
	}
}

func TestHandleActivateBreakglassRequest_WhenGrantNotActive_ItShouldReturn409(t *testing.T) {
	req := approvedActiveGrant("g-1", "kube_read", "slopezma", time.Hour)
	req.Status = store.StatusPending
	h := newBreakglassActivateHandler(&mockBreakglassServiceStore{req: req}, nil)
	rr := postActivate(h, req.RequestID, defaultHeaders())
	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rr.Code)
	}
	if got := errorCode(t, rr); got != "grant_not_active" {
		t.Fatalf("code = %q, want grant_not_active", got)
	}
}

func TestHandleActivateBreakglassRequest_WhenNotApproved_ItShouldReturn409(t *testing.T) {
	req := approvedActiveGrant("g-2", "kube_read", "slopezma", time.Hour)
	req.Approval = store.WorkflowApprovalPending
	h := newBreakglassActivateHandler(&mockBreakglassServiceStore{req: req}, nil)
	rr := postActivate(h, req.RequestID, defaultHeaders())
	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rr.Code)
	}
	if got := errorCode(t, rr); got != "not_approved" {
		t.Fatalf("code = %q, want not_approved", got)
	}
}

func TestHandleActivateBreakglassRequest_WhenOperatorDiffers_ItShouldReturn403(t *testing.T) {
	req := approvedActiveGrant("g-3", "kube_read", "alice", time.Hour)
	h := newBreakglassActivateHandler(&mockBreakglassServiceStore{req: req}, nil)
	rr := postActivate(h, req.RequestID, defaultHeaders())
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rr.Code)
	}
	if got := errorCode(t, rr); got != "operator_mismatch" {
		t.Fatalf("code = %q, want operator_mismatch", got)
	}
}

func TestHandleActivateBreakglassRequest_WhenGrantExpired_ItShouldReturn410(t *testing.T) {
	req := approvedActiveGrant("g-4", "kube_read", "slopezma", time.Hour)
	req.GrantExpiresAt = time.Now().Add(-time.Minute).Format(time.RFC3339Nano)
	h := newBreakglassActivateHandler(&mockBreakglassServiceStore{req: req}, nil)
	rr := postActivate(h, req.RequestID, defaultHeaders())
	if rr.Code != http.StatusGone {
		t.Fatalf("status = %d, want 410", rr.Code)
	}
	if got := errorCode(t, rr); got != "grant_expired" {
		t.Fatalf("code = %q, want grant_expired", got)
	}
}

func TestHandleActivateBreakglassRequest_WhenGrantRemainingTooShort_ItShouldReturn409(t *testing.T) {
	req := approvedActiveGrant("g-5", "kube_read", "slopezma", 2*time.Minute)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	exec := executor.New(nil, nil, nil, nil, executor.ExecutorConfig{Region: "us-east-1"}, logger)
	h := newBreakglassActivateHandler(&mockBreakglassServiceStore{req: req}, exec)
	rr := postActivate(h, req.RequestID, defaultHeaders())
	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", rr.Code, rr.Body.String())
	}
	if got := errorCode(t, rr); got != "grant_too_short" {
		t.Fatalf("code = %q, want grant_too_short", got)
	}
}

func TestHandleActivateBreakglassRequest_WhenGrantValidAndUnknownProfile_ItShouldReturn200Ready(t *testing.T) {
	grantID := "g-6"
	bgStore := &mockBreakglassActivateStore{mockBreakglassServiceStore: mockBreakglassServiceStore{
		req: approvedActiveGrant(grantID, "not_in_catalog", "slopezma", time.Hour),
	}}
	h := newBreakglassActivateHandler(bgStore, nil)
	rr := postActivate(h, grantID, defaultHeaders())
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "ready" || resp["kind"] != "breakglass" {
		t.Fatalf("unexpected body: %+v", resp)
	}
	if len(bgStore.appendEvents) != 1 {
		t.Fatalf("expected one activate event, got %d", len(bgStore.appendEvents))
	}
}

func TestHandleActivateBreakglassRequest_WhenKubeTokenProvisioningFails_ItShouldReturn500(t *testing.T) {
	grantID := "g-7"
	bgStore := &mockBreakglassActivateStore{mockBreakglassServiceStore: mockBreakglassServiceStore{
		req: approvedActiveGrant(grantID, "kube_read", "slopezma", time.Hour),
	}}
	kubeClient := fake.NewSimpleClientset() //nolint:staticcheck // CreateToken unsupported on fake
	_, _ = kubeClient.CoreV1().Namespaces().Create(context.Background(),
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "zoa-jobs"}}, metav1.CreateOptions{})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	exec := executor.New(kubeClient, &rest.Config{Host: "https://eks.example"},
		nil, nil, executor.ExecutorConfig{Region: "us-east-1", TargetType: "mc"}, logger)
	h := newBreakglassActivateHandler(bgStore, exec)
	rr := postActivate(h, grantID, defaultHeaders())
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500: %s", rr.Code, rr.Body.String())
	}
	if got := errorCode(t, rr); got != "provision_failed" {
		t.Fatalf("code = %q, want provision_failed", got)
	}
}
