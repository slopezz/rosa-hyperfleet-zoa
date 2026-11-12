package api

import (
	"fmt"
	"strings"
)

const compoundIDSeparator = "/"

// ParseCompoundResourceID splits deployment/<uuid> (approve, reject, session patterns).
func ParseCompoundResourceID(compound string) (deployment, rawID string, err error) {
	parts := strings.SplitN(compound, compoundIDSeparator, 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid compound id %q: expected <deployment>/<id>", compound)
	}
	return parts[0], parts[1], nil
}

// FormatCompoundResourceID builds deployment/<uuid>.
func FormatCompoundResourceID(deployment, rawID string) string {
	return deployment + compoundIDSeparator + rawID
}
