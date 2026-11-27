package api

import (
	"net/url"
	"strings"
)

// breakglassAuditPath is the canonical audit path for break-glass (profile in path, no request id),
// aligned with trusted-actions/{action}/run for metrics and zoa audit PATH column.
func breakglassAuditPath(profile, suffix string) string {
	profile = strings.TrimSpace(profile)
	if profile == "" {
		profile = "_"
	}
	return V0Prefix + "/breakglass/" + url.PathEscape(profile) + "/" + suffix
}

// sessionAuditPath is the canonical Access audit path (no session id in path; id is execution_id).
func sessionAuditPath(suffix string) string {
	return V0Prefix + "/sessions/" + suffix
}