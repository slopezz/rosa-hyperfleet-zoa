package breakglass

import (
	"slices"
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"
)

func TestKubeElevatedScope_WhenRegistered_ItShouldAllowExecWithinPolicy(t *testing.T) {
	def, ok := Get("kube-elevated")
	if !ok {
		t.Fatal("kube-elevated not registered")
	}
	if def.Metadata.Type != TypeElevated {
		t.Fatalf("expected type elevated, got %s", def.Metadata.Type)
	}
	rules := def.RBAC(actions.TargetTypeMC)
	var hasSecrets, hasExec bool
	for _, rule := range rules {
		if slices.Contains(rule.Resources, "secrets") {
			hasSecrets = true
		}
		if ruleAllowsPodsExec(rule) {
			hasExec = true
		}
	}
	if !hasSecrets || !hasExec {
		t.Fatal("elevated must include secrets read and pods/exec")
	}
	if err := ValidateScope(def); err != nil {
		t.Fatal(err)
	}
}
