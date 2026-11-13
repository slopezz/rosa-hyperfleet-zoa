package breakglass

import "github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"

func init() {
	register(&Definition{
		Metadata: ScopeMetadata{
			ID:          "kube-elevated",
			Plane:       PlaneKube,
			Type:        TypeElevated,
			Description: "kube-write plus secret read and kubectl exec",
			Capabilities: []string{
				"Includes all kube-write RBAC (which includes kube-read)",
				"Adds secrets get/list/watch and pods/exec",
			},
			Authorization: Authorization{
				Approval:            "none",
				DefaultGrantHours:   1,
				DefaultPendingHours: 8,
			},
			Restrictions: []string{
				"Secrets read-only; no RBAC write; no HostedCluster delete (not in RBAC)",
			},
			TargetTypes: []string{actions.TargetTypeRC, actions.TargetTypeMC},
		},
		RBAC: kubeElevatedRBAC,
	})
}
