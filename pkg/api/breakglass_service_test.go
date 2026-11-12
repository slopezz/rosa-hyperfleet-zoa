package api

import (
	"context"
	"testing"
	"time"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/store"
)

type mockBreakglassServiceStore struct {
	req *store.BreakglassRequest
}

func (m *mockBreakglassServiceStore) Create(context.Context, *store.BreakglassRequest) error {
	return nil
}
func (m *mockBreakglassServiceStore) Get(context.Context, string) (*store.BreakglassRequest, error) {
	return m.req, nil
}
func (m *mockBreakglassServiceStore) ListByOperator(context.Context, string, *store.BreakglassFilter) ([]*store.BreakglassRequest, error) {
	return nil, nil
}
func (m *mockBreakglassServiceStore) QueryByTargetAndStatus(context.Context, string, store.Status) ([]*store.BreakglassRequest, error) {
	return nil, nil
}
func (m *mockBreakglassServiceStore) MarkCleaned(context.Context, string) error { return nil }

func (m *mockBreakglassServiceStore) Transition(_ context.Context, _ string, from, to store.Status, updates map[string]interface{}) error {
	if m.req.Status != from {
		return context.Canceled
	}
	m.req.Status = to
	for k, v := range updates {
		switch k {
		case "approvedBy":
			m.req.ApprovedBy = v.(string)
		case "decisionNote":
			m.req.DecisionNote = v.(string)
		}
	}
	return nil
}

func TestApproveBreakglassRequest_WhenSelfApprove_ItShouldFail(t *testing.T) {
	req := &store.BreakglassRequest{
		RequestID:        "id-1",
		Scope:            "kube-read",
		Operator:         "alice",
		Status:           store.StatusPendingApproval,
		RequestExpiresAt: time.Now().Add(time.Hour).Format(time.RFC3339Nano),
	}
	store := &mockBreakglassServiceStore{req: req}
	err := ApproveBreakglassRequest(context.Background(), store, req, "alice", "ok")
	if err != errBreakglassSelfApprove {
		t.Fatalf("expected self approval error, got %v", err)
	}
}

func TestApproveBreakglassRequest_WhenStrictScopeRequiresNote_ItShouldFailWithoutNote(t *testing.T) {
	req := &store.BreakglassRequest{
		RequestID:        "id-2",
		Scope:            "kube-elevated",
		Operator:         "alice",
		Status:           store.StatusPendingApproval,
		RequestExpiresAt: time.Now().Add(time.Hour).Format(time.RFC3339Nano),
	}
	store := &mockBreakglassServiceStore{req: req}
	err := ApproveBreakglassRequest(context.Background(), store, req, "bob", "")
	if err != errBreakglassNoteRequired {
		t.Fatalf("expected note required, got %v", err)
	}
}

func TestRejectBreakglassRequest_WhenNoteRequired_ItShouldFailWithoutNote(t *testing.T) {
	req := &store.BreakglassRequest{
		RequestID: "id-3",
		Scope:     "kube-elevated",
		Status:    store.StatusPendingApproval,
	}
	store := &mockBreakglassServiceStore{req: req}
	err := RejectBreakglassRequest(context.Background(), store, req, "bob", "")
	if err != errBreakglassNoteRequired {
		t.Fatalf("expected note required, got %v", err)
	}
}
