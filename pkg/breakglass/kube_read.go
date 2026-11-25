package breakglass

import (
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"
)

func init() {
	register(&Definition{
		Metadata: ScopeMetadata{
			ID:          "kube-read",
			Plane:       PlaneKube,
			Type:        TypeRead,
			Description: "Read-only kubectl (get/list/watch); no secrets or exec",
			Authorization: Authorization{
				Approval:            "none",
				DefaultGrantHours:   4,
				DefaultRequestHours: 24,
			},
			Capabilities: []string{
				"Cluster-wide get/list/watch: core workloads, networking, storage, policy/HPA, admission webhooks, CRD catalog",
				"Platform operator apiGroups (kube_rbac.go): get/list/watch * per RC or MC",
			},
			Restrictions: []string{
				"No secrets or pods/exec (serviceaccount objects are metadata-only read)",
			},
			TargetTypes: []string{actions.TargetTypeRC, actions.TargetTypeMC},
		},
		RBAC: kubeReadRBAC,
	})
}

func kubeReadRBAC(targetType string) []actions.RBACRule {
	rules := []actions.RBACRule{
		{
			APIGroups: []string{""},
			Resources: []string{
				"configmaps", "endpoints", "events", "namespaces", "nodes",
				"pods", "pods/log", "pods/status", "services", "serviceaccounts",
				"persistentvolumeclaims", "persistentvolumes",
				"replicationcontrollers", "resourcequotas", "limitranges",
			},
			Verbs: verbsRead,
		},
		{
			APIGroups: []string{"apps"},
			Resources: []string{
				"deployments", "replicasets", "statefulsets", "daemonsets",
				"controllerrevisions",
			},
			Verbs: verbsRead,
		},
		{
			APIGroups: []string{"batch"},
			Resources: []string{"jobs", "cronjobs"},
			Verbs:     verbsRead,
		},
		{
			APIGroups: []string{"autoscaling"},
			Resources: []string{"horizontalpodautoscalers"},
			Verbs:     verbsRead,
		},
		{
			APIGroups: []string{"policy"},
			Resources: []string{"poddisruptionbudgets"},
			Verbs:     verbsRead,
		},
		{
			APIGroups: []string{"networking.k8s.io"},
			Resources: []string{"ingresses", "ingressclasses", "networkpolicies"},
			Verbs:     verbsRead,
		},
		{
			APIGroups: []string{"discovery.k8s.io"},
			Resources: []string{"endpointslices"},
			Verbs:     verbsRead,
		},
		{
			APIGroups: []string{"storage.k8s.io"},
			Resources: []string{"storageclasses", "volumeattachments", "csidrivers", "csinodes", "csistoragecapacities"},
			Verbs:     verbsRead,
		},
		{
			APIGroups: []string{"scheduling.k8s.io"},
			Resources: []string{"priorityclasses"},
			Verbs:     verbsRead,
		},
		{
			APIGroups: []string{"coordination.k8s.io"},
			Resources: []string{"leases"},
			Verbs:     verbsRead,
		},
		{
			APIGroups: []string{"apiextensions.k8s.io"},
			Resources: []string{"customresourcedefinitions"},
			Verbs:     verbsRead,
		},
		{
			APIGroups: []string{"rbac.authorization.k8s.io"},
			Resources: []string{"roles", "clusterroles", "rolebindings", "clusterrolebindings"},
			Verbs:     verbsRead,
		},
		{
			APIGroups: []string{"certificates.k8s.io"},
			Resources: []string{"certificatesigningrequests"},
			Verbs:     verbsRead,
		},
		{
			APIGroups: []string{"admissionregistration.k8s.io"},
			Resources: []string{
				"mutatingwebhookconfigurations", "validatingwebhookconfigurations",
				"validatingadmissionpolicies", "validatingadmissionpolicybindings",
			},
			Verbs: verbsRead,
		},
	}
	// Platform operator CRDs: rcPlatformAPIGroups / mcPlatformAPIGroups in kube_rbac.go.
	// Here: get/list/watch on * per group. Elevated reuses the same lists with verbsMutate.
	if targetType == actions.TargetTypeRC {
		rules = append(rules, platformOperatorReadRule(rcPlatformAPIGroups))
	}
	if targetType == actions.TargetTypeMC {
		rules = append(rules, platformOperatorReadRule(mcPlatformAPIGroups))
	}
	return rules
}
