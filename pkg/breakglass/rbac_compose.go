package breakglass

import "github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"

// kubeReadPlusWriteRBAC is full investigation read plus surgical workload mutations.
func kubeReadPlusWriteRBAC(targetType string) []actions.RBACRule {
	rules := kubeReadRBAC(targetType)
	return append(rules, kubeWriteMutationsRBAC(targetType)...)
}

// kubeElevatedRBAC is write (which includes read) plus secret read and kubectl exec.
func kubeElevatedRBAC(targetType string) []actions.RBACRule {
	rules := kubeReadPlusWriteRBAC(targetType)
	rules = append(rules,
		actions.RBACRule{
			APIGroups: []string{""},
			Resources: []string{"secrets"},
			Verbs:     []string{"get", "list", "watch"},
		},
		actions.RBACRule{
			APIGroups: []string{""},
			Resources: []string{"pods/exec"},
			Verbs:     []string{"create"},
		},
	)
	return rules
}

// EffectiveRBAC returns the rules materialized for a target (rc or mc).
func EffectiveRBAC(def *Definition, targetType string) []actions.RBACRule {
	if def == nil || def.RBAC == nil {
		return nil
	}
	return def.RBAC(targetType)
}
