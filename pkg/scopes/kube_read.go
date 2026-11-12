package scopes

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
				Approval:            "manual",
				DefaultGrantHours:   4,
				DefaultPendingHours: 24,
			},
			Capabilities: []string{
				"Investigate workloads, events, and platform CRs on RC or MC",
			},
			Restrictions: []string{
				"No secrets, service account tokens, or pods/exec",
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
				"pods", "pods/log", "services", "persistentvolumeclaims",
				"replicationcontrollers", "resourcequotas", "limitranges",
			},
			Verbs: []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{"apps"},
			Resources: []string{"deployments", "replicasets", "statefulsets", "daemonsets", "controllerrevisions"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{"batch"},
			Resources: []string{"jobs", "cronjobs"},
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
	}
	if targetType == actions.TargetTypeRC {
		rules = append(rules,
			actions.RBACRule{
				APIGroups: []string{"seaplane.io", "argoproj.io", "monitoring.coreos.com"},
				Resources: []string{"*"},
				Verbs:     []string{"get", "list", "watch"},
			},
		)
	}
	if targetType == actions.TargetTypeMC {
		rules = append(rules,
			actions.RBACRule{
				APIGroups: []string{"hypershift.openshift.io"},
				Resources: []string{"*"},
				Verbs:     []string{"get", "list", "watch"},
			},
		)
	}
	return rules
}
