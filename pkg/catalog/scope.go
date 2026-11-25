// Package catalog holds shared filters for Trusted Action and break-glass catalogs.
package catalog

import (
	"strings"
)

// MatchCredentialScope returns true when filter is empty or matches item scope (kube/aws),
// including legacy execution rows stored as kube-api/aws-api.
func MatchCredentialScope(itemScope, filter string) bool {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return true
	}
	itemScope = strings.TrimSpace(itemScope)
	if itemScope == filter {
		return true
	}
	// zoa-retrocompatibility-cleanup: drop kube-api/aws-api after stored rows age out
	switch filter {
	case "kube":
		return itemScope == "kube-api"
	case "aws":
		return itemScope == "aws-api"
	default:
		return false
	}
}

// ExecutionScopeDynamoValues expands a runs-list scope query for DynamoDB (IN when legacy values needed).
func ExecutionScopeDynamoValues(filter string) []string {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return nil
	}
	switch filter {
	case "kube":
		return []string{"kube", "kube-api"} // zoa-retrocompatibility-cleanup
	case "aws":
		return []string{"aws", "aws-api"} // zoa-retrocompatibility-cleanup
	default:
		return []string{filter}
	}
}

// MatchPrivilegeType filters catalog rows by privilege tier (read, write, elevated).
// elevated is valid only on break-glass profiles; TA callers should reject elevated filters upstream.
func MatchPrivilegeType(itemType, filter string) bool {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return true
	}
	return strings.TrimSpace(itemType) == filter
}