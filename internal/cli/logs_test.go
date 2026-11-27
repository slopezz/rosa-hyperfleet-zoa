package cli

import (
	"context"
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
)

func TestShowLogs_WhenLogsAvailable_ItShouldReturnNilError(t *testing.T) {
	mock := &mockClient{
		getExecutionFn: func(_ context.Context, id string, include string) (*client.Execution, error) {
			if id != "exec-123" {
				t.Errorf("expected id 'exec-123', got %q", id)
			}
			if include != "logs" {
				t.Errorf("expected include 'logs', got %q", include)
			}
			return &client.Execution{
				ID:     "exec-123",
				Status: "succeeded",
				Logs:   "2024-01-01T00:00:00Z INFO starting action\n",
			}, nil
		},
	}

	global := newMockGlobalOpts(mock)
	err := showLogs(context.Background(), global, "exec-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestShowLogs_WhenNoLogsAvailable_ItShouldReturnNilError(t *testing.T) {
	mock := &mockClient{
		getExecutionFn: func(_ context.Context, _ string, _ string) (*client.Execution, error) {
			return &client.Execution{
				ID:     "exec-456",
				Status: "succeeded",
				Logs:   "",
			}, nil
		},
	}

	global := newMockGlobalOpts(mock)
	err := showLogs(context.Background(), global, "exec-456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestShowLogs_WhenClientReturnsError_ItShouldPropagateError(t *testing.T) {
	mock := &mockClient{
		getExecutionFn: func(_ context.Context, _ string, _ string) (*client.Execution, error) {
			return nil, &client.APIError{Code: "not_found", Reason: "execution not found"}
		},
	}

	global := newMockGlobalOpts(mock)
	err := showLogs(context.Background(), global, "nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "execution not found" {
		t.Errorf("expected 'execution not found', got %q", err.Error())
	}
}
