package breakglass

import (
	"slices"
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"
)

func TestKubeElevatedScope_WhenRegistered_ItShouldIncludePlatformMutateOnMC(t *testing.T) {
	def, ok := Get("kube_elevated")
	if !ok {
		t.Fatal("kube_elevated not registered")
	}
	rules := def.RBAC(actions.TargetTypeMC)
	if !rulesIncludePlatformMutate(rules, "hypershift.openshift.io") {
		t.Fatal("expected platform elevated mutate rule for hypershift.openshift.io on MC")
	}
}

func rulesIncludePlatformMutate(rules []actions.RBACRule, apiGroup string) bool {
	for _, rule := range rules {
		if !slices.Contains(rule.APIGroups, apiGroup) {
			continue
		}
		if !slices.Contains(rule.Resources, "*") {
			continue
		}
		if slices.Contains(rule.Verbs, "patch") && slices.Contains(rule.Verbs, "create") {
			return true
		}
	}
	return false
}

func TestKubeElevatedScope_WhenRegistered_ItShouldAllowNodeCordonAndNodeClaimReplace(t *testing.T) {
	def, ok := Get("kube_elevated")
	if !ok {
		t.Fatal("kube_elevated not registered")
	}
	rules := def.RBAC(actions.TargetTypeMC)
	var nodeDelete, nodeClaimDelete bool
	for _, rule := range rules {
		if slices.Contains(rule.APIGroups, "") && slices.Contains(rule.Resources, "nodes") {
			if slices.Contains(rule.Verbs, "delete") {
				nodeDelete = true
			}
		}
		if slices.Contains(rule.APIGroups, "karpenter.sh") && slices.Contains(rule.Resources, "nodeclaims") {
			if slices.Contains(rule.Verbs, "delete") {
				nodeClaimDelete = true
			}
		}
	}
	if !nodeDelete {
		t.Fatal("elevated must allow delete on nodes")
	}
	if !nodeClaimDelete {
		t.Fatal("elevated must allow delete on karpenter nodeclaims")
	}
}

func TestKubeElevatedScope_WhenRegistered_ItShouldNotGrantDeleteOnPlatformStar(t *testing.T) {
	def, ok := Get("kube_elevated")
	if !ok {
		t.Fatal("kube_elevated not registered")
	}
	for _, target := range []string{actions.TargetTypeRC, actions.TargetTypeMC} {
		for _, rule := range def.RBAC(target) {
			if slices.Contains(rule.Resources, "*") && slices.Contains(rule.Verbs, "delete") {
				t.Fatalf("elevated must not use delete with resources * (%s)", target)
			}
		}
	}
}

func TestKubeElevatedScope_WhenRegistered_ItShouldNotGrantDeleteOnCRDs(t *testing.T) {
	def, ok := Get("kube_elevated")
	if !ok {
		t.Fatal("kube_elevated not registered")
	}
	for _, target := range []string{actions.TargetTypeRC, actions.TargetTypeMC} {
		rules := def.RBAC(target)
		for _, rule := range rules {
			if slices.Contains(rule.Resources, "customresourcedefinitions") && slices.Contains(rule.Verbs, "delete") {
				t.Fatalf("elevated must not delete CRDs (%s)", target)
			}
		}
	}
}

func TestKubeElevatedScope_WhenRegistered_ItShouldAllowExecWithinPolicy(t *testing.T) {
	def, ok := Get("kube_elevated")
	if !ok {
		t.Fatal("kube_elevated not registered")
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
