package scheduler

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/store"
)

type mockSessionStoreReaper struct {
	expired  []*store.Session
	active   []*store.Session
	listErr  error
	updated  map[string]store.SessionStatus
	updateFn func(id string, from, to store.SessionStatus) error
}

func (m *mockSessionStoreReaper) Put(_ context.Context, _ *store.Session) error { return nil }
func (m *mockSessionStoreReaper) Get(_ context.Context, _ string) (*store.Session, error) {
	return nil, nil
}
func (m *mockSessionStoreReaper) GetByTaskID(_ context.Context, _ string) (*store.Session, error) {
	return nil, nil
}
func (m *mockSessionStoreReaper) List(_ context.Context, _ *store.SessionFilter) ([]*store.Session, error) {
	return nil, nil
}
func (m *mockSessionStoreReaper) UpdateStatus(_ context.Context, id string, from, to store.SessionStatus, _ map[string]interface{}) error {
	if m.updateFn != nil {
		return m.updateFn(id, from, to)
	}
	if m.updated == nil {
		m.updated = make(map[string]store.SessionStatus)
	}
	m.updated[id] = to
	return nil
}
func (m *mockSessionStoreReaper) ListAll(_ context.Context, _ *store.SessionFilter) ([]*store.Session, error) {
	return nil, nil
}
func (m *mockSessionStoreReaper) ListExpired(_ context.Context) ([]*store.Session, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.expired, nil
}
func (m *mockSessionStoreReaper) ListActiveBeforeDeadline(_ context.Context) ([]*store.Session, error) {
	return m.active, nil
}
func (m *mockSessionStoreReaper) RecordExecSession(_ context.Context, _ string, _ string, _ string) error {
	return nil
}

type mockECSReaper struct {
	stopped []string
	stopErr error
}

func (m *mockECSReaper) StopTask(_ context.Context, _, taskArn, _ string) error {
	if m.stopErr != nil {
		return m.stopErr
	}
	m.stopped = append(m.stopped, taskArn)
	return nil
}

func testReaper(ss store.SessionStore, ecs ECSAPI) *Reaper {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewReaper(ss, ecs, logger, "")
}

func TestReaper_Run_WhenNoExpiredSessions_ItShouldReturnNil(t *testing.T) {
	ss := &mockSessionStoreReaper{}
	r := testReaper(ss, nil)

	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReaper_Run_WhenExpiredSessions_ItShouldTerminateThem(t *testing.T) {
	ss := &mockSessionStoreReaper{
		expired: []*store.Session{
			{
				SessionID:  "s1",
				Operator:   "sre1",
				Status:     store.SessionStatusActive,
				EcsCluster: "arn:aws:ecs:us-east-1:123:cluster/test",
				TaskArn:    "arn:aws:ecs:us-east-1:123:task/test/abc",
				CreatedAt:  "2026-09-29T01:00:00Z",
			},
		},
	}
	ecs := &mockECSReaper{}
	r := testReaper(ss, ecs)

	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ecs.stopped) != 1 || ecs.stopped[0] != "arn:aws:ecs:us-east-1:123:task/test/abc" {
		t.Errorf("expected 1 ECS task stopped, got %v", ecs.stopped)
	}
	if ss.updated["s1"] != store.SessionStatusTerminated {
		t.Errorf("expected session s1 terminated, got %v", ss.updated["s1"])
	}
}

