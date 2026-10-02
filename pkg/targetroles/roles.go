package targetroles

import "fmt"

// BoundaryAccessRoleARN is the cross-account role RC Access assumes for ECS lifecycle in the target account.
func BoundaryAccessRoleARN(accountID, targetID string) string {
	return fmt.Sprintf("arn:aws:iam::%s:role/%s-zoa-boundary-access", accountID, targetID)
}

// ExecScopedRoleARN is the role vended on session join for per-task ECS Exec in the target account.
func ExecScopedRoleARN(accountID, targetID string) string {
	return fmt.Sprintf("arn:aws:iam::%s:role/%s-zoa-boundary-exec-scoped", accountID, targetID)
}
