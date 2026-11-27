package cli

import (
	"context"
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
	"github.com/openshift-online/rosa-seaplane-zoa/internal/output"
)

func TestShowOutput_WhenOutputAvailable_ItShouldReturnNilError(t *testing.T) {
	mock := &mockClient{
		getExecutionFn: func(_ context.Context, id string, include string) (*client.Execution, error) {
			if id != "exec-789" {
				t.Errorf("expected id 'exec-789', got %q", id)
			}
			if include != "output" {
				t.Errorf("expected include 'output', got %q", include)
			}
			return &client.Execution{
				ID:     "exec-789",
				Status: "succeeded",
				Output: client.FlexString(`[{"Name":"pod-1","Status":"Running"}]`),
			}, nil
		},
	}

	global := newMockGlobalOpts(mock)
	err := showOutput(context.Background(), global, "exec-789")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestShowOutput_WhenNoOutputAvailable_ItShouldReturnNilError(t *testing.T) {
	mock := &mockClient{
		getExecutionFn: func(_ context.Context, _ string, _ string) (*client.Execution, error) {
			return &client.Execution{
				ID:     "exec-000",
				Status: "succeeded",
				Output: "",
			}, nil
		},
	}

	global := newMockGlobalOpts(mock)
	err := showOutput(context.Background(), global, "exec-000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestShowOutput_WhenJSONFormat_ItShouldReturnNilError(t *testing.T) {
	mock := &mockClient{
		getExecutionFn: func(_ context.Context, _ string, _ string) (*client.Execution, error) {
			return &client.Execution{
				ID:     "exec-json",
				Status: "succeeded",
				Output: client.FlexString(`{"key":"value"}`),
			}, nil
		},
	}

	global := newMockGlobalOpts(mock)
	global.OutputFormat = output.FormatJSON
	err := showOutput(context.Background(), global, "exec-json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestShowOutput_WhenClientReturnsError_ItShouldPropagateError(t *testing.T) {
	mock := &mockClient{
		getExecutionFn: func(_ context.Context, _ string, _ string) (*client.Execution, error) {
			return nil, &client.APIError{Code: "not_found", Reason: "execution not found"}
		},
	}

	global := newMockGlobalOpts(mock)
	err := showOutput(context.Background(), global, "missing-id")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
