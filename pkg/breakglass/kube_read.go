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
				DefaultPendingHours: 24,
			},
			Capabilities: []string{
				"Cluster-wide get/list/watch: core workloads, networking, storage, policy/HPA, admission webhooks, CRD catalog",
				"Platform CRDs (seaplane, argoproj, hypershift, monitoring, cert-manager, …) per target (rc/mc)",
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
			Verbs: []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{"apps"},
			Resources: []string{
				"deployments", "replicasets", "statefulsets", "daemonsets",
				"controllerrevisions",
			},
			Verbs: []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{"batch"},
			Resources: []string{"jobs", "cronjobs"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{"autoscaling"},
			Resources: []string{"horizontalpodautoscalers"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{"policy"},
			Resources: []string{"poddisruptionbudgets"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{"networking.k8s.io"},
			Resources: []string{"ingresses", "ingressclasses", "networkpolicies"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{"discovery.k8s.io"},
			Resources: []string{"endpointslices"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{"storage.k8s.io"},
			Resources: []string{"storageclasses", "volumeattachments", "csidrivers", "csinodes", "csistoragecapacities"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{"scheduling.k8s.io"},
			Resources: []string{"priorityclasses"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{"coordination.k8s.io"},
			Resources: []string{"leases"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{"apiextensions.k8s.io"},
			Resources: []string{"customresourcedefinitions"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{"rbac.authorization.k8s.io"},
			Resources: []string{"roles", "clusterroles", "rolebindings", "clusterrolebindings"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{"certificates.k8s.io"},
			Resources: []string{"certificatesigningrequests"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{"admissionregistration.k8s.io"},
			Resources: []string{
				"mutatingwebhookconfigurations", "validatingwebhookconfigurations",
				"validatingadmissionpolicies", "validatingadmissionpolicybindings",
			},
			Verbs: []string{"get", "list", "watch"},
		},
	}
	// Operator CRDs from rosa-seaplane ArgoCD (shared + regional-cluster / management-cluster charts).
	// Wildcard rules are read-only; secrets stay on kube-elevated only.
	if targetType == actions.TargetTypeRC {
		rules = append(rules, platformOperatorReadRule(rcPlatformAPIGroups))
	}
	if targetType == actions.TargetTypeMC {
		rules = append(rules, platformOperatorReadRule(mcPlatformAPIGroups))
	}
	return rules
}

// eksAddonAPIGroups are AWS/EKS controllers present on RC and MC (eph inventory); each is a distinct apiGroup
// (e.g. cert-manager.io does not cover acme.cert-manager.io or generators.external-secrets.io).
var eksAddonAPIGroups = []string{
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

// mcClusterAPIGroups cover CAPI and HyperShift satellite groups on MC (not under hypershift.openshift.io alone).
var mcClusterAPIGroups = []string{
	"cluster.x-k8s.io",
	"infrastructure.cluster.x-k8s.io",
	"addons.cluster.x-k8s.io",
	"ipam.cluster.x-k8s.io",
	"auditlogpersistence.hypershift.openshift.io",
	"certificates.hypershift.openshift.io",
	"scheduling.hypershift.openshift.io",
}

// rcPlatformAPIGroups covers RC GitOps apps (hyperfleet/seaplane, monitoring, thanos, platform-api, LBC, Karpenter).
var rcPlatformAPIGroups = append([]string{
	"seaplane.io",
	"argoproj.io",
	"monitoring.coreos.com",
	"monitoring.thanos.io",
	"cert-manager.io",
	"external-secrets.io",
	"elbv2.k8s.aws",
	"karpenter.sh",
	"karpenter.k8s.aws",
}, eksAddonAPIGroups...)

// mcPlatformAPIGroups covers MC GitOps (hypershift, kube-applier, monitoring, cert-manager, karpenter).
var mcPlatformAPIGroups = append(append([]string{
	"hypershift.openshift.io",
	"argoproj.io",
	"monitoring.coreos.com",
	"cert-manager.io",
	"external-secrets.io",
	"karpenter.sh",
	"karpenter.k8s.aws",
	"route.openshift.io",
}, mcClusterAPIGroups...), eksAddonAPIGroups...)

func platformOperatorReadRule(apiGroups []string) actions.RBACRule {
	return actions.RBACRule{
		APIGroups: apiGroups,
		Resources: []string{"*"},
		Verbs:     []string{"get", "list", "watch"},
	}
}
