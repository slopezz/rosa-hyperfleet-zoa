package api

import (
	"context"
	"fmt"
	"testing"

	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/store"
)

const testECSTaskID = "6e8699e3938a4bcd1234567890abcdef"

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
			arn:      "arn:aws:sts::123456:assumed-role/zoa-boundary-task-role/" + testECSTaskID,
			wantUser: testECSTaskID,
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

func (m *mockSessionStore) UpdateStatus(ctx context.Context, sessionID string, from, to store.SessionStatus, updates map[string]interface{}) error {
	if m.updateStatusFn != nil {
		return m.updateStatusFn(ctx, sessionID, from, to, updates)
	}
	return nil
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
	mockStore := &mockSessionStore{
		getByTaskIDFn: func(_ context.Context, _ string) (*store.Session, error) {
			t.Fatal("should not query task-id index for human session name")
			return nil, nil
		},
	}
	result, err := ResolveIdentity(
		context.Background(),
		"arn:aws:sts::123456:assumed-role/sre-role/slopezma",
		mockStore,
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

func TestResolveIdentity_WhenTaskIDBridgeHit_ItShouldReturnSessionOperator(t *testing.T) {
	mockStore := &mockSessionStore{
		getByTaskIDFn: func(_ context.Context, taskID string) (*store.Session, error) {
			return &store.Session{
				SessionID: "sess-xyz",
				TaskID:    taskID,
				Operator:  "slopezma",
			}, nil
		},
	}

	arn := "arn:aws:sts::123456:assumed-role/eph-046f5f15-regional-zoa-boundary-task/" + testECSTaskID
	result, err := ResolveIdentity(context.Background(), arn, mockStore)
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

func TestResolveIdentity_WhenTaskIDBridgeMiss_ItShouldFallbackToSessionName(t *testing.T) {
	mockStore := &mockSessionStore{
		getByTaskIDFn: func(_ context.Context, _ string) (*store.Session, error) {
			return nil, nil
		},
	}

	result, err := ResolveIdentity(
		context.Background(),
		"arn:aws:sts::123456:assumed-role/zoa-boundary-task-role/"+testECSTaskID,
		mockStore,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Operator != testECSTaskID {
		t.Errorf("expected fallback operator %q, got %q", testECSTaskID, result.Operator)
	}
	if result.SessionID != "" {
		t.Errorf("expected empty session ID on bridge miss, got %q", result.SessionID)
	}
}

func TestResolveIdentity_WhenBridgeLookupErrors_ItShouldReturnError(t *testing.T) {
	mockStore := &mockSessionStore{
		getByTaskIDFn: func(_ context.Context, _ string) (*store.Session, error) {
			return nil, fmt.Errorf("dynamo down")
		},
	}

	_, err := ResolveIdentity(
		context.Background(),
		"arn:aws:sts::123456:assumed-role/zoa-boundary-task-role/"+testECSTaskID,
		mockStore,
	)
	if err == nil {
		t.Fatal("expected error when bridge lookup fails")
	}
}

func TestResolveIdentity_WhenEmptyARN_ItShouldReturnError(t *testing.T) {
	_, err := ResolveIdentity(context.Background(), "", nil)
	if err == nil {
		t.Fatal("expected error for empty ARN")
	}
}

func TestResolveIdentity_WhenNilSessionStoreAndTaskIDARN_ItShouldFallback(t *testing.T) {
	result, err := ResolveIdentity(
		context.Background(),
		"arn:aws:sts::123456:assumed-role/zoa-boundary-task-role/"+testECSTaskID,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Operator != testECSTaskID {
		t.Errorf("expected operator %q, got %q", testECSTaskID, result.Operator)
	}
}

func TestResolveIdentity_WhenHumanSessionName_ItShouldNotCallTaskIDBridge(t *testing.T) {
	called := false
	mockStore := &mockSessionStore{
		getByTaskIDFn: func(_ context.Context, _ string) (*store.Session, error) {
			called = true
			return nil, nil
		},
	}
	result, err := ResolveIdentity(
		context.Background(),
		"arn:aws:sts::123456:assumed-role/sre-role/slopezma",
		mockStore,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Fatal("expected task-id bridge lookup to be skipped for human session name")
	}
	if result.Operator != "slopezma" {
		t.Errorf("expected operator slopezma, got %q", result.Operator)
	}
}

func TestLooksLikeECSTaskID_WhenValidHex32_ItShouldReturnTrue(t *testing.T) {
	if !looksLikeECSTaskID(testECSTaskID) {
		t.Error("expected true for 32-char hex task id")
	}
}

func TestResolveIdentity_TableDriven_ItShouldCoverAttributionPaths(t *testing.T) {
	laptopARN := "arn:aws:sts::123456:assumed-role/sre-role/slopezma"
	taskARN := "arn:aws:sts::123456:assumed-role/zoa-boundary-task-role/" + testECSTaskID

	tests := []struct {
		name     string
		arn      string
		store    store.SessionStore
		wantOp   string
		wantSess string
	}{
		{
			name:     "laptop with store skips bridge",
			arn:      laptopARN,
			store:    &mockSessionStore{},
			wantOp:   "slopezma",
			wantSess: "",
		},
		{
			name: "boundary hit",
			arn:  taskARN,
			store: &mockSessionStore{getByTaskIDFn: func(_ context.Context, _ string) (*store.Session, error) {
				return &store.Session{Operator: "slopezma", SessionID: "s-1"}, nil
			}},
			wantOp:   "slopezma",
			wantSess: "s-1",
		},
		{
			name: "boundary miss",
			arn:  taskARN,
			store: &mockSessionStore{getByTaskIDFn: func(_ context.Context, _ string) (*store.Session, error) {
				return nil, nil
			}},
			wantOp: testECSTaskID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveIdentity(context.Background(), tt.arn, tt.store)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Operator != tt.wantOp || got.SessionID != tt.wantSess {
				t.Fatalf("got operator=%q session=%q", got.Operator, got.SessionID)
			}
		})
	}
}

func TestLooksLikeECSTaskID_WhenInvalid_ItShouldReturnFalse(t *testing.T) {
	tests := []string{
		"",
		"slopezma",
		"task-abc123",
		"6e8699e3938abcd",
		"zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz",
	}
	for _, s := range tests {
		if looksLikeECSTaskID(s) {
			t.Errorf("expected false for %q", s)
		}
	}
}
