package api

import (
	"context"
	"fmt"
	"testing"

	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/store"
)

func TestExtractSREIdentity_WhenValidAssumedRoleARN_ItShouldReturnUsernameAndRole(t *testing.T) {
	tests := []struct {
		name     string
		arn      string
		wantUser string
		wantRole string
	}{
		{
			name:     "standard SRE ARN",
			arn:      "arn:aws:sts::123456:assumed-role/sre-role/slopezma",
			wantUser: "slopezma",
			wantRole: "sre-role",
		},
		{
			name:     "boundary task ARN",
			arn:      "arn:aws:sts::123456:assumed-role/zoa-boundary-task-role/abc123def",
			wantUser: "abc123def",
			wantRole: "zoa-boundary-task-role",
		},
		{
			name:     "long role path ARN",
			arn:      "arn:aws:sts::123456:assumed-role/some/nested/role/slopezma",
			wantUser: "slopezma",
			wantRole: "role",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			username, role, err := ExtractSREIdentity(tt.arn)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if username != tt.wantUser {
				t.Errorf("expected username %q, got %q", tt.wantUser, username)
			}
			if role != tt.wantRole {
				t.Errorf("expected role %q, got %q", tt.wantRole, role)
			}
		})
	}
}

func TestExtractSREIdentity_WhenInvalidARN_ItShouldReturnError(t *testing.T) {
	tests := []struct {
		name string
		arn  string
	}{
		{name: "empty ARN", arn: ""},
		{name: "not assumed-role", arn: "arn:aws:iam::123456:user/slopezma"},
		{name: "too few parts", arn: "arn:aws:sts::123456:assumed-role/slopezma"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ExtractSREIdentity(tt.arn)
			if err == nil {
				t.Fatal("expected error for invalid ARN")
			}
		})
	}
}

type mockSessionStore struct {
	getFn          func(ctx context.Context, id string) (*store.Session, error)
	getByTaskIDFn  func(ctx context.Context, taskID string) (*store.Session, error)
	putFn          func(ctx context.Context, s *store.Session) error
	listFn         func(ctx context.Context, filter *store.SessionFilter) ([]*store.Session, error)
	updateStatusFn func(ctx context.Context, id string, from, to store.SessionStatus, updates map[string]interface{}) error
	listByOperFn   func(ctx context.Context, operator string, limit int) ([]*store.Session, error)
	listExpiredFn  func(ctx context.Context) ([]*store.Session, error)
}

func (m *mockSessionStore) Put(ctx context.Context, s *store.Session) error {
	if m.putFn != nil {
		return m.putFn(ctx, s)
	}
	return nil
}

func (m *mockSessionStore) Get(ctx context.Context, id string) (*store.Session, error) {
	if m.getFn != nil {
		return m.getFn(ctx, id)
	}
	return nil, nil
}

func (m *mockSessionStore) GetByTaskID(ctx context.Context, taskID string) (*store.Session, error) {
	if m.getByTaskIDFn != nil {
		return m.getByTaskIDFn(ctx, taskID)
	}
	return nil, nil
}

func (m *mockSessionStore) List(ctx context.Context, filter *store.SessionFilter) ([]*store.Session, error) {
	if m.listFn != nil {
		return m.listFn(ctx, filter)
	}
	return nil, nil
}

func (m *mockSessionStore) UpdateStatus(ctx context.Context, id string, from, to store.SessionStatus, updates map[string]interface{}) error {
	if m.updateStatusFn != nil {
		return m.updateStatusFn(ctx, id, from, to, updates)
	}
	return nil
}

func (m *mockSessionStore) ListByOperator(ctx context.Context, operator string, limit int) ([]*store.Session, error) {
	if m.listByOperFn != nil {
		return m.listByOperFn(ctx, operator, limit)
	}
	return nil, nil
}

func (m *mockSessionStore) ListAll(_ context.Context, _ *store.SessionFilter) ([]*store.Session, error) {
	return nil, nil
}

