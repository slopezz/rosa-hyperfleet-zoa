package cli

import (
	"fmt"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/compoundid"
)

// FormatSessionID creates a compound session ID from a deployment name and
// a raw session ID returned by the Access Lambda.
func FormatSessionID(deployment, rawID string) string {
	return compoundid.Format(deployment, rawID)
}

// ParseSessionID splits a compound session ID into deployment and raw ID.
func ParseSessionID(compound string) (deployment, rawID string, err error) {
	dep, id, err := compoundid.Parse(compound)
	if err != nil {
		return "", "", fmt.Errorf("invalid session ID %q: expected <deployment>/<session-id>", compound)
	}
	return dep, id, nil
}