func TestReaper_Run_WhenListExpiredFails_ItShouldReturnError(t *testing.T) {
	ss := &mockSessionStoreReaper{listErr: fmt.Errorf("dynamodb error")}
	r := testReaper(ss, nil)

	err := r.Run(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestReaper_Run_WhenStopTaskFails_ItShouldContinueAndReturnError(t *testing.T) {
	ss := &mockSessionStoreReaper{
		expired: []*store.Session{
			{
				SessionID:  "s1",
				Status:     store.SessionStatusActive,
				EcsCluster: "cluster",
				TaskArn:    "task-1",
			},
			{
				SessionID:  "s2",
				Status:     store.SessionStatusActive,
				EcsCluster: "cluster",
				TaskArn:    "task-2",
			},
		},
	}
	ecs := &mockECSReaper{stopErr: fmt.Errorf("ecs unavailable")}
	r := testReaper(ss, ecs)

	err := r.Run(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestReaper_Run_WhenTargetClusterSet_ItShouldSkipOtherClusters(t *testing.T) {
	ss := &mockSessionStoreReaper{
		expired: []*store.Session{
			{
				SessionID:     "s-mc",
				TargetCluster: "mc01",
				Status:        store.SessionStatusActive,
				EcsCluster:    "cluster",
				TaskArn:       "task-mc",
			},
			{
				SessionID:     "s-rc",
				TargetCluster: "eph-test-rc",
				Status:        store.SessionStatusActive,
				EcsCluster:    "cluster",
				TaskArn:       "task-rc",
			},
		},
	}
	ecs := &mockECSReaper{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := NewReaper(ss, ecs, logger, "eph-test-rc")

	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ecs.stopped) != 1 || ecs.stopped[0] != "task-rc" {
		t.Errorf("expected only rc task stopped, got %v", ecs.stopped)
	}
}

func TestReaper_Run_WhenTaskNeverJoinedAndNoExecAtAWS_ItShouldTerminateIdle(t *testing.T) {
	ss := &mockSessionStoreReaper{
		active: []*store.Session{
			{
				SessionID:     "s-never-join",
				TargetCluster: "mc01",
				Status:        store.SessionStatusActive,
				TaskID:        "task-orphan",
				TaskArn:       "arn:aws:ecs:us-east-1:123:task/c/task-orphan",
				EcsCluster:    "cluster",
				CreatedAt:     time.Now().Add(-3 * time.Hour).Format(time.RFC3339Nano),
			},
		},
	}
	ecs := &mockECSReaper{}
	activity := &mockExecActivity{found: false}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := NewReaper(ss, ecs, logger, "mc01", WithExecActivity(activity, time.Hour))

	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ecs.stopped) != 1 {
		t.Fatalf("expected ECS stop for unused task, got %v", ecs.stopped)
	}
}

func TestReaper_Run_WhenIdleSession_ItShouldTerminateWithIdleReason(t *testing.T) {
	oldActivity := time.Now().Add(-2 * time.Hour)
	ss := &mockSessionStoreReaper{
		active: []*store.Session{
			{
				SessionID:      "s-idle",
				TargetCluster:  "mc01",
				Status:         store.SessionStatusActive,
				TaskID:         "task-1",
				TaskArn:        "arn:aws:ecs:us-east-1:123:task/c/task-1",
				EcsCluster:     "cluster",
				ExecSessionIDs: []string{"ecs-execute-command-k7zkjuilu2vhsrp76e48ie3ciy"},
			},
		},
	}
	ecs := &mockECSReaper{}
	activity := &mockExecActivity{last: oldActivity, found: true}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := NewReaper(ss, ecs, logger, "mc01", WithExecActivity(activity, time.Hour))

	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ecs.stopped) != 1 {
		t.Fatalf("expected ECS stop, got %v", ecs.stopped)
	}
	if ss.updated["s-idle"] != store.SessionStatusTerminated {
		t.Errorf("expected terminated, got %v", ss.updated)
	}
}

type mockExecActivity struct {
	last  time.Time
	found bool
	err   error
}

func (m *mockExecActivity) LastTerminalActivity(_ context.Context, _ string, _ string, _ []string) (time.Time, bool, error) {
	if m.err != nil {
		return time.Time{}, false, m.err
	}
	return m.last, m.found, nil
}

func TestReaper_Run_WhenNoEcsTask_ItShouldStillUpdateStatus(t *testing.T) {
	ss := &mockSessionStoreReaper{
		expired: []*store.Session{
			{
				SessionID: "s-no-task",
				Operator:  "sre1",
				Status:    store.SessionStatusActive,
			},
		},
	}
	r := testReaper(ss, nil)

	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ss.updated["s-no-task"] != store.SessionStatusTerminated {
		t.Errorf("expected session terminated, got %v", ss.updated["s-no-task"])
	}
}
