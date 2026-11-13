package store

// Workflow approval outcome (peer decision). Not the scope catalog policy (see pkg/breakglass.Authorization).
const (
	WorkflowApprovalPending  = "pending"
	WorkflowApprovalApproved = "approved"
	WorkflowApprovalRejected = "rejected"
)
