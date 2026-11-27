package cli

import (
	"net/url"
	"regexp"
	"strings"
)

var auditPathUUID = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// formatAuditPathForTable normalizes stored paths for human tables: drop /api/v0/
// and redact id segments already shown in EXEC_ID (uuid, ECS task id, compound approve ids).
func formatAuditPathForTable(path string) string {
	const prefix = "/api/v0/"
	if !strings.HasPrefix(path, prefix) {
		return path
	}
	return redactAuditPathIDs(path[len(prefix):])
}

func redactAuditPathIDs(path string) string {
	path = strings.Trim(path, "/")
	if path == "" {
		return path
	}
	parts := strings.Split(path, "/")
	out := make([]string, 0, len(parts))
	for _, seg := range parts {
		if auditPathSegmentIsRedactable(seg) {
			continue
		}
		out = append(out, seg)
	}
	return strings.Join(out, "/")
}

func auditPathSegmentIsRedactable(seg string) bool {
	decoded, err := url.PathUnescape(seg)
	if err != nil {
		decoded = seg
	}
	if auditPathSegmentIsRedactableID(decoded) {
		return true
	}
	if strings.Contains(decoded, "/") {
		tail := decoded[strings.LastIndex(decoded, "/")+1:]
		if auditPathSegmentIsRedactableID(tail) {
			return true
		}
	}
	return false
}

func auditPathSegmentIsRedactableID(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if auditPathUUID.MatchString(s) {
		return true
	}
	if len(s) == 32 && isHex32(s) {
		return true
	}
	return false
}

func isHex32(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}