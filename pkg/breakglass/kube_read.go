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

// rcPlatformAPIGroups covers RC GitOps apps (e.g. hyperfleet, monitoring, thanos-operator, platform-api TGBs).
var rcPlatformAPIGroups = []string{
	"seaplane.io",
	"argoproj.io",
	"monitoring.coreos.com",
	"monitoring.thanos.io",
	"cert-manager.io",
	"external-secrets.io",
	"elbv2.k8s.aws",
	"karpenter.sh",
	"karpenter.k8s.aws",
}

// mcPlatformAPIGroups covers MC GitOps apps (hypershift, monitoring, shared cert-manager/karpenter/argocd).
var mcPlatformAPIGroups = []string{
	"hypershift.openshift.io",
	"argoproj.io",
	"monitoring.coreos.com",
	"cert-manager.io",
	"external-secrets.io",
	"karpenter.sh",
	"karpenter.k8s.aws",
	"secrets-store.csi.x-k8s.io",
	"route.openshift.io",
}

func platformOperatorReadRule(apiGroups []string) actions.RBACRule {
	return actions.RBACRule{
		APIGroups: apiGroups,
		Resources: []string{"*"},
		Verbs:     []string{"get", "list", "watch"},
	}
}
