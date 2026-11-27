package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/cli/reason"
	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
)

func TestBreakglassRequestArgs_WhenMissingProfile_ItShouldErrorWithUsage(t *testing.T) {
	cmd := newBreakglassRequestCommand(&GlobalOptions{})
	err := cmd.Args(cmd, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing profile name") {
		t.Fatalf("got %v", err)
	}
}

func TestPrintBreakglassRequestDispatched_WhenComplete_ItShouldMatchSessionStyle(t *testing.T) {
	req := &client.BreakglassRequest{
		RequestID:     "abc-def",
		TargetCluster: "eph-1fcc5750-regional",
		Profile:       "kube_read",
	}
	var buf bytes.Buffer
	printBreakglassRequestDispatched(&buf, req)
	want := "✓ abc-def [eph-1fcc5750-regional] profile=kube_read\n"
	if buf.String() != want {
		t.Fatalf("dispatched line = %q, want %q", buf.String(), want)
	}
}

func TestBreakglassRequestNextStep_WhenApproved_ItShouldSuggestActivate(t *testing.T) {
	req := &client.BreakglassRequest{RequestID: "abc", Status: "active"}
	got := breakglassRequestNextStep(req, "us-east-1", false)
	if !strings.Contains(got, "activate abc") {
		t.Fatalf("expected activate hint, got %q", got)
	}
}

func TestRenderBreakglassRequest_WhenApproved_ItShouldPrintActivateHintOnStderr(t *testing.T) {
	req := &client.BreakglassRequest{
		RequestID:      "abc",
		Status:         "active",
		Profile:        "kube_read",
		TargetCluster:  "eph-regional",
		GrantExpiresAt: "2030-01-01T00:00:00Z",
	}
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	global := &GlobalOptions{}
	if err := renderBreakglassRequest(global, req, false); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	os.Stderr = old
	var stderr bytes.Buffer
	_, _ = io.Copy(&stderr, r)
	out := stderr.String()
	if !strings.Contains(out, "✓ abc [eph-regional] profile=kube_read") {
		t.Fatalf("expected dispatched line on stderr, got:\n%s", out)
	}
	if !strings.Contains(out, "activate abc") {
		t.Fatalf("expected activate hint, got:\n%s", out)
	}
}

func TestCreateBreakglassRequest_WhenZOAReasonEnvSet_ItShouldSendEnvReason(t *testing.T) {
	t.Setenv(reason.EnvVar, "ROSAENG-4242")

	var sentReason string
	mock := &mockClient{
		createBreakglassFn: func(_ context.Context, _ string, r string) (*client.BreakglassRequest, error) {
			sentReason = r
			return &client.BreakglassRequest{RequestID: "req-1", Status: "pending"}, nil
		},
	}

	global := newMockGlobalOpts(mock)
	_, err := createBreakglassRequest(context.Background(), global, "kube_read", "")
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
			return &client.BreakglassRequest{RequestID: "req-2", Status: "pending"}, nil
		},
	}

	global := newMockGlobalOpts(mock)
	_, err := createBreakglassRequest(context.Background(), global, "kube_read", "ROSAENG-4242")
	if err != nil {
		t.Fatalf("createBreakglassRequest: %v", err)
	}
	if sentReason != "ROSAENG-4242" {
		t.Fatalf("--reason must win over ZOA_REASON; got %q", sentReason)
	}
}
