package breakglass

// awsScopeCatalogLines are appended to aws-* scope describe/markdown (target-specific notes only).
func awsScopeCatalogLines(scopeID, targetType string) []string {
	if scopeID == "aws-write" && targetType == "mc" {
		return []string{"MC: ReadOnlyAccess only."}
	}
	return nil
}
