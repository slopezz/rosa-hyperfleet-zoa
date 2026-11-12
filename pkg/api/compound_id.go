package api

import (
	"strings"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/compoundid"
)

// ParseCompoundResourceID splits deployment/<uuid> (approve, reject, session patterns).
func ParseCompoundResourceID(compound string) (deployment, rawID string, err error) {
	return compoundid.Parse(compound)
}

// FormatCompoundResourceID builds deployment/<uuid>.
func FormatCompoundResourceID(deployment, rawID string) string {
	return compoundid.Format(deployment, rawID)
}

// resolveResourceID returns the DynamoDB key from a path id (raw UUID or compound).
func resolveResourceID(pathID string) string {
	_, rawID := resolveCompoundResource(pathID)
	return rawID
}

// resolveCompoundResource splits an optional deployment prefix from the resource id.
func resolveCompoundResource(pathID string) (deployment, rawID string) {
	pathID = strings.TrimSpace(pathID)
	dep, id, err := compoundid.Parse(pathID)
	if err != nil {
		return "", pathID
	}
	return dep, id
}
