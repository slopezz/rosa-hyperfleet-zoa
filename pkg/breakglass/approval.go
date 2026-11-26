package breakglass

// NeedsApproval reports whether a profile requires a peer approval workflow before activate.
// v1 break-glass profiles use approval "none" (same as Trusted Actions today). Future
// workflows use an approval object (min_count, approver rules) — see docs/approval-workflow.md.
func NeedsApproval(auth Authorization) bool {
	return auth.Approval != "none"
}
