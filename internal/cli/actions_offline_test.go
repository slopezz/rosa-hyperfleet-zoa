package cli

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/cli/targettype"
	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/output"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/actions"
)

func TestListActionsOffline_WhenTargetTypeRC_ItShouldWriteMarkdownWithoutAPI(t *testing.T) {
	global := &GlobalOptions{OutputFormat: output.FormatMarkdown}
	opts := &catalogOptions{offline: true, targetType: actions.DeploymentTargetRC}

	stdout := captureStdout(t, func() {
		if err := listActionsOffline(global, opts); err != nil {
			t.Fatalf("listActionsOffline: %v", err)
		}
	})

	if !strings.Contains(stdout, "## get_resource") {
		t.Fatalf("expected catalog body, got: %s", stdout[:min(200, len(stdout))])
	}
}

func TestListActionsOffline_WhenMarkdownWithoutOfflineFlag_ItShouldError(t *testing.T) {
	global := &GlobalOptions{OutputFormat: output.FormatMarkdown}
	opts := &catalogOptions{offline: false}
	err := listActions(context.Background(), global, opts)
	if err == nil || !strings.Contains(err.Error(), "--offline") {
		t.Fatalf("expected markdown offline error, got %v", err)
	}
}

func TestDescribeActionOffline_WhenActionExists_ItShouldNotCallAPI(t *testing.T) {
	global := &GlobalOptions{OutputFormat: output.FormatJSON}
	opts := &catalogOptions{offline: true, targetType: targettype.RC}

	stdout := captureStdout(t, func() {
		if err := describeActionOffline(global, opts, "get_resource"); err != nil {
			t.Fatalf("describeActionOffline: %v", err)
		}
	})
	if !strings.Contains(stdout, `"name"`) || !strings.Contains(stdout, "get_resource") {
		t.Fatalf("unexpected output: %s", stdout)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	oldErr := os.Stderr
	os.Stderr, _ = os.Open(os.DevNull)
	fn()
	_ = w.Close()
	os.Stdout = old
	os.Stderr = oldErr
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	return buf.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
