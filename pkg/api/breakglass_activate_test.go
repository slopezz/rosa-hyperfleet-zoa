package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/breakglass"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/config"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/executor"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/store"
)

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
