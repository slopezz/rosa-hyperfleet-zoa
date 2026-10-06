package main

import (
	"io"
	"log/slog"
	"testing"

	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/actions"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestParseParams_WhenEmpty_ItShouldReturnEmptyMap(t *testing.T) {
	params := parseParams("", testLogger())
	if len(params) != 0 {
		t.Fatalf("expected empty params, got %v", params)
	}
}

func TestParseParams_WhenValidJSON_ItShouldUnmarshal(t *testing.T) {
	params := parseParams(`{"cluster_id":"abc","gather":"mc"}`, testLogger())
	if params["cluster_id"] != "abc" {
		t.Errorf("expected cluster_id=abc, got %q", params["cluster_id"])
	}
	if params["gather"] != "mc" {
		t.Errorf("expected gather=mc, got %q", params["gather"])
	}
}

func TestParseParams_WhenInvalidJSON_ItShouldReturnEmptyMap(t *testing.T) {
	params := parseParams("{not-json", testLogger())
	if len(params) != 0 {
		t.Fatalf("expected empty params on parse error, got %v", params)
	}
}

func TestParseParamsWithApplyDefaults_WhenMustGatherPartialParams_ItShouldFillNonGatherDefaults(t *testing.T) {
	t.Setenv("ZOA_TARGET_TYPE", "mc")
	actions.SetDeploymentTarget("mc")

	action, ok := actions.GetCatalog("must_gather")
	if !ok {
		t.Fatal("must_gather action not registered")
	}

	params := parseParams(`{"gather":"hcp","cluster_id":"1600392f-9a94-4957-b672-eff8dc2be0bb"}`, testLogger())
	actions.ApplyDefaults(action.Metadata(), params)

	if params["skip_must_gather_image"] != "false" {
		t.Errorf("expected skip_must_gather_image=false after ApplyDefaults, got %q", params["skip_must_gather_image"])
	}
}
