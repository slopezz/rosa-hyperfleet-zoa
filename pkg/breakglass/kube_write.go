package breakglass

import "github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"

func init() {
	register(&Definition{
		Metadata: ScopeMetadata{
			ID:          "kube-write",
			Plane:       PlaneKube,
			Type:        TypeWrite,
			Description: "Full kube-read plus workload mutate (controllers: create/patch/update; pods, configmaps, jobs: full mutate)",
			Capabilities: []string{
				"Includes all kube-read RBAC",
				"Apps controllers: create/patch/update (no delete); full mutate on pods, configmaps, jobs",
			},
			Authorization: Authorization{
				Approval:            "none",
				DefaultGrantHours:   4,
				DefaultRequestHours: 24,
			},
			Restrictions: []string{
				"No secrets, pods/exec, RBAC bind writes, or platform CR mutate (use kube-elevated)",
			},
			TargetTypes: []string{actions.TargetTypeRC, actions.TargetTypeMC},
		},
		RBAC: kubeWriteRBAC,
	})
}

func kubeWriteRBAC(targetType string) []actions.RBACRule {
	return append(kubeReadRBAC(targetType), kubeWriteMutationsRBAC(targetType)...)
}

func kubeWriteMutationsRBAC(string) []actions.RBACRule {
	return []actions.RBACRule{
		{
			APIGroups: []string{"apps"},
			Resources: []string{"deployments", "daemonsets", "statefulsets", "replicasets"},
			Verbs:     verbsMutate,
		},
		{
			APIGroups: []string{"batch"},
			Resources: []string{"jobs"},
			Verbs:     verbsMutateFull,
		},
		{
			APIGroups: []string{""},
			Resources: []string{"pods", "configmaps"},
			Verbs:     verbsMutateFull,
		},
	}
}
