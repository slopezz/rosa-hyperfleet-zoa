package parambind

import (
	"testing"

	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/actions"
)

func TestTAParamBindings_WhenDefined_ItShouldHaveUniqueAPIAndFlagNames(t *testing.T) {
	seenAPI := make(map[string]string)
	seenFlag := make(map[string]string)
	for _, b := range TAParamBindings() {
		if b.APIParam == "" || b.FlagName == "" || b.DescribeCLI == "" || b.ShortHelp == "" {
			t.Fatalf("incomplete binding: %+v", b)
		}
		if prev, ok := seenAPI[b.APIParam]; ok {
			t.Fatalf("duplicate api param %q (%s and %s)", b.APIParam, prev, b.FlagName)
		}
		seenAPI[b.APIParam] = b.FlagName
		if prev, ok := seenFlag[b.FlagName]; ok {
			t.Fatalf("duplicate flag %q (%s and %s)", b.FlagName, prev, b.APIParam)
		}
		seenFlag[b.FlagName] = b.APIParam
		if CLILineForParam(b.APIParam) != b.DescribeCLI {
			t.Fatalf("CLILineForParam(%q) = %q, want %q", b.APIParam, CLILineForParam(b.APIParam), b.DescribeCLI)
		}
		if ParamFlagHelp(b.FlagName) != b.ShortHelp {
			t.Fatalf("ParamFlagHelp(%q) = %q, want %q", b.FlagName, ParamFlagHelp(b.FlagName), b.ShortHelp)
		}
	}
}

func TestToAPIParams_WhenDedicatedFlagsSet_ItShouldMapAPIKeys(t *testing.T) {
	got := ToAPIParams(RunTAParams{
		Namespace:     "openshift-ingress",
		Resource:      "pods",
		AllNamespaces: true,
		Verbose:       true,
		LabelSelector: "app=nginx",
	}, nil)
	want := map[string]string{
		"namespace":      "openshift-ingress",
		"resource":       "pods",
		"all_namespaces": "true",
		"verbose":        "true",
		"label_selector": "app=nginx",
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("param %q: got %q want %q (full map %v)", k, got[k], v, got)
		}
	}
}

func TestToAPIParams_WhenExtraParamProvided_ItShouldIncludeViaParam(t *testing.T) {
	got := ToAPIParams(RunTAParams{}, []string{"field_selector=involvedObject.name=my-pod"})
	if got["field_selector"] != "involvedObject.name=my-pod" {
		t.Fatalf("expected field_selector via --param, got %v", got)
	}
}

func TestToAPIParams_WhenDedicatedAndExtraConflict_ItShouldPreferDedicated(t *testing.T) {
	got := ToAPIParams(RunTAParams{Namespace: "a"}, []string{"namespace=b"})
	if got["namespace"] != "a" {
		t.Fatalf("dedicated flag should win over --param, got %v", got)
	}
}

func TestTAParamBindings_WhenComparedToActionCatalog_ItShouldCoverCommonDedicatedParams(t *testing.T) {
	dedicated := make(map[string]bool)
	for _, b := range TAParamBindings() {
		dedicated[b.APIParam] = true
	}

	// Params that intentionally use --param only (no first-class flag). Update when adding TA params.
	viaParamOnly := map[string]bool{
		"field_selector":         true,
		"extra_namespaces":       true,
		"skip_must_gather_image": true,
	}

	allCatalogParams := make(map[string]bool)
	for _, a := range actions.ListCatalog() {
		for _, p := range a.Metadata().Parameters {
			allCatalogParams[p.Name] = true
		}
	}

	for name := range allCatalogParams {
		if viaParamOnly[name] {
			continue
		}
		if !dedicated[name] {
			t.Errorf("catalog parameter %q has no dedicated zoa run flag — add to taParamBindings or viaParamOnly", name)
		}
	}
}
