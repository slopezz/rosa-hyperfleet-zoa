// kube_read break-glass profile: full RBAC in kubeReadRBAC below.
//
// Verbs: kube_rbac.go only. APIGroups and resources are explicit in this file.
package breakglass

import (
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"
)

func init() {
	register(&Definition{
		Metadata: ProfileMetadata{
			Name: "kube_read",
			Scope: ScopeKube,
			Type:        TypeRead,
			Description: "Read-only kubectl (get/list/watch); no secrets or exec",
			Authorization: Authorization{
				Approval:            "none",
				DefaultGrantHours:   4,
				DefaultRequestHours: 24,
			},
			Capabilities: []string{
				"Cluster-wide get/list/watch: core workloads, networking, storage, policy/HPA, admission webhooks, CRD catalog",
				"Platform operator apiGroups: get/list/watch * per RC or MC (see kubeReadPlatform*APIGroups)",
			},
			Restrictions: []string{
				"No secrets or pods/exec (serviceaccount objects are metadata-only read)",
			},
			TargetTypes: []string{actions.TargetTypeRC, actions.TargetTypeMC},
			Examples: []string{
				"zoa breakglass request kube_read --reason ROSAENG-1234",
				"zoa breakglass request kube_read --reason ROSAENG-1234 --wait",
				"zoa breakglass request kube_read --reason ROSAENG-1234 --wait --wait-timeout 10m --wait-poll-interval 1m",
			},
		},
		RBAC: kubeReadRBAC,
	})
}

// kubeReadRBAC is the complete kube_read ClusterRole policy for rc or mc.
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
	switch targetType {
	case actions.TargetTypeRC:
		rules = append(rules, actions.RBACRule{
			APIGroups: kubeReadPlatformRCAPIGroups,
			Resources: []string{"*"},
			Verbs:     verbsRead,
		})
	case actions.TargetTypeMC:
		rules = append(rules, actions.RBACRule{
			APIGroups: kubeReadPlatformMCAPIGroups,
			Resources: []string{"*"},
			Verbs:     verbsRead,
		})
	}
	return rules
}

// Platform operator apiGroups (GitOps + EKS addons). Keep aligned with kube_write / kube_elevated lists.
var kubeReadPlatformRCAPIGroups = []string{
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

var kubeReadPlatformMCAPIGroups = []string{
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
