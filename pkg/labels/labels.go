// Package labels defines ZOA Kubernetes label keys and values shared across
// executor, scheduler GC, and Trusted Actions that create child resources.
package labels

import "os"

const (
	KeyExecutionID = "zoa.openshift.io/execution-id"
	KeyRequestID   = "zoa.openshift.io/request-id"
	KeyAction      = "zoa.openshift.io/action"
	KeyTarget      = "zoa.openshift.io/target"
	KeyComponent   = "zoa.openshift.io/component"

	KeyManagedBy      = "app.kubernetes.io/managed-by"
	ValueManagedByZOA = "zoa"

	ComponentMustGather = "must-gather"

	// ClusterNamespacePrefix is the Seaplane v2 HostedCluster namespace prefix (cluster-<uuid>).
	ClusterNamespacePrefix = "cluster-"

	DefaultJobsNamespace = "zoa-jobs"
	envJobsNamespace     = "ZOA_JOBS_NAMESPACE"
)

// JobsNamespace returns the namespace for ZOA async Jobs and child pods.
func JobsNamespace() string {
	if v := os.Getenv(envJobsNamespace); v != "" {
		return v
	}
	return DefaultJobsNamespace
}

// MustGatherPodSelector matches must-gather child pods for GC cleanup.
func MustGatherPodSelector() string {
	return KeyManagedBy + "=" + ValueManagedByZOA + "," + KeyComponent + "=" + ComponentMustGather
}

// ZOAManagedSelector matches ZOA-created Jobs and related resources.
func ZOAManagedSelector() string {
	return KeyManagedBy + "=" + ValueManagedByZOA
}
