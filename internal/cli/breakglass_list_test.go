package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
)

func TestPrintBreakglassRequestTable_WhenMine_ItShouldRenderRows(t *testing.T) {
	list := &client.BreakglassRequestList{
		Count: 1,
		Items: []client.BreakglassRequest{{
			RequestID:     "req-1",
			Scope:         "kube-read",
			TargetCluster: "eph-regional",
			Status:        "approved",
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
	if !strings.Contains(out, "req-1") || !strings.Contains(out, "kube-read") {
		t.Fatalf("expected table row, got:\n%s", out)
	}
}

func TestFormatBreakglassTime_WhenRFC3339_ItShouldFormatUTC(t *testing.T) {
	got := formatBreakglassTime("2026-01-15T10:00:00Z")
	if got != "2026-01-15 10:00:00" {
		t.Fatalf("got %q", got)
	}
}
