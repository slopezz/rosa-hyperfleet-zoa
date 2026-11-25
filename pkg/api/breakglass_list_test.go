package api

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/config"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/executor"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/store"
)

type filterCapturingBreakglassStore struct {
	capturedFilter *store.BreakglassFilter
}

func (m *filterCapturingBreakglassStore) Create(context.Context, *store.BreakglassRequest) error {
	return nil
}
func (m *filterCapturingBreakglassStore) Get(context.Context, string) (*store.BreakglassRequest, error) {
	return nil, nil
}
func (m *filterCapturingBreakglassStore) ListByOperator(context.Context, string, *store.BreakglassFilter) ([]*store.BreakglassRequest, error) {
	return nil, nil
}
func (m *filterCapturingBreakglassStore) ListAll(_ context.Context, filter *store.BreakglassFilter) ([]*store.BreakglassRequest, error) {
	m.capturedFilter = filter
	return nil, nil
}
func (m *filterCapturingBreakglassStore) Transition(context.Context, string, store.Status, store.Status, map[string]interface{}) error {
	return nil
}
func (m *filterCapturingBreakglassStore) QueryByTargetAndStatus(context.Context, string, store.Status) ([]*store.BreakglassRequest, error) {
	return nil, nil
}
func (m *filterCapturingBreakglassStore) MarkCleaned(context.Context, string) error { return nil }
func (m *filterCapturingBreakglassStore) AppendActivateAt(context.Context, string, string) error {
	return nil
}

func testBreakglassHandler(bgStore store.BreakglassStore) *Handler {
	cfg := &config.Config{
		HandlerMode:              "api",
		ArtifactBucket:           "test-bucket",
		TargetCluster:            "eph-regional",
		BreakglassTable:          "test-breakglass",
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
	return New(cfg, &mockExecStore{}, &mockAuditStore{}, exec, nil, logger, WithBreakglassStore(bgStore))
}

func TestHandleListBreakglassRequests_WhenNoTargetQuery_ItShouldNotDefaultToLambdaTargetCluster(t *testing.T) {
	bgStore := &filterCapturingBreakglassStore{}
	h := testBreakglassHandler(bgStore)

	rr := doRequest(h, "GET", "/api/v0/breakglass/requests?scope=mine", nil, defaultHeaders())
	if rr.Code != httpStatusOK(t, rr) {
		return
	}
	f := bgStore.capturedFilter
	if f == nil {
		t.Fatal("expected filter to be passed to store")
	}
	if f.Target != nil {
		t.Errorf("expected no default target filter, got %q", *f.Target)
	}
}

func TestHandleListBreakglassRequests_WhenTargetProvided_ItShouldPassTargetFilter(t *testing.T) {
	bgStore := &filterCapturingBreakglassStore{}
	h := testBreakglassHandler(bgStore)

	rr := doRequest(h, "GET", "/api/v0/breakglass/requests?target=eph-mc01", nil, defaultHeaders())
	if rr.Code != httpStatusOK(t, rr) {
		return
	}
	f := bgStore.capturedFilter
	if f == nil || f.Target == nil || *f.Target != "eph-mc01" {
		t.Errorf("expected target=eph-mc01, got %v", f)
	}
}

func httpStatusOK(t *testing.T, rr *httptest.ResponseRecorder) int {
	t.Helper()
	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	return rr.Code
}