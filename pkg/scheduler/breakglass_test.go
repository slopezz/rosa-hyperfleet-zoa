package scheduler

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/store"
)

type mockBreakglassStore struct {
	items []*store.BreakglassRequest
}

func (m *mockBreakglassStore) Create(_ context.Context, _ *store.BreakglassRequest) error { return nil }
func (m *mockBreakglassStore) Get(_ context.Context, _ string) (*store.BreakglassRequest, error) {
	return nil, nil
}
func (m *mockBreakglassStore) ListByOperator(_ context.Context, _ string, _ *store.BreakglassFilter) ([]*store.BreakglassRequest, error) {
	return nil, nil
}
func (m *mockBreakglassStore) MarkCleaned(_ context.Context, _ string) error { return nil }

func (m *mockBreakglassStore) QueryByTargetAndStatus(_ context.Context, target string, status store.Status) ([]*store.BreakglassRequest, error) {
	var out []*store.BreakglassRequest
	for _, item := range m.items {
		if item.TargetCluster == target && item.Status == status {
			out = append(out, item)
		}
	}
	return out, nil
}

func (m *mockBreakglassStore) Transition(_ context.Context, requestID string, from, to store.Status, _ map[string]interface{}) error {
	for _, item := range m.items {
		if item.RequestID == requestID && item.Status == from {
			item.Status = to
			return nil
		}
	}
	return nil
}

func TestBreakglassReconciler_WhenPendingExpired_ItShouldMarkExpired(t *testing.T) {
	past := time.Now().Add(-time.Hour).Format(time.RFC3339Nano)
	mock := &mockBreakglassStore{
		items: []*store.BreakglassRequest{
			{
				RequestID:        "req-1",
				TargetCluster:    "mc01",
				Status:           store.StatusPendingApproval,
				RequestExpiresAt: past,
			},
		},
	}
	r := NewBreakglassReconciler(mock, nil, "mc01", slog.Default())
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.items[0].Status != store.StatusExpired {
		t.Fatalf("expected expired, got %s", mock.items[0].Status)
	}
}
