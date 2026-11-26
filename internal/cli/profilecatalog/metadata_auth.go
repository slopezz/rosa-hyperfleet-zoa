package profilecatalog

import (
	"fmt"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/breakglass"
)

// AuthString reads a string authorization field from profile metadata.
func AuthString(p client.ProfileMetadata, key string) string {
	if p.Authorization == nil {
		return ""
	}
	raw, ok := p.Authorization[key]
	if !ok || raw == nil {
		return ""
	}
	if v, ok := raw.(string); ok {
		return v
	}
	return fmt.Sprint(raw)
}

// AuthInt reads an integer authorization field from profile metadata.
func AuthInt(p client.ProfileMetadata, key string) int {
	if p.Authorization == nil {
		return 0
	}
	raw, ok := p.Authorization[key]
	if !ok || raw == nil {
		return 0
	}
	switch v := raw.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

// Approval returns the approval mode from metadata.
func Approval(p client.ProfileMetadata) string {
	return AuthString(p, "approval")
}

// RequestTTL formats default_request_hours for table/describe output.
func RequestTTL(p client.ProfileMetadata) string {
	h := AuthInt(p, "default_request_hours")
	if h <= 0 {
		return ""
	}
	return fmt.Sprintf("%dh", h)
}

// GrantTTL formats default_grant_hours for table/describe output.
func GrantTTL(p client.ProfileMetadata) string {
	h := AuthInt(p, "default_grant_hours")
	if h <= 0 {
		return ""
	}
	return fmt.Sprintf("%dh", h)
}

// CredentialTTL returns the per-activate credential max for the profile scope (kube/aws).
func CredentialTTL(p client.ProfileMetadata) string {
	if p.Scope == "" {
		return ""
	}
	return breakglass.CredentialMaxTTLDisplay(p.Scope)
}