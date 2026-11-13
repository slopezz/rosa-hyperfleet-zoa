package breakglass

import "github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"

func init() {
	register(&Definition{
		Metadata: ScopeMetadata{
			ID:          "kube-write",
			Plane:       PlaneKube,
			Type:        TypeWrite,
			Description: "Full kube-read plus surgical workload changes (patch/update; delete pods and jobs)",
			Capabilities: []string{
				"Includes all kube-read RBAC",
				"Patch/update deployments, statefulsets, daemonsets, pods, configmaps; delete pods and jobs",
			},
			Authorization: Authorization{
				Approval:            "none",
				DefaultGrantHours:   4,
				DefaultPendingHours: 24,
			},
			Restrictions: []string{
				"No secrets, RBAC bind changes, or hosted cluster delete",
			},
			TargetTypes: []string{actions.TargetTypeRC, actions.TargetTypeMC},
		},
		RBAC: kubeReadPlusWriteRBAC,
	})
}

// kubeWriteMutationsRBAC is the write delta (no cluster-wide read).
func kubeWriteMutationsRBAC(string) []actions.RBACRule {
	return []actions.RBACRule{
		{
			APIGroups: []string{"apps"},
			Resources: []string{"deployments", "daemonsets", "statefulsets", "replicasets"},
			Verbs:     []string{"get", "list", "patch", "update"},
		},
		{
			APIGroups: []string{"batch"},
			Resources: []string{"jobs"},
			Verbs:     []string{"get", "list", "patch", "update", "delete"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"pods", "configmaps"},
			Verbs:     []string{"get", "list", "patch", "update", "delete"},
		},
	}
}
