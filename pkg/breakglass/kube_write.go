// kube_write break-glass profile: full RBAC in kubeWriteRBAC below.
//
// Verbs: kube_rbac.go only. APIGroups and resources are explicit in this file.
package breakglass

import "github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"

func init() {
	register(&Definition{
		Metadata: ProfileMetadata{
			Name:        "kube_write",
			Scope:       ScopeKube,
			Type:        TypeWrite,
			Description: "Workload read/write: get/list/watch cluster-wide plus surgical mutate on apps controllers, pods, configmaps, and jobs",
			Capabilities: []string{
				"Cluster-wide get/list/watch (same coverage as kube_read)",
				"Apps controllers: create/patch/update (no delete); full mutate on pods, configmaps, jobs",
			},
			Authorization: Authorization{
				Approval:            "none",
				DefaultGrantHours:   4,
				DefaultRequestHours: 24,
			},
			Restrictions: []string{
				"No secrets, pods/exec, RBAC bind writes, or platform CR mutate (use kube_elevated)",
			},
			TargetTypes: []string{actions.TargetTypeRC, actions.TargetTypeMC},
			Examples: []string{
				"zoa breakglass request kube_write --reason ROSAENG-1234",
				"zoa breakglass request kube_write --reason ROSAENG-1234 --wait",
				"zoa breakglass request kube_write --reason ROSAENG-1234 --wait --wait-timeout 10m --wait-poll-interval 1m",
			},
		},
		RBAC: kubeWriteRBAC,
	})
}

// kubeWriteRBAC is the complete kube_write ClusterRole policy for rc or mc.
func kubeWriteRBAC(targetType string) []actions.RBACRule {
	rules := []actions.RBACRule{
		{
			APIGroups: []string{""},
			Resources: []string{
				"endpoints", "events", "namespaces", "nodes",
				"pods/log", "pods/status", "services", "serviceaccounts",
				"persistentvolumeclaims", "persistentvolumes",
				"replicationcontrollers", "resourcequotas", "limitranges",
			},
			Verbs: verbsRead,
		},
		{
			APIGroups: []string{""},
			Resources: []string{"pods", "configmaps"},
			Verbs:     verbsMutateFull,
		},
		{
			APIGroups: []string{"apps"},
			Resources: []string{"controllerrevisions"},
			Verbs:     verbsRead,
		},
		{
			APIGroups: []string{"apps"},
			Resources: []string{"deployments", "replicasets", "statefulsets", "daemonsets"},
			Verbs:     verbsMutate,
		},
		{
			APIGroups: []string{"batch"},
			Resources: []string{"cronjobs"},
			Verbs:     verbsRead,
		},
		{
			APIGroups: []string{"batch"},
			Resources: []string{"jobs"},
			Verbs:     verbsMutateFull,
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
			APIGroups: kubeWritePlatformRCAPIGroups,
			Resources: []string{"*"},
			Verbs:     verbsRead,
		})
	case actions.TargetTypeMC:
		rules = append(rules, actions.RBACRule{
			APIGroups: kubeWritePlatformMCAPIGroups,
			Resources: []string{"*"},
			Verbs:     verbsRead,
		})
	}
	return rules
}

// Platform operator apiGroups (GitOps + EKS addons). Keep aligned with kube_read / kube_elevated lists.
var kubeWritePlatformRCAPIGroups = []string{
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

var kubeWritePlatformMCAPIGroups = []string{
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
