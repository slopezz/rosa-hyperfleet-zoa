package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

type mockExecutionStore struct {
	executions map[string]*Execution
}

func newMockExecutionStore() *mockExecutionStore {
	return &mockExecutionStore{
		executions: make(map[string]*Execution),
	}
}

func (m *mockExecutionStore) Create(_ context.Context, exec *Execution) error {
	if _, exists := m.executions[exec.ID]; exists {
		return fmt.Errorf("execution already exists: %s", exec.ID)
	}
	m.executions[exec.ID] = exec
	return nil
}

func (m *mockExecutionStore) Get(_ context.Context, executionID string) (*Execution, error) {
	exec, ok := m.executions[executionID]
	if !ok {
		return nil, nil
	}
	return exec, nil
}

func (m *mockExecutionStore) List(_ context.Context, accountID string, limit int, filter *ListFilter) ([]*Execution, error) {
	var results []*Execution
	for _, exec := range m.executions {
		if exec.AccountID != accountID {
			continue
		}
		if filter != nil {
			if filter.Status != nil && exec.Status != *filter.Status {
				continue
			}
			if filter.Action != nil && exec.Action != *filter.Action {
				continue
			}
		}
		results = append(results, exec)
		if limit > 0 && len(results) >= limit {
			break
		}
	}
	return results, nil
}

func (m *mockExecutionStore) TransitionStatus(_ context.Context, id string, from, to Status) error {
	exec, ok := m.executions[id]
	if !ok {
		return fmt.Errorf("not found: %s", id)
	}
	if exec.Status != from {
		return fmt.Errorf("conditional check failed: expected %s, got %s", from, exec.Status)
	}
	exec.Status = to
	return nil
}

func (m *mockExecutionStore) TransitionWithMetadata(_ context.Context, id string, from, to Status, updates map[string]interface{}) error {
	exec, ok := m.executions[id]
	if !ok {
		return fmt.Errorf("not found: %s", id)
	}
	if exec.Status != from {
		return fmt.Errorf("conditional check failed: expected %s, got %s", from, exec.Status)
	}
	exec.Status = to
	if v, ok := updates["completedAt"]; ok {
		exec.CompletedAt = v.(string)
	}
	if v, ok := updates["durationMs"]; ok {
		exec.DurationMs = v.(int64)
	}
	return nil
}

func (m *mockExecutionStore) QueryByStatus(_ context.Context, status Status) ([]*Execution, error) {
	var results []*Execution
	for _, exec := range m.executions {
		if exec.Status == status {
			results = append(results, exec)
		}
	}
	return results, nil
}

func (m *mockExecutionStore) QueryByStatusAndClass(_ context.Context, status Status, class string) ([]*Execution, error) {
	var results []*Execution
	for _, exec := range m.executions {
		if exec.Status == status && exec.ExecutionMode == class {
			results = append(results, exec)
		}
	}
	return results, nil
}

func (m *mockExecutionStore) QueryTerminal(_ context.Context, olderThan time.Duration) ([]*Execution, error) {
	threshold := time.Now().Add(-olderThan)
	var results []*Execution
	for _, exec := range m.executions {
		if !exec.Status.IsTerminal() || exec.Cleaned {
			continue
		}
		t, err := exec.CreatedAtTime()
		if err != nil {
			continue
		}
		if t.Before(threshold) {
			results = append(results, exec)
		}
	}
	return results, nil
}

func (m *mockExecutionStore) MarkCleaned(_ context.Context, id string) error {
	exec, ok := m.executions[id]
	if !ok {
		return fmt.Errorf("not found: %s", id)
	}
	if exec.Cleaned {
		return fmt.Errorf("already cleaned: %s", id)
	}
	exec.Cleaned = true
	exec.CleanedAt = time.Now().Format(time.RFC3339Nano)
	return nil
}

func (m *mockExecutionStore) ListByTargetAndAction(_ context.Context, target, action string, since time.Time) ([]*Execution, error) {
	var results []*Execution
	for _, exec := range m.executions {
		if exec.TargetCluster != target || exec.Action != action {
			continue
		}
		createdAt, _ := exec.CreatedAtTime()
		if createdAt.After(since) {
			results = append(results, exec)
		}
	}
	return results, nil
}

func (m *mockExecutionStore) CountActiveByTarget(_ context.Context, target string) (int, error) {
	count := 0
	for _, exec := range m.executions {
		if exec.TargetCluster == target && exec.Status == StatusDispatched {
			count++
		}
	}
	return count, nil
}

func (m *mockExecutionStore) QueryByTargetAndStatus(_ context.Context, target string, status Status) ([]*Execution, error) {
	var results []*Execution
	for _, exec := range m.executions {
		if exec.TargetCluster == target && exec.Status == status {
			results = append(results, exec)
		}
	}
	return results, nil
}

func (m *mockExecutionStore) QueryTerminalByTarget(_ context.Context, target string, olderThan time.Duration) ([]*Execution, error) {
	threshold := time.Now().Add(-olderThan)
	var results []*Execution
	for _, exec := range m.executions {
		if exec.TargetCluster != target || !exec.Status.IsTerminal() || exec.Cleaned {
			continue
		}
		t, err := exec.CreatedAtTime()
		if err != nil {
			continue
		}
		if t.Before(threshold) {
			results = append(results, exec)
		}
	}
	return results, nil
}

