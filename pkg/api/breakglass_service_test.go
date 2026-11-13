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
func (m *mockBreakglassServiceStore) ListAll(context.Context, *store.BreakglassFilter) ([]*store.BreakglassRequest, error) {
	return nil, nil
}
func (m *mockBreakglassServiceStore) QueryByTargetAndStatus(context.Context, string, store.Status) ([]*store.BreakglassRequest, error) {
	return nil, nil
}
func (m *mockBreakglassServiceStore) MarkCleaned(context.Context, string) error { return nil }

func (m *mockBreakglassServiceStore) AppendActivateAt(context.Context, string, string) error {
	return nil
}

func (m *mockBreakglassServiceStore) Transition(_ context.Context, _ string, from, to store.Status, updates map[string]interface{}) error {
	if m.req.Status != from {
		return context.Canceled
	}
	m.req.Status = to
	applyBreakglassUpdates(m.req, updates)
	return nil
}

func applyBreakglassUpdates(req *store.BreakglassRequest, updates map[string]interface{}) {
	for k, v := range updates {
		switch k {
		case "approval":
			req.Approval = v.(string)
		case "grantExpiresAt":
			req.GrantExpiresAt = v.(string)
		case "decidedBy":
			req.DecidedBy = v.(string)
		case "decidedAt":
			req.DecidedAt = v.(string)
		case "expiredAt":
			req.ExpiredAt = v.(string)
		case "decisionNote":
			req.DecisionNote = v.(string)
		}
	}
}

func TestApproveBreakglassRequest_WhenSelfApprove_ItShouldFail(t *testing.T) {
	req := &store.BreakglassRequest{
		RequestID:        "id-1",
		Scope:            "kube-read",
		Operator:         "alice",
		Status:           store.StatusPending,
		RequestExpiresAt: time.Now().Add(time.Hour).Format(time.RFC3339Nano),
	}
	store := &mockBreakglassServiceStore{req: req}
	err := ApproveBreakglassRequest(context.Background(), store, req, "alice", "ok")
	if err != errBreakglassSelfApprove {
		t.Fatalf("expected self approval error, got %v", err)
	}
}

func TestApproveBreakglassRequest_WhenPeerApprove_ItShouldSetDecidedFields(t *testing.T) {
	req := &store.BreakglassRequest{
		RequestID:        "id-2",
		Scope:            "kube-read",
		Operator:         "alice",
		Status:           store.StatusPending,
		Approval:         store.WorkflowApprovalPending,
		RequestExpiresAt: time.Now().Add(time.Hour).Format(time.RFC3339Nano),
	}
	st := &mockBreakglassServiceStore{req: req}
	if err := ApproveBreakglassRequest(context.Background(), st, req, "bob", "ok"); err != nil {
		t.Fatal(err)
	}
	if st.req.Approval != store.WorkflowApprovalApproved {
		t.Fatalf("approval %q", st.req.Approval)
	}
	if st.req.DecidedBy != "bob" || st.req.DecidedAt == "" {
		t.Fatalf("decided: by=%q at=%q", st.req.DecidedBy, st.req.DecidedAt)
	}
}
