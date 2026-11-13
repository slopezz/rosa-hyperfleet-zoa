package cli

import (
	"context"
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/cli/reason"
	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
)

func TestCreateBreakglassRequest_WhenZOAReasonEnvSet_ItShouldSendEnvReason(t *testing.T) {
	t.Setenv(reason.EnvVar, "ROSAENG-4242")

	var sentReason string
	mock := &mockClient{
		createBreakglassFn: func(_ context.Context, _ string, r string) (*client.BreakglassRequest, error) {
			sentReason = r
			return &client.BreakglassRequest{RequestID: "req-1", Status: "pending_approval"}, nil
		},
	}

	global := newMockGlobalOpts(mock)
	_, err := createBreakglassRequest(context.Background(), global, "kube-read", "")
	if err != nil {
		t.Fatalf("createBreakglassRequest: %v", err)
	}
	if sentReason != "ROSAENG-4242" {
		t.Fatalf("expected ZOA_REASON on create, got %q", sentReason)
	}
}

func TestCreateBreakglassRequest_WhenFlagAndEnvSet_ItShouldPreferFlag(t *testing.T) {
	t.Setenv(reason.EnvVar, "ROSAENG-9999")

	var sentReason string
	mock := &mockClient{
		createBreakglassFn: func(_ context.Context, _ string, r string) (*client.BreakglassRequest, error) {
			sentReason = r
			return &client.BreakglassRequest{RequestID: "req-2", Status: "pending_approval"}, nil
		},
	}

	global := newMockGlobalOpts(mock)
	_, err := createBreakglassRequest(context.Background(), global, "kube-read", "ROSAENG-4242")
	if err != nil {
		t.Fatalf("createBreakglassRequest: %v", err)
	}
	if sentReason != "ROSAENG-4242" {
		t.Fatalf("--reason must win over ZOA_REASON; got %q", sentReason)
	}
}
