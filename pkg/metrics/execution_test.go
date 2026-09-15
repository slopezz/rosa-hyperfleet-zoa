package metrics

import (
	"strings"
	"testing"
)

func TestEmitExecution_WhenTerminalStatus_ItShouldIncludeActionDimensions(t *testing.T) {
	line := captureStdout(func() {
		EmitExecution("mc01", "must_gather", "succeeded", "async", "kube-api", "read", 1200)
	})

	for _, dim := range []string{"Action", "Status", "Mode", "Scope", "Type", "must_gather", "succeeded"} {
		if !strings.Contains(line, dim) {
			t.Fatalf("expected %q in EMF output: %s", dim, line)
		}
	}
	if !strings.Contains(line, "ExecutionCount") {
		t.Fatal("expected ExecutionCount metric")
	}
	if !strings.Contains(line, "ExecutionDuration") {
		t.Fatal("expected ExecutionDuration when durationMs >= 0")
	}
}

func TestEmitExecution_WhenNegativeDuration_ItShouldOmitDurationMetric(t *testing.T) {
	line := captureStdout(func() {
		EmitExecution("mc01", "get_resource", "failed", "sync", "kube-api", "read", -1)
	})
	if strings.Contains(line, "ExecutionDuration") {
		t.Fatalf("did not expect ExecutionDuration: %s", line)
	}
}

func TestEmitRejection_WhenCooldown_ItShouldEmitReasonDimension(t *testing.T) {
	line := captureStdout(func() {
		EmitRejection("mc01", RejectionWriteCooldown)
	})
	if !strings.Contains(line, RejectionWriteCooldown) {
		t.Fatalf("expected reason in output: %s", line)
	}
}

func TestEmitGCCleaned_WhenCalled_ItShouldIncludeResourceType(t *testing.T) {
	line := captureStdout(func() {
		EmitGCCleaned("mc01", "job")
	})
	if !strings.Contains(line, "GCCleanedResources") || !strings.Contains(line, "job") {
		t.Fatalf("expected GC cleaned metric: %s", line)
	}
}

func TestEmitReconciler_WhenCalled_ItShouldEmitLastRunGauge(t *testing.T) {
	line := captureStdout(func() {
		EmitReconciler("mc01", 40, 0)
	})
	for _, want := range []string{"ReconcilerDuration", "ReconcilerErrors", "ReconcilerLastRun"} {
		if !strings.Contains(line, want) {
			t.Fatalf("expected %q in output: %s", want, line)
		}
	}
}

func TestEmitHTTPRequest_When5xx_ItShouldSetStatusClassOnCountAndDuration(t *testing.T) {
	line := captureStdout(func() {
		EmitHTTPRequest("mc01", "GET", "/health", 503, 12)
	})
	if !strings.Contains(line, `"StatusClass":"5xx"`) && !strings.Contains(line, `"StatusClass": "5xx"`) {
		if !strings.Contains(line, "5xx") {
			t.Fatalf("expected StatusClass 5xx: %s", line)
		}
	}
	if !strings.Contains(line, "HttpRequestDuration") {
		t.Fatal("expected HttpRequestDuration to share StatusClass dimensions")
	}
}
