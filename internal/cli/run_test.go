package cli

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/cli/parambind"
	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/client"
	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/output"
)

func TestBuildParams_WhenClusterIDSet_ItShouldIncludeClusterID(t *testing.T) {
	opts := &runOptions{ta: parambind.RunTAParams{ClusterID: "1600392f-9a94-4957-b672-eff8dc2be0bb"}}
	params := parambind.ToAPIParams(opts.ta, opts.params)
	if params["cluster_id"] != "1600392f-9a94-4957-b672-eff8dc2be0bb" {
		t.Errorf("expected cluster_id, got %q", params["cluster_id"])
	}
}

func TestBuildParams_WhenGatherSet_ItShouldIncludeGather(t *testing.T) {
	opts := &runOptions{ta: parambind.RunTAParams{Gather: "mc,rc"}}
	params := parambind.ToAPIParams(opts.ta, opts.params)
	if params["gather"] != "mc,rc" {
		t.Errorf("expected gather=mc,rc, got %q", params["gather"])
	}
}

func TestBuildParams_WhenNamespaceSet_ItShouldIncludeNamespace(t *testing.T) {
	opts := &runOptions{ta: parambind.RunTAParams{Namespace: "grafana"}}
	params := parambind.ToAPIParams(opts.ta, opts.params)
	if params["namespace"] != "grafana" {
		t.Errorf("expected namespace=grafana, got %q", params["namespace"])
	}
}

func TestBuildParams_WhenAllNamespacesSet_ItShouldIncludeAllNamespacesTrue(t *testing.T) {
	opts := &runOptions{ta: parambind.RunTAParams{AllNamespaces: true}}
	params := parambind.ToAPIParams(opts.ta, opts.params)
	if params["all_namespaces"] != "true" {
		t.Errorf("expected all_namespaces=true, got %q", params["all_namespaces"])
	}
}

func TestBuildParams_WhenSelectorSet_ItShouldIncludeLabelSelector(t *testing.T) {
	opts := &runOptions{ta: parambind.RunTAParams{LabelSelector: "app=nginx"}}
	params := parambind.ToAPIParams(opts.ta, opts.params)
	if params["label_selector"] != "app=nginx" {
		t.Errorf("expected label_selector=app=nginx, got %q", params["label_selector"])
	}
}

func TestBuildParams_WhenVerboseSet_ItShouldIncludeVerboseTrue(t *testing.T) {
	opts := &runOptions{ta: parambind.RunTAParams{Verbose: true}}
	params := parambind.ToAPIParams(opts.ta, opts.params)
	if params["verbose"] != "true" {
		t.Errorf("expected verbose=true, got %q", params["verbose"])
	}
}

func TestBuildParams_WhenExtraParamsProvided_ItShouldMergeThem(t *testing.T) {
	opts := &runOptions{
		ta:     parambind.RunTAParams{Namespace: "default"},
		params: []string{"custom_key=custom_value", "another=val"},
	}
	params := parambind.ToAPIParams(opts.ta, opts.params)
	if params["custom_key"] != "custom_value" {
		t.Errorf("expected custom_key=custom_value, got %q", params["custom_key"])
	}
	if params["another"] != "val" {
		t.Errorf("expected another=val, got %q", params["another"])
	}
}

func TestBuildParams_WhenExtraParamConflicts_ItShouldPreferBuiltInFlag(t *testing.T) {
	opts := &runOptions{
		ta:     parambind.RunTAParams{Namespace: "grafana"},
		params: []string{"namespace=should-be-ignored"},
	}
	params := parambind.ToAPIParams(opts.ta, opts.params)
	if params["namespace"] != "grafana" {
		t.Errorf("expected namespace=grafana (from flag), got %q", params["namespace"])
	}
}

func TestBuildParams_WhenNoParamsSet_ItShouldReturnNil(t *testing.T) {
	opts := &runOptions{}
	params := parambind.ToAPIParams(opts.ta, opts.params)
	if params != nil {
		t.Errorf("expected nil params, got %v", params)
	}
}

