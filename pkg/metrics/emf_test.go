package metrics

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

func captureStdout(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

func TestEmit_WhenDimensionsProvided_ItShouldSortKeysAndEmitValidEMF(t *testing.T) {
	line := captureStdout(func() {
		Emit(
			map[string]string{
				"Cluster": "test-cluster",
				"Action":  "get_resource",
				"Status":  "succeeded",
			},
			map[string]MetricValue{
				"ExecutionCount": Count(1),
			},
		)
	})

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(line), &payload); err != nil {
		t.Fatalf("invalid JSON: %v\nline: %s", err, line)
	}

	aws, ok := payload["_aws"].(map[string]interface{})
	if !ok {
		t.Fatal("missing _aws block")
	}
	cwMetrics, ok := aws["CloudWatchMetrics"].([]interface{})
	if !ok || len(cwMetrics) == 0 {
		t.Fatal("missing CloudWatchMetrics")
	}
	directive, ok := cwMetrics[0].(map[string]interface{})
	if !ok {
		t.Fatal("invalid directive")
	}
	if directive["Namespace"] != Namespace {
		t.Fatalf("expected namespace %q, got %v", Namespace, directive["Namespace"])
	}

	dims, ok := directive["Dimensions"].([]interface{})
	if !ok || len(dims) == 0 {
		t.Fatal("missing dimensions")
	}
	dimKeys, ok := dims[0].([]interface{})
	if !ok || len(dimKeys) != 3 {
		t.Fatalf("expected 3 dimension keys, got %v", dims[0])
	}
	// Sorted alphabetically: Action, Cluster, Status
	if dimKeys[0] != "Action" || dimKeys[1] != "Cluster" || dimKeys[2] != "Status" {
		t.Fatalf("expected sorted dimension keys, got %v", dimKeys)
	}
}

func TestEmitHTTPRequest_WhenCalled_ItShouldUseRouteTemplateNotRawPath(t *testing.T) {
	line := captureStdout(func() {
		EmitHTTPRequest("mc01", "GET", "/api/v0/trusted-actions/runs/550e8400-e29b-41d4-a716-446655440000", 200, 42)
	})

	if line == "" {
		t.Fatal("expected EMF output")
	}
	if strings.Contains(line, "550e8400") {
		t.Fatal("raw UUID should not appear in EMF output")
	}
	if !bytes.Contains([]byte(line), []byte("RouteTemplate")) {
		t.Fatal("expected RouteTemplate dimension")
	}
}
