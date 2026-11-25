package breakglass

import "github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"

func init() {
	register(&Definition{
		Metadata: ScopeMetadata{
			ID:          "kube-elevated",
			Plane:       PlaneKube,
			Type:        TypeElevated,
			Description: "Near-admin kube: write plus platform/storage/node mutate, secrets read, and exec",
			Capabilities: []string{
				"Includes all kube-write RBAC (which includes kube-read)",
				"Platform operator apiGroups (kube_rbac.go): create/patch/update on * per RC/MC — no delete on those CRs",
				"Core delta: verbsMutateFull on read-tier core/apps/batch/networking/discovery/coordination plus PVC/PV, nodes, karpenter NodeClaims (delete allowed)",
				"Secrets get/list/watch and pods/exec create",
			},
			Authorization: Authorization{
				Approval:            "none",
				DefaultGrantHours:   1,
				DefaultRequestHours: 8,
			},
			Restrictions: []string{
				"No RBAC bind writes, admission webhook / CRD delete, namespace delete, or secret writes",
				"Platform operator CRs (kube_rbac.go): no delete via resources * (HostedCluster and similar)",
			},
			TargetTypes: []string{actions.TargetTypeRC, actions.TargetTypeMC},
		},
		RBAC: kubeElevatedRBAC,
	})
}

func kubeElevatedRBAC(targetType string) []actions.RBACRule {
	return append(kubeWriteRBAC(targetType), kubeElevatedMutationsRBAC(targetType)...)
}

func kubeElevatedMutationsRBAC(targetType string) []actions.RBACRule {
	rules := kubeElevatedCoreMutationsRBAC()
	// Platform operator CRDs: rcPlatformAPIGroups / mcPlatformAPIGroups in kube_rbac.go.
	if targetType == actions.TargetTypeRC {
		rules = append(rules, platformOperatorElevatedRule(rcPlatformAPIGroups))
	}
	if targetType == actions.TargetTypeMC {
		rules = append(rules, platformOperatorElevatedRule(mcPlatformAPIGroups))
	}
	return append(rules, kubeElevatedPrivilegeRBAC()...)
}

func kubeElevatedCoreMutationsRBAC() []actions.RBACRule {
	return []actions.RBACRule{
		{
			APIGroups: []string{""},
			Resources: []string{
				"configmaps", "endpoints", "pods", "services",
				"persistentvolumeclaims", "persistentvolumes",
				"replicationcontrollers", "resourcequotas", "limitranges",
			},
			Verbs: verbsMutateFull,
		},
		{
			APIGroups: []string{"apps"},
			Resources: []string{
				"deployments", "replicasets", "statefulsets", "daemonsets",
				"controllerrevisions",
			},
			Verbs: verbsMutateFull,
		},
		{
			APIGroups: []string{"batch"},
			Resources: []string{"jobs", "cronjobs"},
			Verbs:     verbsMutateFull,
		},
		{
			APIGroups: []string{"autoscaling"},
			Resources: []string{"horizontalpodautoscalers"},
			Verbs:     verbsMutateFull,
		},
		{
			APIGroups: []string{"policy"},
			Resources: []string{"poddisruptionbudgets"},
			Verbs:     verbsMutateFull,
		},
		{
			APIGroups: []string{"networking.k8s.io"},
			Resources: []string{"ingresses", "ingressclasses", "networkpolicies"},
			Verbs:     verbsMutateFull,
		},
		{
			APIGroups: []string{"discovery.k8s.io"},
			Resources: []string{"endpointslices"},
			Verbs:     verbsMutateFull,
		},
		{
			APIGroups: []string{"storage.k8s.io"},
			Resources: []string{"volumeattachments"},
			Verbs:     verbsMutateFull,
		},
		{
			APIGroups: []string{"coordination.k8s.io"},
			Resources: []string{"leases"},
			Verbs:     verbsMutateFull,
		},
		{
			APIGroups: []string{""},
			Resources: []string{"nodes"},
			Verbs:     verbsMutateFull,
		},
		{
			APIGroups: []string{"karpenter.sh"},
			Resources: []string{"nodeclaims"},
			Verbs:     verbsMutateFull, // replace stuck nodes (NodePool stays verbsMutate via platformOperatorElevatedRule)
		},
	}
}

func kubeElevatedPrivilegeRBAC() []actions.RBACRule {
	return []actions.RBACRule{
		{
			APIGroups: []string{""},
			Resources: []string{"secrets"},
			Verbs:     verbsRead,
		},
		{
			APIGroups: []string{""},
			Resources: []string{"pods/exec"},
			Verbs:     []string{"create"},
		},
	}
}
