package cli

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
)

func TestListBreakglass_WhenCalled_ItShouldSetMineTrue(t *testing.T) {
	mock := &mockClient{
		listBreakglassFn: func(_ context.Context, params *client.BreakglassListParams) (*client.BreakglassRequestList, error) {
			if !params.Mine {
				t.Fatal("list command must set mine=true")
			}
			if params.Limit != 50 {
				t.Fatalf("expected default limit 50, got %d", params.Limit)
			}
			return &client.BreakglassRequestList{Items: []client.BreakglassRequest{}}, nil
		},
	}
	global := newMockGlobalOpts(mock)
	err := listBreakglassRequests(context.Background(), global, breakglassListOpts{mine: true, since: "24h", limit: 50})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListBreakglassHistory_WhenCalled_ItShouldNotSetMine(t *testing.T) {
	mock := &mockClient{
		listBreakglassFn: func(_ context.Context, params *client.BreakglassListParams) (*client.BreakglassRequestList, error) {
			if params.Mine {
				t.Fatal("history must not set mine=true")
			}
			if params.Operator != "alice" {
				t.Fatalf("expected operator filter, got %q", params.Operator)
			}
			return &client.BreakglassRequestList{Items: []client.BreakglassRequest{}}, nil
		},
	}
	global := newMockGlobalOpts(mock)
	err := listBreakglassRequests(context.Background(), global, breakglassListOpts{
		fleetView: true,
		operator:  "alice",
		since:     "24h",
		limit:     50,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrintBreakglassRequestTable_WhenMine_ItShouldRenderRows(t *testing.T) {
	list := &client.BreakglassRequestList{
		Count: 1,
		Items: []client.BreakglassRequest{{
			RequestID:     "req-1",
			Profile:       "kube_read",
			TargetCluster: "eph-regional",
			Status:        "active",
			Reason:        "ROSAENG-1",
			CreatedAt:     "2026-01-15T10:00:00Z",
		}},
	}
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	global := &GlobalOptions{}
	err = printBreakglassRequestTable(global, list, false)
	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	if err != nil {
		t.Fatalf("printBreakglassRequestTable: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "ID") || !strings.Contains(out, "PROFILE") || !strings.Contains(out, "req-1") || !strings.Contains(out, "kube_read") {
		t.Fatalf("expected table row, got:\n%s", out)
	}
}

func TestFormatBreakglassTime_WhenRFC3339_ItShouldFormatUTC(t *testing.T) {
	got := formatBreakglassTime("2026-01-15T10:00:00Z")
	if got != "2026-01-15 10:00:00" {
		t.Fatalf("got %q", got)
	}
}

func TestBreakglassLifecycleStatus_WhenPending_ItShouldEchoStatus(t *testing.T) {
	req := &client.BreakglassRequest{Status: "pending"}
	if got := breakglassLifecycleStatus(req); got != "pending" {
		t.Fatalf("got %q", got)
	}
}

func TestBreakglassWorkflowApproval_WhenApproved_ItShouldNotFoldActor(t *testing.T) {
	req := &client.BreakglassRequest{Status: "active", Approval: "approved", DecidedBy: "bob"}
	if got := breakglassWorkflowApproval(req); got != "approved" {
		t.Fatalf("got %q", got)
	}
	if got := breakglassDecidedBy(req); got != "bob" {
		t.Fatalf("got %q", got)
	}
}

func TestBreakglassDecidedAt_WhenRejected_ItShouldPreferUnifiedField(t *testing.T) {
	req := &client.BreakglassRequest{
		Status:    "denied",
		Approval:  "rejected",
		DecidedAt: "2026-11-13T12:00:00Z",
	}
	if got := breakglassDecidedAt(req); got != "2026-11-13T12:00:00Z" {
		t.Fatalf("got %q", got)
	}
}

func TestBreakglassValidUntil_WhenPending_ItShouldUseRequestExpires(t *testing.T) {
	req := &client.BreakglassRequest{
		Status:           "pending",
		RequestExpiresAt: "2026-11-14T10:00:00Z",
		GrantExpiresAt:   "2026-11-14T14:00:00Z",
	}
	if got := breakglassValidUntil(req); got != "2026-11-14T10:00:00Z" {
		t.Fatalf("got %q", got)
	}
}
