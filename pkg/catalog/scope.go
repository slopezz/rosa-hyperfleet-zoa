// Package catalog holds shared filters for Trusted Action and break-glass catalogs.
package catalog

import (
	"strings"
)

// MatchCredentialScope returns true when filter is empty or equals itemScope (kube or aws).
func MatchCredentialScope(itemScope, filter string) bool {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return true
	}
	return strings.TrimSpace(itemScope) == filter
}

// ExecutionScopeDynamoValues is the DynamoDB IN list for a runs-list scope filter.
func ExecutionScopeDynamoValues(filter string) []string {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return nil
	}
	return []string{filter}
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