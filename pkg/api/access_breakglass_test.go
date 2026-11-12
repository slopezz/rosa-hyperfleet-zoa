package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/store"
)

type mockBreakglassAccessStore struct {
	req *store.BreakglassRequest
}

func (m *mockBreakglassAccessStore) Create(context.Context, *store.BreakglassRequest) error {
	return nil
}
func (m *mockBreakglassAccessStore) Get(_ context.Context, id string) (*store.BreakglassRequest, error) {
	if m.req != nil && m.req.RequestID == id {
		return m.req, nil
	}
	return nil, nil
}
func (m *mockBreakglassAccessStore) ListByOperator(context.Context, string, *store.BreakglassFilter) ([]*store.BreakglassRequest, error) {
	return nil, nil
}
func (m *mockBreakglassAccessStore) Transition(_ context.Context, _ string, from, to store.Status, updates map[string]interface{}) error {
	if m.req == nil || m.req.Status != from {
		return context.Canceled
	}
	m.req.Status = to
	if v, ok := updates["approvedBy"]; ok {
		m.req.ApprovedBy = v.(string)
	}
	return nil
}
func (m *mockBreakglassAccessStore) QueryByTargetAndStatus(context.Context, string, store.Status) ([]*store.BreakglassRequest, error) {
	return nil, nil
}
func (m *mockBreakglassAccessStore) MarkCleaned(context.Context, string) error { return nil }

func TestAccessHandler_WhenApproveCompoundID_ItShouldResolveRawRequest(t *testing.T) {
	bg := &store.BreakglassRequest{
		RequestID:        "550e8400-e29b-41d4-a716-446655440000",
		Scope:            "kube-read",
		Operator:         "requester",
		Status:           store.StatusPendingApproval,
		RequestExpiresAt: time.Now().Add(time.Hour).Format(time.RFC3339Nano),
	}
	h := testAccessHandler(&mockSessionStoreAccess{}, &mockTargetStoreAccess{})
	h.breakglassStore = &mockBreakglassAccessStore{req: bg}
	h.cfg.DeploymentName = "eph-rc"

	rec := doAccessRequest(h, http.MethodPost, "/api/v0/approve/eph-rc%2F550e8400-e29b-41d4-a716-446655440000", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if bg.Status != store.StatusApproved {
		t.Fatalf("expected approved status, got %s", bg.Status)
	}
}
