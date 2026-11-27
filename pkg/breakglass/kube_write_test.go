package breakglass

import (
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"
)

func TestKubeWriteScope_WhenRegistered_ItShouldGrantWorkloadMutate(t *testing.T) {
	def, ok := Get("kube_write")
	if !ok {
		t.Fatal("kube_write not registered")
	}
	if def.Metadata.Type != TypeWrite {
		t.Fatalf("expected type write, got %s", def.Metadata.Type)
	}
	rules := def.RBAC(actions.TargetTypeMC)
	if !rulesIncludeVerbOnResource(rules, "apps", "deployments", "patch") {
		t.Fatal("expected apps/deployments patch")
	}
	if rulesIncludeVerbOnResource(rules, "apps", "deployments", "delete") {
		t.Fatal("kube_write must not delete deployments")
	}
	if err := ValidateProfile(def); err != nil {
		t.Fatal(err)
	}
}

func rulesIncludeVerbOnResource(rules []actions.RBACRule, group, resource, verb string) bool {
	for _, rule := range rules {
		if !sliceContains(rule.APIGroups, group) {
			continue
		}
		if !sliceContains(rule.Resources, resource) && !sliceContains(rule.Resources, "*") {
			continue
		}
		if sliceContains(rule.Verbs, verb) {
			return true
		}
	}
	return false
}

func sliceContains(ss []string, v string) bool {
	for _, s := range ss {
		if s == v {
			return true
		}
	}
	return false
}
