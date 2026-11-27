package cli

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
	"github.com/openshift-online/rosa-seaplane-zoa/internal/output"
)

func TestFormatParams_WhenEmpty_ItShouldReturnDash(t *testing.T) {
	result := formatParams(nil)
	if result != "-" {
		t.Errorf("expected '-', got %q", result)
	}

	result = formatParams(map[string]string{})
	if result != "-" {
		t.Errorf("expected '-' for empty map, got %q", result)
	}
}

func TestFormatParams_WhenSingleParam_ItShouldFormatAsKeyValue(t *testing.T) {
	result := formatParams(map[string]string{"namespace": "grafana"})
	if result != "[namespace=grafana]" {
		t.Errorf("expected '[namespace=grafana]', got %q", result)
	}
}

func TestFormatParams_WhenMultipleParams_ItShouldIncludeAll(t *testing.T) {
	params := map[string]string{"namespace": "grafana", "resource": "pods"}
	result := formatParams(params)
	if result != "[namespace=grafana resource=pods]" && result != "[resource=pods namespace=grafana]" {
		t.Errorf("unexpected format: %q", result)
	}
}

func TestListRuns_WhenExecutionsExist_ItShouldReturnNilError(t *testing.T) {
	now := time.Now()
	dur := int64(2500)
	mock := &mockClient{
		listExecutionsFn: func(_ context.Context, query url.Values) (*client.ExecutionList, error) {
			if query.Get("limit") != "20" {
				t.Errorf("expected limit=20, got %q", query.Get("limit"))
			}
			return &client.ExecutionList{
				Items: []client.Execution{
					{
						ID:            "exec-run-1",
						Action:        "get_pods",
						TargetCluster: "mc-useast1-1",
						Status:        "succeeded",
						Scope:         "kube",
						Type:          "read",
						ExecutionMode: "sync",
						Operator:      "arn:aws:iam::123456:user/slopezma",
						CreatedAt:     &now,
						DurationMs:    &dur,
						Params:        map[string]string{"namespace": "grafana"},
					},
				},
				Count: 1,
			}, nil
		},
	}

	global := newMockGlobalOpts(mock)
	opts := &runsOptions{limit: 20}
	err := listRuns(context.Background(), global, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListRuns_WhenNoExecutions_ItShouldReturnNilError(t *testing.T) {
	mock := &mockClient{
		listExecutionsFn: func(_ context.Context, _ url.Values) (*client.ExecutionList, error) {
			return &client.ExecutionList{Items: []client.Execution{}}, nil
		},
	}

	global := newMockGlobalOpts(mock)
	opts := &runsOptions{limit: 20}
	err := listRuns(context.Background(), global, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListRuns_WhenFiltersProvided_ItShouldPassQueryParameters(t *testing.T) {
	mock := &mockClient{
		listExecutionsFn: func(_ context.Context, query url.Values) (*client.ExecutionList, error) {
			if query.Get("target") != "mc-useast1-1" {
				t.Errorf("expected target=mc-useast1-1, got %q", query.Get("target"))
			}
			if query.Get("status") != "failed" {
				t.Errorf("expected status=failed, got %q", query.Get("status"))
			}
			if query.Get("action") != "rollout_restart" {
				t.Errorf("expected action=rollout_restart, got %q", query.Get("action"))
			}
			if query.Get("scope") != "kube" {
				t.Errorf("expected scope=kube, got %q", query.Get("scope"))
			}
			if query.Get("type") != "write" {
				t.Errorf("expected type=write, got %q", query.Get("type"))
			}
			if query.Get("execution_mode") != "async" {
				t.Errorf("expected execution_mode=async, got %q", query.Get("execution_mode"))
			}
			if query.Get("dry_run") != "true" {
				t.Errorf("expected dry_run=true, got %q", query.Get("dry_run"))
			}
			if query.Get("since") != "1h" {
				t.Errorf("expected since=1h, got %q", query.Get("since"))
			}
			return &client.ExecutionList{Items: []client.Execution{}}, nil
		},
	}

	global := newMockGlobalOpts(mock)
	opts := &runsOptions{
		target:        "mc-useast1-1",
		status:        "failed",
		action:        "rollout_restart",
		scope:         "kube",
		actionType:    "write",
		executionMode: "async",
		dryRun:        true,
		since:         "1h",
		limit:         20,
	}
	err := listRuns(context.Background(), global, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListRuns_WhenJSONFormat_ItShouldReturnNilError(t *testing.T) {
	now := time.Now()
	mock := &mockClient{
		listExecutionsFn: func(_ context.Context, _ url.Values) (*client.ExecutionList, error) {
			return &client.ExecutionList{
				Items: []client.Execution{
					{ID: "exec-1", Status: "succeeded", CreatedAt: &now},
				},
			}, nil
		},
	}

	global := newMockGlobalOpts(mock)
	global.OutputFormat = output.FormatJSON
	opts := &runsOptions{limit: 20}
	err := listRuns(context.Background(), global, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListRuns_WhenClientReturnsError_ItShouldPropagateError(t *testing.T) {
	mock := &mockClient{
		listExecutionsFn: func(_ context.Context, _ url.Values) (*client.ExecutionList, error) {
			return nil, &client.APIError{Code: "forbidden", Reason: "access denied"}
		},
	}

	global := newMockGlobalOpts(mock)
	opts := &runsOptions{limit: 20}
	err := listRuns(context.Background(), global, opts)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
