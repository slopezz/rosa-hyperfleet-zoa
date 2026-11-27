package cli

import (
	"context"
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/cli/reason"
	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
)

func TestRunAction_WhenZOAReasonEnvSet_ItShouldDispatchEnvReason(t *testing.T) {
	t.Setenv(reason.EnvVar, "TEST-12345")

	var dispatchedReason string
	mock := &mockClient{
		dispatchFn: func(_ context.Context, _ string, req *client.DispatchRequest) (*client.DispatchResponse, error) {
			dispatchedReason = req.Reason
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
	opts := &runOptions{reason: ""}
	if err := runAction(context.Background(), global, opts, "get_resource"); err != nil {
		t.Fatalf("runAction: %v", err)
	}
	if dispatchedReason != "TEST-12345" {
		t.Fatalf("expected ZOA_REASON on dispatch, got %q", dispatchedReason)
	}
}

func TestRunAction_WhenZOAReasonAndFlagSet_ItShouldPreferFlagReason(t *testing.T) {
	t.Setenv(reason.EnvVar, "TEST-99999")

	var dispatchedReason string
	mock := &mockClient{
		dispatchFn: func(_ context.Context, _ string, req *client.DispatchRequest) (*client.DispatchResponse, error) {
			dispatchedReason = req.Reason
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
	opts := &runOptions{reason: "TEST-12345"}
	if err := runAction(context.Background(), global, opts, "get_resource"); err != nil {
		t.Fatalf("runAction: %v", err)
	}
	if dispatchedReason != "TEST-12345" {
		t.Fatalf("--reason must win over ZOA_REASON; dispatch reason was %q", dispatchedReason)
	}
}

func TestRunAction_WhenNoReasonFlagOrEnv_ItShouldFailBeforeDispatch(t *testing.T) {
	t.Setenv(reason.EnvVar, "")

	mock := &mockClient{
		dispatchFn: func(_ context.Context, _ string, _ *client.DispatchRequest) (*client.DispatchResponse, error) {
			t.Fatal("dispatch should not be called without reason")
			return nil, nil
		},
	}

	global := newMockGlobalOpts(mock)
	opts := &runOptions{reason: ""}
	if err := runAction(context.Background(), global, opts, "get_resource"); err == nil {
		t.Fatal("expected missing reason error")
	}
}
