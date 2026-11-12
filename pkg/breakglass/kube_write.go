package breakglass

import "github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"

func init() {
	register(&Definition{
		Metadata: ScopeMetadata{
			ID:          "kube-write",
			Plane:       PlaneKube,
			Type:        TypeWrite,
			Description: "Surgical workload changes (patch/update; delete pods and jobs only)",
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
		RBAC: kubeWriteRBAC,
	})
}

func kubeWriteRBAC(string) []actions.RBACRule {
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
