package breakglass

import (
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"
)

func TestKubeWriteScope_WhenRegistered_ItShouldIncludeReadRBAC(t *testing.T) {
	def, ok := Get("kube-write")
	if !ok {
		t.Fatal("kube-write not registered")
	}
	readRules := kubeReadRBAC(actions.TargetTypeMC)
	writeRules := def.RBAC(actions.TargetTypeMC)
	if len(writeRules) <= len(readRules) {
		t.Fatalf("write scope should include read rules: got %d rules, read alone is %d", len(writeRules), len(readRules))
	}
}

func TestKubeWriteScope_WhenRegistered_ItShouldValidateAndDisallowExec(t *testing.T) {
	def, ok := Get("kube-write")
	if !ok {
		t.Fatal("kube-write not registered")
	}
	if def.Metadata.Type != TypeWrite {
		t.Fatalf("expected type write, got %s", def.Metadata.Type)
	}
	if err := ValidateScope(def); err != nil {
		t.Fatal(err)
	}
}