func TestFormatTags_WhenDryRunWithExecutedAction_ItShouldShowMapping(t *testing.T) {
	result := formatTags("delete_pod", "get_pod", false, true)
	expected := " [dry-run:delete_pod→get_pod]"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestFormatTags_WhenForced_ItShouldShowForced(t *testing.T) {
	result := formatTags("rollout_restart", "", true, false)
	expected := " [forced]"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestFormatTags_WhenDryRunAndForced_ItShouldShowBoth(t *testing.T) {
	result := formatTags("delete_pod", "get_pod", true, true)
	expected := " [dry-run:delete_pod→get_pod, forced]"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestFormatTags_WhenNoFlags_ItShouldReturnEmpty(t *testing.T) {
	result := formatTags("get_pods", "", false, false)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestIsTerminalStatus_WhenSucceeded_ItShouldReturnTrue(t *testing.T) {
	for _, status := range []string{"succeeded", "failed", "timed_out", "rejected"} {
		if !isTerminalStatus(status) {
			t.Errorf("expected %q to be terminal", status)
		}
	}
}

func TestIsTerminalStatus_WhenRunning_ItShouldReturnFalse(t *testing.T) {
	for _, status := range []string{"running", "pending", "dispatched"} {
		if isTerminalStatus(status) {
			t.Errorf("expected %q to be non-terminal", status)
		}
	}
}

func TestRunAction_WhenSyncSucceeds_ItShouldReturnNilError(t *testing.T) {
	dur := int64(1200)
	mock := &mockClient{
		dispatchFn: func(_ context.Context, action string, req *client.DispatchRequest) (*client.DispatchResponse, error) {
			if action != "get_pods" {
				t.Errorf("expected action 'get_pods', got %q", action)
			}
			if req.Reason != "ROSAENG-1234" {
				t.Errorf("expected reason 'ROSAENG-1234', got %q", req.Reason)
			}
			if req.Params["namespace"] != "grafana" {
				t.Errorf("expected namespace=grafana, got %q", req.Params["namespace"])
			}
			return &client.DispatchResponse{
				ID:            "exec-sync-1",
				Status:        "succeeded",
				TargetCluster: "mc-useast1-1",
				ExecutionMode: "sync",
			}, nil
		},
		getExecutionFn: func(_ context.Context, id string, include string) (*client.Execution, error) {
			return &client.Execution{
				ID:            id,
				Status:        "succeeded",
				ExecutionMode: "sync",
				Output:        client.FlexString(`[{"name":"pod-1","status":"Running"}]`),
				DurationMs:    &dur,
			}, nil
		},
	}

	global := newMockGlobalOpts(mock)
	opts := &runOptions{
		ta:     parambind.RunTAParams{Namespace: "grafana"},
		reason: "ROSAENG-1234",
	}
	err := runAction(context.Background(), global, opts, "get_pods")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunAction_WhenNoWait_ItShouldReturnImmediately(t *testing.T) {
	mock := &mockClient{
		dispatchFn: func(_ context.Context, _ string, _ *client.DispatchRequest) (*client.DispatchResponse, error) {
			return &client.DispatchResponse{
				ID:            "exec-nowait",
				Status:        "dispatched",
				TargetCluster: "mc-useast1-1",
				ExecutionMode: "async",
			}, nil
		},
	}

	global := newMockGlobalOpts(mock)
	opts := &runOptions{
		reason: "ROSAENG-1234",
		noWait: true,
	}
	err := runAction(context.Background(), global, opts, "get_resource")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunAction_WhenNoWaitJSON_ItShouldReturnDispatchResponse(t *testing.T) {
	mock := &mockClient{
		dispatchFn: func(_ context.Context, _ string, _ *client.DispatchRequest) (*client.DispatchResponse, error) {
			return &client.DispatchResponse{
				ID:            "exec-nowait-json",
				Status:        "dispatched",
				TargetCluster: "mc-useast1-1",
				ExecutionMode: "async",
			}, nil
		},
	}

	global := newMockGlobalOpts(mock)
	global.OutputFormat = output.FormatJSON
	opts := &runOptions{
		reason: "ROSAENG-1234",
		noWait: true,
	}
	err := runAction(context.Background(), global, opts, "get_resource")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunAction_WhenDispatchFails_ItShouldReturnError(t *testing.T) {
	mock := &mockClient{
		dispatchFn: func(_ context.Context, _ string, _ *client.DispatchRequest) (*client.DispatchResponse, error) {
			return nil, &client.APIError{Code: "cooldown", Reason: "action was executed within the last 60s; use force=true to override"}
		},
	}

	global := newMockGlobalOpts(mock)
	opts := &runOptions{reason: "ROSAENG-1234"}
	err := runAction(context.Background(), global, opts, "delete_pod")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRunAction_WhenSyncFails_ItShouldReturnError(t *testing.T) {
	dur := int64(500)
	mock := &mockClient{
		dispatchFn: func(_ context.Context, _ string, _ *client.DispatchRequest) (*client.DispatchResponse, error) {
			return &client.DispatchResponse{
				ID:            "exec-fail",
				Status:        "failed",
				TargetCluster: "mc-useast1-1",
				ExecutionMode: "sync",
			}, nil
		},
		getExecutionFn: func(_ context.Context, _ string, _ string) (*client.Execution, error) {
			return &client.Execution{
				ID:            "exec-fail",
				Status:        "failed",
				ExecutionMode: "sync",
				Logs:          "ERROR: pod not found\n",
				DurationMs:    &dur,
			}, nil
		},
	}

	global := newMockGlobalOpts(mock)
	opts := &runOptions{reason: "ROSAENG-1234"}
	err := runAction(context.Background(), global, opts, "delete_pod")
	if err == nil {
		t.Fatal("expected error for failed execution, got nil")
	}
}

func TestPoll_WhenExecutionCompletesImmediately_ItShouldReturnWithOutput(t *testing.T) {
	mock := &mockClient{
		getExecutionFn: func(_ context.Context, id string, include string) (*client.Execution, error) {
			return &client.Execution{
				ID:     id,
				Status: "succeeded",
				Output: client.FlexString("result data"),
			}, nil
		},
	}

	exec, err := poll(context.Background(), mock, "exec-done", pollConfig{
		interval: 10 * time.Millisecond,
		timeout:  1 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exec.Status != "succeeded" {
		t.Errorf("expected status 'succeeded', got %q", exec.Status)
	}
}

func TestPoll_WhenExecutionTransitions_ItShouldWaitAndReturn(t *testing.T) {
	callCount := 0
	mock := &mockClient{
		getExecutionFn: func(_ context.Context, id string, include string) (*client.Execution, error) {
			callCount++
			if callCount < 3 {
				return &client.Execution{ID: id, Status: "running"}, nil
			}
			return &client.Execution{ID: id, Status: "succeeded", Output: client.FlexString("final")}, nil
		},
	}

	exec, err := poll(context.Background(), mock, "exec-transition", pollConfig{
		interval: 5 * time.Millisecond,
		timeout:  1 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exec.Status != "succeeded" {
		t.Errorf("expected status 'succeeded', got %q", exec.Status)
	}
	if callCount < 3 {
		t.Errorf("expected at least 3 calls, got %d", callCount)
	}
}

func TestPoll_WhenTimeoutExceeded_ItShouldReturnTimeoutError(t *testing.T) {
	mock := &mockClient{
		getExecutionFn: func(_ context.Context, id string, _ string) (*client.Execution, error) {
			return &client.Execution{ID: id, Status: "running"}, nil
		},
	}

	_, err := poll(context.Background(), mock, "exec-stuck", pollConfig{
		interval: 5 * time.Millisecond,
		timeout:  30 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestPoll_WhenContextCanceled_ItShouldReturnContextError(t *testing.T) {
	mock := &mockClient{
		getExecutionFn: func(_ context.Context, id string, _ string) (*client.Execution, error) {
			return &client.Execution{ID: id, Status: "running"}, nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	_, err := poll(ctx, mock, "exec-canceled", pollConfig{
		interval: 5 * time.Millisecond,
		timeout:  5 * time.Second,
	})
	if err == nil {
		t.Fatal("expected error on context cancel, got nil")
	}
}

func TestPoll_WhenGetExecutionFails_ItShouldReturnError(t *testing.T) {
	mock := &mockClient{
		getExecutionFn: func(_ context.Context, _ string, _ string) (*client.Execution, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}

	_, err := poll(context.Background(), mock, "exec-err", pollConfig{
		interval: 5 * time.Millisecond,
		timeout:  1 * time.Second,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestPoll_WhenFailed_ItShouldReturnLogsInsteadOfOutput(t *testing.T) {
	callCount := 0
	mock := &mockClient{
		getExecutionFn: func(_ context.Context, id string, include string) (*client.Execution, error) {
			callCount++
			if callCount == 1 {
				return &client.Execution{ID: id, Status: "failed"}, nil
			}
			if include != "logs" {
				t.Errorf("expected include='logs' for failed execution, got %q", include)
			}
			return &client.Execution{ID: id, Status: "failed", Logs: "error: something broke"}, nil
		},
	}

	exec, err := poll(context.Background(), mock, "exec-failed", pollConfig{
		interval: 5 * time.Millisecond,
		timeout:  1 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exec.Logs != "error: something broke" {
		t.Errorf("expected logs in result, got %q", exec.Logs)
	}
}

func TestPoll_WhenFailedLogsOmitOutputBytes_ItShouldPreserveFromListResponse(t *testing.T) {
	listBytes := int64(9771733)
	callCount := 0
	mock := &mockClient{
		getExecutionFn: func(_ context.Context, id string, include string) (*client.Execution, error) {
			callCount++
			if callCount == 1 {
				return &client.Execution{
					ID:          id,
					Status:      "failed",
					OutputBytes: &listBytes,
				}, nil
			}
			if include != "logs" {
				t.Errorf("expected include='logs' for failed execution, got %q", include)
			}
			return &client.Execution{
				ID:     id,
				Status: "failed",
				Logs:   "gather image collection failed",
			}, nil
		},
	}

	exec, err := poll(context.Background(), mock, "exec-partial-bytes", pollConfig{
		interval: 5 * time.Millisecond,
		timeout:  1 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exec.OutputBytes == nil {
		t.Fatal("expected OutputBytes from list response to be preserved")
	}
	if *exec.OutputBytes != listBytes {
		t.Errorf("expected OutputBytes=%d, got %d", listBytes, *exec.OutputBytes)
	}
	if exec.Logs != "gather image collection failed" {
		t.Errorf("expected logs from second get, got %q", exec.Logs)
	}
}

func TestPrintRunResult_WhenOutputIsDownloadHint_ItShouldAutoDownload(t *testing.T) {
	dur := int64(5000)
	body := []byte("tarball-bytes")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/trusted-actions/runs/exec-mg/output" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(body)
	}))
	defer server.Close()

	mock := &mockClient{
		rawGetFn: func(_ context.Context, path string) (*http.Response, error) {
			return http.Get(server.URL + path)
		},
	}
	global := newMockGlobalOpts(mock)

	exec := &client.Execution{
		ID:            "exec-mg",
		Status:        "succeeded",
		ExecutionMode: "async",
		OutputFormat:  "tar.gz",
		DurationMs:    &dur,
	}

	err := printRunResult(context.Background(), global, exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	outPath := "zoa-exec-mg-output.tar.gz"
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("expected downloaded artifact %q: %v", outPath, err)
	}
	t.Cleanup(func() { _ = os.Remove(outPath) })

	content, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(content, body) {
		t.Errorf("unexpected artifact content")
	}
}

func TestPrintRunResult_WhenFailedWithPartialOutput_ItShouldAutoDownload(t *testing.T) {
	dur := int64(120000)
	outBytes := int64(9771733)
	body := []byte("partial-tar")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/trusted-actions/runs/exec-partial/output" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(body)
	}))
	defer server.Close()

	mock := &mockClient{
		rawGetFn: func(_ context.Context, path string) (*http.Response, error) {
			return http.Get(server.URL + path)
		},
	}
	global := newMockGlobalOpts(mock)

	exec := &client.Execution{
		ID:            "exec-partial",
		Status:        "failed",
		ExecutionMode: "async",
		DurationMs:    &dur,
		OutputBytes:   &outBytes,
		Logs:          `{"msg":"gather image collection failed"}` + "\n",
	}

	err := printRunResult(context.Background(), global, exec)
	if err == nil {
		t.Fatal("expected error for failed execution")
	}

	outPath := "zoa-exec-partial-output.tar.gz"
	if _, statErr := os.Stat(outPath); statErr != nil {
		t.Fatalf("expected partial download at %q: %v", outPath, statErr)
	}
	t.Cleanup(func() { _ = os.Remove(outPath) })
}

func TestRunAction_WhenWaitCompletesWithDownloadHint_ItShouldAutoDownload(t *testing.T) {
	body := []byte("must-gather-tar")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/trusted-actions/runs/exec-wait-mg/output" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(body)
	}))
	defer server.Close()

	mock := &mockClient{
		dispatchFn: func(_ context.Context, _ string, _ *client.DispatchRequest) (*client.DispatchResponse, error) {
			return &client.DispatchResponse{
				ID:            "exec-wait-mg",
				Status:        "dispatched",
				TargetCluster: "mc-useast1-1",
				ExecutionMode: "async",
			}, nil
		},
		getExecutionFn: func(_ context.Context, id string, _ string) (*client.Execution, error) {
			return &client.Execution{
				ID:            id,
				Status:        "succeeded",
				ExecutionMode: "async",
				OutputFormat:  "tar.gz",
			}, nil
		},
		rawGetFn: func(_ context.Context, path string) (*http.Response, error) {
			return http.Get(server.URL + path)
		},
	}

	global := newMockGlobalOpts(mock)
	opts := &runOptions{
		reason: "ROSAENG-1234",
		ta: parambind.RunTAParams{
			ClusterID: "1600392f-9a94-4957-b672-eff8dc2be0bb",
			Gather:    "hcp",
		},
		wait:        true,
		waitTimeout: 1 * time.Second,
	}

	err := runAction(context.Background(), global, opts, "must_gather")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	outPath := "zoa-exec-wait-mg-output.tar.gz"
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("expected downloaded artifact %q: %v", outPath, err)
	}
	t.Cleanup(func() { _ = os.Remove(outPath) })
}
