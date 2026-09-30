package cli

import (
	"fmt"
	"strings"
)

// sessionIDSeparator is the delimiter between deployment and session ID in
// compound session IDs (e.g. "us-east-1/sess-abc123").
const sessionIDSeparator = "/"

// FormatSessionID creates a compound session ID from a deployment name and
// a raw session ID returned by the Access Lambda.
func FormatSessionID(deployment, rawID string) string {
	return deployment + sessionIDSeparator + rawID
}

// ParseSessionID splits a compound session ID into deployment and raw ID.
// Returns an error if the ID does not contain exactly one separator.
func ParseSessionID(compound string) (deployment, rawID string, err error) {
	parts := strings.SplitN(compound, sessionIDSeparator, 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid session ID %q: expected <deployment>/<session-id>", compound)
	}
	return parts[0], parts[1], nil
}
