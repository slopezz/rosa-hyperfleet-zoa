package cli

import (
	"context"
	"testing"

	"github.com/spf13/cobra"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/client"
)

func TestSessionStart_WhenJiraMissing_ItShouldNotCallAPI(t *testing.T) {
	mock := &mockClient{
		sessionStartFn: func(_ context.Context, _ *client.SessionStartRequest) (*client.SessionStartResponse, error) {
			t.Fatal("SessionStart should not be called without --jira")
			return nil, nil
		},
	}
	root := sessionTestRoot(newMockGlobalOpts(mock))
	root.SetArgs([]string{"session", "start", "us-east-1", "mc01", "--no-connect"})

	if err := root.Execute(); err == nil {
		t.Fatal("expected error when --jira is missing")
	}
}

func TestSessionStart_WhenJiraSet_ItShouldPassTicketToAPI(t *testing.T) {
	var gotJira string
	mock := &mockClient{
		sessionStartFn: func(_ context.Context, req *client.SessionStartRequest) (*client.SessionStartResponse, error) {
			gotJira = req.Jira
			return &client.SessionStartResponse{
				SessionID: "sess-abc",
				Status:    "active",
				Target:    "mc01",
			}, nil
		},
	}
	root := sessionTestRoot(newMockGlobalOpts(mock))
	root.SetArgs([]string{"session", "start", "us-east-1", "mc01", "--jira", "ROSAENG-4242", "--no-connect"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if gotJira != "ROSAENG-4242" {
		t.Fatalf("expected jira on request, got %q", gotJira)
	}
}

func sessionTestRoot(opts *GlobalOptions) *cobra.Command {
	root := &cobra.Command{Use: "zoa"}
	root.AddCommand(newSessionCommand(opts))
	return root
}
