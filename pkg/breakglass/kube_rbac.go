package breakglass

// Kube break-glass RBAC — shared verb vocabulary only.
//
// APIGroups, Resources, and policy live in kube_read.go, kube_write.go, and kube_elevated.go.
// Expand verb literals here, not per rule.
//
//   - verbsRead       get, list, watch
//   - verbsMutate     read + create, patch, update (no delete)
//   - verbsMutateFull read + create, patch, update, delete
//   - verbsExecCreate pods/exec create only (kube_elevated)
//
// CLI: zoa breakglass describe kube_read|kube_write|kube_elevated --target-type rc|mc
//
// AWS profiles: aws_read.go / aws_write.go / aws_elevated.go (IAM via profile name → env ARN).

var (
	verbsRead       = []string{"get", "list", "watch"}
	verbsMutate     = []string{"get", "list", "watch", "create", "patch", "update"}
	verbsMutateFull = []string{"get", "list", "watch", "create", "patch", "update", "delete"}
	verbsExecCreate = []string{"create"}
)