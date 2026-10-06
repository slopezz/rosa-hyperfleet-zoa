package cli

import (
	"context"
	"testing"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/client"
	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/output"
)

func TestListActions_WhenActionsExist_ItShouldReturnNilError(t *testing.T) {
	mock := &mockClient{
		listActionsFn: func(_ context.Context) (*client.ActionList, error) {
			return &client.ActionList{
				Items: []client.Action{
					{Name: "get_pods", Scope: "kube-api", Type: "read", Description: "List pods"},
					{Name: "rollout_restart", Scope: "kube-api", Type: "write", Description: "Restart deployment"},
				},
			}, nil
		},
	}

	global := newMockGlobalOpts(mock)
	err := listActions(context.Background(), global, &catalogOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListActions_WhenJSONFormat_ItShouldReturnNilError(t *testing.T) {
	mock := &mockClient{
		listActionsFn: func(_ context.Context) (*client.ActionList, error) {
			return &client.ActionList{
				Items: []client.Action{
					{Name: "get_pods", Scope: "kube-api", Type: "read"},
				},
			}, nil
		},
	}

	global := newMockGlobalOpts(mock)
	global.OutputFormat = output.FormatJSON
	err := listActions(context.Background(), global, &catalogOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListActions_WhenClientReturnsError_ItShouldPropagateError(t *testing.T) {
	mock := &mockClient{
		listActionsFn: func(_ context.Context) (*client.ActionList, error) {
			return nil, &client.APIError{Code: "forbidden", Reason: "access denied"}
		},
	}

	global := newMockGlobalOpts(mock)
	err := listActions(context.Background(), global, &catalogOptions{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDescribeAction_WhenActionExists_ItShouldReturnNilError(t *testing.T) {
	mock := &mockClient{
		getActionFn: func(_ context.Context, name string) (*client.Action, error) {
			if name != "get_pods" {
				t.Errorf("expected name 'get_pods', got %q", name)
			}
			return &client.Action{
				Name:          "get_pods",
				Scope:         "kube-api",
				Type:          "read",
				Description:   "List pods in a namespace",
				ExecutionMode: "sync",
				Params: []client.ActionParam{
					{Name: "namespace", Required: true, Description: "Target namespace"},
					{Name: "label_selector", Required: false, Description: "Label filter"},
				},
			}, nil
		},
	}

	global := newMockGlobalOpts(mock)
	err := describeAction(context.Background(), global, &catalogOptions{}, "get_pods")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDescribeAction_WhenActionHasCooldown_ItShouldReturnNilError(t *testing.T) {
	mock := &mockClient{
		getActionFn: func(_ context.Context, _ string) (*client.Action, error) {
			return &client.Action{
				Name:                 "delete_pod",
				Scope:                "kube-api",
				Type:                 "write",
				Description:          "Delete a specific pod",
				WriteCooldownSeconds: 60,
				TimeoutSeconds:       30,
				DryRunAction:         "get_pod",
				Authorization: client.ActionAuthorization{
					Approval: "peer",
				},
			}, nil
		},
	}

	global := newMockGlobalOpts(mock)
	err := describeAction(context.Background(), global, &catalogOptions{}, "delete_pod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDescribeAction_WhenClientReturnsError_ItShouldPropagateError(t *testing.T) {
	mock := &mockClient{
		getActionFn: func(_ context.Context, _ string) (*client.Action, error) {
			return nil, &client.APIError{Code: "not_found", Reason: "action not found"}
		},
	}

	global := newMockGlobalOpts(mock)
	err := describeAction(context.Background(), global, &catalogOptions{}, "nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
