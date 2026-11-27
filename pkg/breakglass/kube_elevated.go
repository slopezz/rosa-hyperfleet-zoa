// kube_elevated break-glass profile: full RBAC in kubeElevatedRBAC below.
//
// Verbs: kube_rbac.go only. APIGroups and resources are explicit in this file.
package breakglass

import "github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"

func init() {
	register(&Definition{
		Metadata: ProfileMetadata{
			Name:        "kube_elevated",
			Scope:       ScopeKube,
			Type:        TypeElevated,
			Description: "Near-admin kube: broad workload mutate, platform operator CR patch (no platform * delete), secrets read, and exec",
			Capabilities: []string{
				"Full kube_write coverage plus elevated workload and platform mutate",
				"Platform operator apiGroups: create/patch/update on * per RC/MC — no delete on those CRs",
				"Core elevated mutate (incl. nodes, PVC/PV, NodeClaims delete); secrets read and pods/exec",
			},
			Authorization: Authorization{
				Approval:            "none",
				DefaultGrantHours:   1,
				DefaultRequestHours: 8,
			},
			Restrictions: []string{
				"No RBAC bind writes, admission webhook / CRD delete, namespace delete, or secret writes",
				"Platform operator CRs: no delete via resources * (HostedCluster and similar)",
			},
			TargetTypes: []string{actions.TargetTypeRC, actions.TargetTypeMC},
			Examples: []string{
				"zoa breakglass request kube_elevated --reason ROSAENG-1234",
				"zoa breakglass request kube_elevated --reason ROSAENG-1234 --wait",
				"zoa breakglass request kube_elevated --reason ROSAENG-1234 --wait --wait-timeout 10m --wait-poll-interval 1m",
			},
		},
		RBAC: kubeElevatedRBAC,
	})
}

// kubeElevatedRBAC is the complete kube_elevated ClusterRole policy for rc or mc.
func kubeElevatedRBAC(targetType string) []actions.RBACRule {
	rules := []actions.RBACRule{
		{
			APIGroups: []string{""},
			Resources: []string{
				"events", "namespaces", "pods/log", "pods/status", "serviceaccounts",
			},
			Verbs: verbsRead,
		},
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
			APIGroups: []string{""},
			Resources: []string{"nodes"},
			Verbs:     verbsMutateFull,
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
			Resources: []string{"storageclasses", "csidrivers", "csinodes", "csistoragecapacities"},
			Verbs:     verbsRead,
		},
		{
			APIGroups: []string{"storage.k8s.io"},
			Resources: []string{"volumeattachments"},
			Verbs:     verbsMutateFull,
		},
		{
			APIGroups: []string{"scheduling.k8s.io"},
			Resources: []string{"priorityclasses"},
			Verbs:     verbsRead,
		},
		{
			APIGroups: []string{"coordination.k8s.io"},
			Resources: []string{"leases"},
			Verbs:     verbsMutateFull,
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
		{
			APIGroups: []string{"karpenter.sh"},
			Resources: []string{"nodeclaims"},
			Verbs:     verbsMutateFull,
		},
		{
			APIGroups: []string{""},
			Resources: []string{"secrets"},
			Verbs:     verbsRead,
		},
		{
			APIGroups: []string{""},
			Resources: []string{"pods/exec"},
			Verbs:     verbsExecCreate,
		},
	}
	switch targetType {
	case actions.TargetTypeRC:
		rules = append(rules, actions.RBACRule{
			APIGroups: kubeElevatedPlatformRCAPIGroups,
			Resources: []string{"*"},
			Verbs:     verbsMutate,
		})
	case actions.TargetTypeMC:
		rules = append(rules, actions.RBACRule{
			APIGroups: kubeElevatedPlatformMCAPIGroups,
			Resources: []string{"*"},
			Verbs:     verbsMutate,
		})
	}
	return rules
}

// Platform operator apiGroups (GitOps + EKS addons). Keep aligned with kube_read / kube_write lists.
var kubeElevatedPlatformRCAPIGroups = []string{
	"seaplane.io",
	"argoproj.io",
	"monitoring.coreos.com",
	"monitoring.thanos.io",
	"cert-manager.io",
	"external-secrets.io",
	"elbv2.k8s.aws",
	"karpenter.sh",
	"karpenter.k8s.aws",
	"acme.cert-manager.io",
	"generators.external-secrets.io",
	"gateway.k8s.aws",
	"networking.k8s.aws",
	"vpcresources.k8s.aws",
	"crd.k8s.amazonaws.com",
	"secrets-store.csi.x-k8s.io",
	"autoscaling.x-k8s.io",
	"aga.k8s.aws",
}

var kubeElevatedPlatformMCAPIGroups = []string{
	"hypershift.openshift.io",
	"argoproj.io",
	"monitoring.coreos.com",
	"cert-manager.io",
	"external-secrets.io",
	"karpenter.sh",
	"karpenter.k8s.aws",
	"route.openshift.io",
	"cluster.x-k8s.io",
	"infrastructure.cluster.x-k8s.io",
	"addons.cluster.x-k8s.io",
	"ipam.cluster.x-k8s.io",
	"auditlogpersistence.hypershift.openshift.io",
	"certificates.hypershift.openshift.io",
	"scheduling.hypershift.openshift.io",
	"acme.cert-manager.io",
	"generators.external-secrets.io",
	"gateway.k8s.aws",
	"networking.k8s.aws",
	"vpcresources.k8s.aws",
	"crd.k8s.amazonaws.com",
	"secrets-store.csi.x-k8s.io",
	"autoscaling.x-k8s.io",
	"aga.k8s.aws",
}