func TestMockExecutionStore_Create(t *testing.T) {
	s := newMockExecutionStore()
	ctx := context.Background()

	exec := &Execution{
		ID:        "test-1",
		Action:    "get-pods",
		AccountID: "acct-1",
		Status:    StatusDispatched,
		CreatedAt: time.Now().Format(time.RFC3339Nano),
	}

	if err := s.Create(ctx, exec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := s.Create(ctx, exec); err == nil {
		t.Fatal("expected conflict error on duplicate create")
	}
}

func TestMockExecutionStore_Get(t *testing.T) {
	s := newMockExecutionStore()
	ctx := context.Background()

	exec := &Execution{
		ID:        "test-2",
		Action:    "get-pods",
		AccountID: "acct-1",
		Status:    StatusDispatched,
		CreatedAt: time.Now().Format(time.RFC3339Nano),
	}
	_ = s.Create(ctx, exec)

	got, err := s.Get(ctx, "test-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected execution, got nil")
		return
	}
	if got.ID != "test-2" {
		t.Fatalf("expected ID test-2, got %s", got.ID)
	}

	got, err = s.Get(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil for nonexistent execution")
	}
}

func TestMockExecutionStore_TransitionStatus(t *testing.T) {
	s := newMockExecutionStore()
	ctx := context.Background()

	exec := &Execution{
		ID:        "test-3",
		Action:    "restart-pod",
		AccountID: "acct-1",
		Status:    StatusDispatched,
		CreatedAt: time.Now().Format(time.RFC3339Nano),
	}
	_ = s.Create(ctx, exec)

	if err := s.TransitionStatus(ctx, "test-3", StatusDispatched, StatusSucceeded); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := s.Get(ctx, "test-3")
	if got.Status != StatusSucceeded {
		t.Fatalf("expected status succeeded, got %s", got.Status)
	}

	// Conditional write should fail if status doesn't match
	if err := s.TransitionStatus(ctx, "test-3", StatusDispatched, StatusFailed); err == nil {
		t.Fatal("expected conditional check error")
	}
}

func TestMockExecutionStore_QueryByStatus(t *testing.T) {
	s := newMockExecutionStore()
	ctx := context.Background()

	_ = s.Create(ctx, &Execution{ID: "e1", Status: StatusDispatched, CreatedAt: time.Now().Format(time.RFC3339Nano)})
	_ = s.Create(ctx, &Execution{ID: "e2", Status: StatusSucceeded, CreatedAt: time.Now().Format(time.RFC3339Nano)})
	_ = s.Create(ctx, &Execution{ID: "e3", Status: StatusDispatched, CreatedAt: time.Now().Format(time.RFC3339Nano)})

	dispatched, err := s.QueryByStatus(ctx, StatusDispatched)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dispatched) != 2 {
		t.Fatalf("expected 2 dispatched, got %d", len(dispatched))
	}
}

func TestMockExecutionStore_CountActiveByTarget(t *testing.T) {
	s := newMockExecutionStore()
	ctx := context.Background()

	_ = s.Create(ctx, &Execution{ID: "e1", TargetCluster: "mc-1", Status: StatusDispatched, CreatedAt: time.Now().Format(time.RFC3339Nano)})
	_ = s.Create(ctx, &Execution{ID: "e2", TargetCluster: "mc-1", Status: StatusDispatched, CreatedAt: time.Now().Format(time.RFC3339Nano)})
	_ = s.Create(ctx, &Execution{ID: "e3", TargetCluster: "mc-1", Status: StatusSucceeded, CreatedAt: time.Now().Format(time.RFC3339Nano)})
	_ = s.Create(ctx, &Execution{ID: "e4", TargetCluster: "mc-2", Status: StatusDispatched, CreatedAt: time.Now().Format(time.RFC3339Nano)})

	count, err := s.CountActiveByTarget(ctx, "mc-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 active for mc-1, got %d", count)
	}

	count, err = s.CountActiveByTarget(ctx, "mc-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 active for mc-2, got %d", count)
	}
}

func TestMockExecutionStore_ListByTargetAndAction(t *testing.T) {
	s := newMockExecutionStore()
	ctx := context.Background()

	recent := time.Now().Add(-1 * time.Minute).Format(time.RFC3339Nano)
	old := time.Now().Add(-10 * time.Minute).Format(time.RFC3339Nano)

	_ = s.Create(ctx, &Execution{ID: "e1", TargetCluster: "mc-1", Action: "restart-pod", CreatedAt: recent})
	_ = s.Create(ctx, &Execution{ID: "e2", TargetCluster: "mc-1", Action: "restart-pod", CreatedAt: old})
	_ = s.Create(ctx, &Execution{ID: "e3", TargetCluster: "mc-1", Action: "get-pods", CreatedAt: recent})

	since := time.Now().Add(-5 * time.Minute)
	results, err := s.ListByTargetAndAction(ctx, "mc-1", "restart-pod", since)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != "e1" {
		t.Fatalf("expected e1, got %s", results[0].ID)
	}
}

func TestStatus_IsTerminal(t *testing.T) {
	tests := []struct {
		status   Status
		terminal bool
	}{
		{StatusDispatched, false},
		{StatusApproved, false},
		{StatusPendingApproval, false},
		{StatusSucceeded, true},
		{StatusFailed, true},
		{StatusTimedOut, true},
		{StatusRejected, true},
	}

	for _, tt := range tests {
		if got := tt.status.IsTerminal(); got != tt.terminal {
			t.Errorf("Status(%q).IsTerminal() = %v, want %v", tt.status, got, tt.terminal)
		}
	}
}
