package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
)

func TestRenderBreakglassGetHuman_WhenApproved_ItShouldPrintStdoutMetadata(t *testing.T) {
	req := &client.BreakglassRequest{
		RequestID:     "abc",
		Status:        "active",
		Profile:       "kube_read",
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

func TestBreakglassWaitApprovalError_WhenRejected_ItShouldFail(t *testing.T) {
	req := &client.BreakglassRequest{RequestID: "x", Status: "denied"}
	if err := breakglassWaitApprovalError(req, false); err == nil {
		t.Fatal("expected error for rejected")
	}
}

func TestBreakglassWaitApprovalError_WhenApproved_ItShouldSucceed(t *testing.T) {
	req := &client.BreakglassRequest{RequestID: "x", Status: "active"}
	if err := breakglassWaitApprovalError(req, false); err != nil {
		t.Fatalf("active should not error: %v", err)
	}
}

func TestIsTerminalBreakglassStatus_WhenPending_ItShouldBeNonTerminal(t *testing.T) {
	if isTerminalBreakglassStatus("pending") {
		t.Fatal("pending should not be terminal")
	}
	if !isTerminalBreakglassStatus("active") {
		t.Fatal("active should be terminal")
	}
}
