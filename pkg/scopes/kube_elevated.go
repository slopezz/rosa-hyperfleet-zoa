package scopes

import "github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"

func init() {
	register(&Definition{
		Metadata: ScopeMetadata{
			ID:          "kube-elevated",
			Plane:       PlaneKube,
			Type:        TypeElevated,
			Description: "Near-admin operational access without secrets or RBAC escalation",
			Authorization: Authorization{
				Approval:            "strict",
				DefaultGrantHours:   1,
				DefaultPendingHours: 8,
				MinApprovers:        1,
				RequireDecisionNote: true,
			},
			Restrictions: []string{
				"No secrets, serviceaccounts, or RBAC bind mutations",
			},
			TargetTypes: []string{actions.TargetTypeRC, actions.TargetTypeMC},
		},
		RBAC: func(targetType string) []actions.RBACRule {
			rules := kubeReadRBAC(targetType)
			rules = append(rules, kubeWriteRBAC(targetType)...)
			rules = append(rules, actions.RBACRule{
				APIGroups: []string{""},
				Resources: []string{"pods/exec"},
				Verbs:     []string{"create"},
			})
			return rules
		},
	})
}
