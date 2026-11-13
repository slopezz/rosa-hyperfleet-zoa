package cli

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
)

func TestPrintBreakglassRequestDispatched_WhenComplete_ItShouldMatchSessionStyle(t *testing.T) {
	req := &client.BreakglassRequest{
		RequestID:     "abc-def",
		TargetCluster: "eph-1fcc5750-regional",
		Scope:         "kube-read",
	}
	var buf bytes.Buffer
	printBreakglassRequestDispatched(&buf, req)
	want := "✓ abc-def [eph-1fcc5750-regional] scope=kube-read\n"
	if buf.String() != want {
		t.Fatalf("dispatched line = %q, want %q", buf.String(), want)
	}
}

func TestPrintBreakglassActivateDispatched_WhenComplete_ItShouldAppendActivatedSuffix(t *testing.T) {
	var buf bytes.Buffer
	printBreakglassActivateDispatched(&buf, "req-1", "eph-regional", "aws-read")
	want := "✓ req-1 [eph-regional] scope=aws-read · activated\n"
	if buf.String() != want {
		t.Fatalf("activate line = %q, want %q", buf.String(), want)
	}
}

func TestBreakglassRequestNextStep_WhenApproved_ItShouldSuggestActivate(t *testing.T) {
	req := &client.BreakglassRequest{RequestID: "abc", Status: "approved"}
	got := breakglassRequestNextStep(req, "us-east-1", false)
	if !strings.Contains(got, "activate abc") {
		t.Fatalf("expected activate hint, got %q", got)
	}
}

func TestRenderBreakglassRequest_WhenApproved_ItShouldPrintActivateHintOnStderr(t *testing.T) {
	req := &client.BreakglassRequest{
		RequestID:      "abc",
		Status:         "approved",
		Scope:          "kube-read",
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
	if !strings.Contains(out, "✓ abc [eph-regional] scope=kube-read") {
		t.Fatalf("expected dispatched line on stderr, got:\n%s", out)
	}
	if !strings.Contains(out, "activate abc") {
		t.Fatalf("expected activate hint, got:\n%s", out)
	}
}

func TestBreakglassWaitApprovalError_WhenRejected_ItShouldFail(t *testing.T) {
	req := &client.BreakglassRequest{RequestID: "x", Status: "rejected"}
	if err := breakglassWaitApprovalError(req, false); err == nil {
		t.Fatal("expected error for rejected")
	}
}

func TestRenderBreakglassGetHuman_WhenApproved_ItShouldPrintStdoutMetadata(t *testing.T) {
	req := &client.BreakglassRequest{
		RequestID:     "abc",
		Status:        "approved",
		Scope:         "kube-read",
		TargetCluster: "eph-regional",
	}
	var buf bytes.Buffer
	renderBreakglassGetHuman(&buf, req, false)
	out := buf.String()
	if strings.Contains(out, "✓ ") {
		t.Fatalf("get human view must not print dispatch ack, got:\n%s", out)
	}
	if !strings.Contains(out, "ID:        abc") || !strings.Contains(out, "TARGET:    eph-regional") {
		t.Fatalf("expected TA-style fields, got:\n%s", out)
	}
	if !strings.Contains(out, "activate abc") {
		t.Fatalf("expected NEXT activate hint, got:\n%s", out)
	}
}

func TestBreakglassWaitApprovalError_WhenApproved_ItShouldSucceed(t *testing.T) {
	req := &client.BreakglassRequest{RequestID: "x", Status: "approved"}
	if err := breakglassWaitApprovalError(req, false); err != nil {
		t.Fatalf("approved should not error: %v", err)
	}
}
