package scopes

// NeedsApproval reports whether a scope requires a manual peer decision before connect.
func NeedsApproval(auth Authorization) bool {
	switch auth.Approval {
	case "auto", "none":
		return false
	default:
		return true
	}
}