func (m *mockSessionStore) ListExpired(ctx context.Context) ([]*store.Session, error) {
	if m.listExpiredFn != nil {
		return m.listExpiredFn(ctx)
	}
	return nil, nil
}

func (m *mockSessionStore) ListActiveBeforeDeadline(ctx context.Context) ([]*store.Session, error) {
	return nil, nil
}

func (m *mockSessionStore) RecordExecSession(ctx context.Context, sessionID, operator, execSessionID string) error {
	return nil
}

func TestResolveIdentity_WhenDirectSREARN_ItShouldReturnUsernameAndEmptySession(t *testing.T) {
	result, err := ResolveIdentity(
		context.Background(),
		"arn:aws:sts::123456:assumed-role/sre-role/slopezma",
		nil,
		"",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Operator != "slopezma" {
		t.Errorf("expected operator 'slopezma', got %q", result.Operator)
	}
	if result.SessionID != "" {
		t.Errorf("expected empty session ID for direct caller, got %q", result.SessionID)
	}
}

func TestResolveIdentity_WhenBoundaryTaskRole_ItShouldLookupSessionByTaskID(t *testing.T) {
	mockStore := &mockSessionStore{
		getByTaskIDFn: func(_ context.Context, taskID string) (*store.Session, error) {
			if taskID != "task-abc123" {
				return nil, fmt.Errorf("unexpected task ID: %s", taskID)
			}
			return &store.Session{
				SessionID: "sess-xyz",
				TaskID:    "task-abc123",
				Operator:  "slopezma",
			}, nil
		},
	}

	result, err := ResolveIdentity(
		context.Background(),
		"arn:aws:sts::123456:assumed-role/zoa-boundary-task-role/task-abc123",
		mockStore,
		"",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Operator != "slopezma" {
		t.Errorf("expected operator 'slopezma', got %q", result.Operator)
	}
	if result.SessionID != "sess-xyz" {
		t.Errorf("expected session ID 'sess-xyz', got %q", result.SessionID)
	}
}

func TestResolveIdentity_WhenBoundaryTaskRoleNoSession_ItShouldReturnError(t *testing.T) {
	mockStore := &mockSessionStore{
		getByTaskIDFn: func(_ context.Context, _ string) (*store.Session, error) {
			return nil, nil
		},
	}

	_, err := ResolveIdentity(
		context.Background(),
		"arn:aws:sts::123456:assumed-role/zoa-boundary-task-role/task-unknown",
		mockStore,
		"",
	)
	if err == nil {
		t.Fatal("expected error when session not found")
	}
}

func TestResolveIdentity_WhenBoundaryTaskRoleNoStore_ItShouldReturnError(t *testing.T) {
	_, err := ResolveIdentity(
		context.Background(),
		"arn:aws:sts::123456:assumed-role/zoa-boundary-task-role/task-abc",
		nil,
		"",
	)
	if err == nil {
		t.Fatal("expected error when session store is nil for boundary role")
	}
}

func TestResolveIdentity_WhenEmptyARN_ItShouldReturnError(t *testing.T) {
	_, err := ResolveIdentity(context.Background(), "", nil, "")
	if err == nil {
		t.Fatal("expected error for empty ARN")
	}
}

func TestResolveIdentity_WhenCustomBoundaryRolePrefix_ItShouldMatch(t *testing.T) {
	mockStore := &mockSessionStore{
		getByTaskIDFn: func(_ context.Context, _ string) (*store.Session, error) {
			return &store.Session{
				SessionID: "sess-custom",
				Operator:  "testuser",
			}, nil
		},
	}

	result, err := ResolveIdentity(
		context.Background(),
		"arn:aws:sts::123456:assumed-role/custom-boundary-role/task-xyz",
		mockStore,
		"custom-boundary",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Operator != "testuser" {
		t.Errorf("expected operator 'testuser', got %q", result.Operator)
	}
	if result.SessionID != "sess-custom" {
		t.Errorf("expected session ID 'sess-custom', got %q", result.SessionID)
	}
}
