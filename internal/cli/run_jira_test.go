package cli

import (
	"context"
	"testing"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/cli/jira"
	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/client"
)

func TestRunAction_WhenZOAJIRAEnvSet_ItShouldDispatchEnvJira(t *testing.T) {
	t.Setenv(jira.EnvVar, "ROSAENG-555")

	var dispatchedJira string
	mock := &mockClient{
		dispatchFn: func(_ context.Context, _ string, req *client.DispatchRequest) (*client.DispatchResponse, error) {
			dispatchedJira = req.Jira
			return &client.DispatchResponse{
				ID:            "exec-1",
				Status:        "succeeded",
				TargetCluster: "mc-1",
				ExecutionMode: "sync",
			}, nil
		},
		getExecutionFn: func(_ context.Context, id string, _ string) (*client.Execution, error) {
			return &client.Execution{ID: id, Status: "succeeded", ExecutionMode: "sync"}, nil
		},
	}

	global := newMockGlobalOpts(mock)
	opts := &runOptions{jira: ""}
	if err := runAction(context.Background(), global, opts, "get_resource"); err != nil {
		t.Fatalf("runAction: %v", err)
	}
	if dispatchedJira != "ROSAENG-555" {
		t.Fatalf("expected ZOA_JIRA on dispatch, got %q", dispatchedJira)
	}
}

func TestRunAction_WhenZOAJIRAAndFlagSet_ItShouldPreferFlagJira(t *testing.T) {
	t.Setenv(jira.EnvVar, "ROSAENG-555")

	var dispatchedJira string
	mock := &mockClient{
		dispatchFn: func(_ context.Context, _ string, req *client.DispatchRequest) (*client.DispatchResponse, error) {
			dispatchedJira = req.Jira
			return &client.DispatchResponse{
				ID:            "exec-2",
				Status:        "succeeded",
				TargetCluster: "mc-1",
				ExecutionMode: "sync",
			}, nil
		},
		getExecutionFn: func(_ context.Context, id string, _ string) (*client.Execution, error) {
			return &client.Execution{ID: id, Status: "succeeded", ExecutionMode: "sync"}, nil
		},
	}

	global := newMockGlobalOpts(mock)
	opts := &runOptions{jira: "ROSAENG-888"}
	if err := runAction(context.Background(), global, opts, "get_resource"); err != nil {
		t.Fatalf("runAction: %v", err)
	}
	if dispatchedJira != "ROSAENG-888" {
		t.Fatalf("--jira must win over ZOA_JIRA; dispatch jira was %q", dispatchedJira)
	}
}

func TestRunAction_WhenNoJiraFlagOrEnv_ItShouldFailBeforeDispatch(t *testing.T) {
	t.Setenv(jira.EnvVar, "")

	mock := &mockClient{
		dispatchFn: func(_ context.Context, _ string, _ *client.DispatchRequest) (*client.DispatchResponse, error) {
			t.Fatal("dispatch should not be called without jira")
			return nil, nil
		},
	}

	global := newMockGlobalOpts(mock)
	opts := &runOptions{jira: ""}
	if err := runAction(context.Background(), global, opts, "get_resource"); err == nil {
		t.Fatal("expected missing jira error")
	}
}
