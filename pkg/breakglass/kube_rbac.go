package breakglass

import "github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"

// Kube break-glass RBAC — verbs, platform operator catalog (RC/MC), shared rule helpers.
//
// Effective permissions are the union of all rules from each scope's RBAC builder for target rc or mc.
//
//	Scope id       │ Entry function     │ Composition (each tier includes the previous)
//	───────────────┼────────────────────┼────────────────────────────────────────────────────────────
//	kube-read      │ kubeReadRBAC       │ get/list/watch on core + rcPlatformAPIGroups / mcPlatformAPIGroups
//	kube-write     │ kubeWriteRBAC      │ kubeReadRBAC + kubeWriteMutationsRBAC (surgical workloads)
//	kube-elevated  │ kubeElevatedRBAC   │ kubeWriteRBAC + mutations in kube_elevated.go
//
// Elevated: platform apiGroups use verbsMutate on * (no delete); elevated core uses verbsMutateFull
// (nodes, PVC/PV, HPA, PDB, volumeattachments, NodeClaims, ingress/networkpolicy, apps/batch deltas, …).
// Policy limits (all kube scopes): no RBAC bind writes, no secret writes, exec only on elevated.
// Full rules: zoa breakglass describe kube-read|kube-write|kube-elevated --target-type rc|mc
//
// AWS scopes: aws_read.go / aws_write.go / aws_elevated.go (AwsRoleKey only).
// IAM in rosa-seaplane terraform/modules/zoa-lambda/breakglass_iam.tf.

var (
	verbsRead = []string{"get", "list", "watch"}
	// Mutate without delete (kube-write apps controllers, elevated platform CRs, …).
	verbsMutate = []string{"get", "list", "watch", "create", "patch", "update"}
	// kube-write (pods, jobs, configmaps, …) and elevated core workloads: full namespaced mutate.
	verbsMutateFull = []string{"get", "list", "watch", "create", "patch", "update", "delete"}
)

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
		Verbs:     verbsRead,
	}
}

func platformOperatorElevatedRule(apiGroups []string) actions.RBACRule {
	return actions.RBACRule{
		APIGroups: apiGroups,
		Resources: []string{"*"},
		Verbs:     verbsMutate,
	}
}