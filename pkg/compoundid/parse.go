package compoundid

import (
	"fmt"
	"strings"
)

const Separator = "/"

// Parse splits deployment/<resource-id> (sessions, break-glass approve, terminate).
func Parse(compound string) (deployment, rawID string, err error) {
	parts := strings.SplitN(compound, Separator, 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid compound id %q: expected <deployment>/<id>", compound)
	}
	return parts[0], parts[1], nil
}

// Format builds deployment/<resource-id>.
func Format(deployment, rawID string) string {
	return deployment + Separator + rawID
}
