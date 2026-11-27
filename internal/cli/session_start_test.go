package cli

import (
	"context"
	"testing"

	"github.com/spf13/cobra"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/cli/reason"
	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
	"github.com/openshift-online/rosa-seaplane-zoa/internal/output"
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
	root.SetArgs([]string{"session", "start", "us-east-1", "mc01", "--reason", "TEST-12345", "--no-connect"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if gotReason != "TEST-12345" {
		t.Fatalf("expected reason on request, got %q", gotReason)
	}
}

func TestSessionStart_WhenNoConnect_ItShouldNotCallSessionJoin(t *testing.T) {
	mock := &mockClient{
		sessionStartFn: func(_ context.Context, _ *client.SessionStartRequest) (*client.SessionStartResponse, error) {
			return &client.SessionStartResponse{
				SessionID: "sess-nc",
				Status:    "active",
				Target:    "mc01",
			}, nil
		},
		sessionJoinFn: func(_ context.Context, _ string) (*client.SessionJoinResponse, error) {
			t.Fatal("SessionJoin should not be called with --no-connect")
			return nil, nil
		},
	}
	root := sessionTestRoot(newMockGlobalOpts(mock))
	root.SetArgs([]string{"session", "start", "us-east-1", "mc01", "--reason", "TEST-12345", "--no-connect"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

func TestSessionStart_WhenConnect_ItShouldCallSessionJoin(t *testing.T) {
	var joinCalled bool
	mock := &mockClient{
		sessionStartFn: func(_ context.Context, _ *client.SessionStartRequest) (*client.SessionStartResponse, error) {
			return &client.SessionStartResponse{
				SessionID: "sess-join",
				Status:    "active",
				Target:    "mc01",
				Deadline:  "2030-01-01T00:00:00Z",
			}, nil
		},
		sessionJoinFn: func(_ context.Context, sessionID string) (*client.SessionJoinResponse, error) {
			joinCalled = true
			if sessionID != "sess-join" {
				t.Fatalf("expected session id sess-join, got %q", sessionID)
			}
			return &client.SessionJoinResponse{
				TaskArn:       "arn:aws:ecs:us-east-1:123:task/c/t",
				ClusterArn:    "arn:aws:ecs:us-east-1:123:cluster/c",
				Region:        "us-east-1",
				ContainerName: "zoa-boundary",
				ExecCommand:   "/bin/bash",
				ExecCredentials: &client.ExecCredentials{
					AccessKeyID:     "AKIA",
					SecretAccessKey: "secret",
					SessionToken:    "token",
				},
			}, nil
		},
	}
	root := sessionTestRoot(newMockGlobalOpts(mock))
	root.SetArgs([]string{"session", "start", "us-east-1", "mc01", "--reason", "TEST-12345"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected ECS Exec to fail in unit test environment")
	}
	if !joinCalled {
		t.Fatal("expected SessionJoin before ECS Exec")
	}
}

func TestSessionStart_WhenReasonOnlyInEnv_ItShouldPassToAPI(t *testing.T) {
	t.Setenv(reason.EnvVar, "TEST-12345")

	var gotReason string
	mock := &mockClient{
		sessionStartFn: func(_ context.Context, req *client.SessionStartRequest) (*client.SessionStartResponse, error) {
			gotReason = req.Reason
			return &client.SessionStartResponse{SessionID: "sess-env", Status: "active"}, nil
		},
	}
	root := sessionTestRoot(newMockGlobalOpts(mock))
	root.SetArgs([]string{"session", "start", "us-east-1", "mc01", "--no-connect"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if gotReason != "TEST-12345" {
		t.Fatalf("expected ZOA_REASON on request, got %q", gotReason)
	}
}

func TestSessionStart_WhenExecScriptWithNoConnect_ItShouldRejectBeforeAPI(t *testing.T) {
	mock := &mockClient{
		sessionStartFn: func(_ context.Context, _ *client.SessionStartRequest) (*client.SessionStartResponse, error) {
			t.Fatal("SessionStart should not be called when --exec-script and --no-connect")
			return nil, nil
		},
	}
	root := sessionTestRoot(newMockGlobalOpts(mock))
	root.SetArgs([]string{
		"session", "start", "us-east-1", "mc01",
		"--reason", "TEST-12345", "--no-connect", "--exec-script", "true",
	})

	if err := root.Execute(); err == nil {
		t.Fatal("expected error for --exec-script with --no-connect")
	}
}

func TestSessionStart_WhenExecScriptWithJSONOutput_ItShouldRejectBeforeAPI(t *testing.T) {
	mock := &mockClient{
		sessionStartFn: func(_ context.Context, _ *client.SessionStartRequest) (*client.SessionStartResponse, error) {
			t.Fatal("SessionStart should not be called when --exec-script and -o json")
			return nil, nil
		},
	}
	opts := newMockGlobalOpts(mock)
	opts.OutputFormat = output.FormatJSON
	root := sessionTestRoot(opts)
	root.SetArgs([]string{
		"session", "start", "us-east-1", "mc01",
		"--reason", "TEST-12345", "--exec-script", "true",
	})

	if err := root.Execute(); err == nil {
		t.Fatal("expected error for --exec-script with -o json")
	}
}

func TestSessionStart_WhenExecScriptMaxOutputNonPositive_ItShouldRejectBeforeAPI(t *testing.T) {
	mock := &mockClient{
		sessionStartFn: func(_ context.Context, _ *client.SessionStartRequest) (*client.SessionStartResponse, error) {
			t.Fatal("SessionStart should not be called when --exec-script-max-output is invalid")
			return nil, nil
		},
	}
	root := sessionTestRoot(newMockGlobalOpts(mock))
	root.SetArgs([]string{
		"session", "start", "us-east-1", "mc01",
		"--reason", "TEST-12345", "--exec-script", "true", "--exec-script-max-output", "0",
	})

	if err := root.Execute(); err == nil {
		t.Fatal("expected error for --exec-script-max-output 0")
	}
}

func TestResolveSessionStartArgs(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		flagDeployment string
		flagTarget     string
		wantDeployment string
		wantTarget     string
	}{
		{
			name:           "When both positional args are given it should use them",
			args:           []string{"us-east-1", "mc01"},
			wantDeployment: "us-east-1",
			wantTarget:     "mc01",
		},
		{
			name:           "When positional args are given they should override flags",
			args:           []string{"us-east-1", "mc01"},
			flagDeployment: "eu-west-1",
			flagTarget:     "mc02",
			wantDeployment: "us-east-1",
			wantTarget:     "mc01",
		},
		{
			name:           "When one positional arg is given it should be deployment with flag target",
			args:           []string{"us-east-1"},
			flagTarget:     "mc01",
			wantDeployment: "us-east-1",
			wantTarget:     "mc01",
		},
		{
			name:           "When no positional args are given it should fall back to flags",
			args:           []string{},
			flagDeployment: "us-east-1",
			flagTarget:     "mc01",
			wantDeployment: "us-east-1",
			wantTarget:     "mc01",
		},
		{
			name:           "When nothing is provided it should return empty strings",
			args:           []string{},
			wantDeployment: "",
			wantTarget:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deployment, target := resolveSessionStartArgs(tt.args, tt.flagDeployment, tt.flagTarget)
			if deployment != tt.wantDeployment {
				t.Errorf("deployment = %q, want %q", deployment, tt.wantDeployment)
			}
			if target != tt.wantTarget {
				t.Errorf("target = %q, want %q", target, tt.wantTarget)
			}
		})
	}
}

func sessionTestRoot(opts *GlobalOptions) *cobra.Command {
	root := &cobra.Command{Use: "zoa"}
	root.AddCommand(newSessionCommand(opts))
	return root
}
