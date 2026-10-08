package cli

import (
	"context"
	"testing"

	"github.com/spf13/cobra"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/cli/reason"
	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/client"
)

func TestSessionStart_WhenReasonMissing_ItShouldNotCallAPI(t *testing.T) {
	mock := &mockClient{
		sessionStartFn: func(_ context.Context, _ *client.SessionStartRequest) (*client.SessionStartResponse, error) {
			t.Fatal("SessionStart should not be called without --reason")
			return nil, nil
		},
	}
	root := sessionTestRoot(newMockGlobalOpts(mock))
	root.SetArgs([]string{"session", "start", "us-east-1", "mc01", "--no-connect"})

	if err := root.Execute(); err == nil {
		t.Fatal("expected error when --reason is missing")
	}
}

func TestSessionStart_WhenReasonSet_ItShouldPassToAPI(t *testing.T) {
	var gotReason string
	mock := &mockClient{
		sessionStartFn: func(_ context.Context, req *client.SessionStartRequest) (*client.SessionStartResponse, error) {
			gotReason = req.Reason
			return &client.SessionStartResponse{
				SessionID: "sess-abc",
				Status:    "active",
				Target:    "mc01",
			}, nil
		},
	}
	root := sessionTestRoot(newMockGlobalOpts(mock))
	root.SetArgs([]string{"session", "start", "us-east-1", "mc01", "--reason", "ROSAENG-4242", "--no-connect"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if gotReason != "ROSAENG-4242" {
		t.Fatalf("expected reason on request, got %q", gotReason)
	}
}

func TestSessionStart_WhenReasonOnlyInEnv_ItShouldPassToAPI(t *testing.T) {
	t.Setenv(reason.EnvVar, "ROSAENG-777")

	var gotReason string
	mock := &mockClient{
		sessionStartFn: func(_ context.Context, req *client.SessionStartRequest) (*client.SessionStartResponse, error) {
			gotReason = req.Reason
			return &client.SessionStartResponse{SessionID: "sess-env", Status: "creating"}, nil
		},
	}
	root := sessionTestRoot(newMockGlobalOpts(mock))
	root.SetArgs([]string{"session", "start", "us-east-1", "mc01", "--no-connect"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if gotReason != "ROSAENG-777" {
		t.Fatalf("expected ZOA_REASON on request, got %q", gotReason)
	}
}

func sessionTestRoot(opts *GlobalOptions) *cobra.Command {
	root := &cobra.Command{Use: "zoa"}
	root.AddCommand(newSessionCommand(opts))
	return root
}
