// Package catalog holds shared filters and vocabulary for Trusted Action and break-glass catalogs.
package catalog

// Credential scopes (kube vs aws) — same JSON field and CLI --scope filter as TA and break-glass.
const (
	ScopeKube = "kube"
	ScopeAWS  = "aws"
)

// Privilege types for catalog metadata and list filters.
const (
	TypeRead     = "read"
	TypeWrite    = "write"
	TypeElevated = "elevated" // break-glass profiles only; TA API rejects elevated filters upstream
)
