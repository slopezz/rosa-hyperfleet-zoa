package breakglass

import "github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"

const (
	PlaneKube = "kube"
	PlaneAWS  = "aws"

	TypeRead     = "read"
	TypeWrite    = "write"
	TypeElevated = "elevated"
)

// Authorization mirrors scope approval workflow defaults (snapshotted on create).
type Authorization struct {
	Approval            string `json:"approval"`
	DefaultGrantHours   int    `json:"default_grant_hours"`
	DefaultPendingHours int    `json:"default_pending_hours"`
	MinApprovers        int    `json:"min_approvers,omitempty"`
	RequireDecisionNote bool   `json:"require_decision_note,omitempty"`
	PageOnRequest       bool   `json:"page_on_request,omitempty"`
}

// ScopeMetadata is returned by list/describe APIs.
type ScopeMetadata struct {
	ID             string        `json:"id"`
	Plane          string        `json:"plane"`
	Type           string        `json:"type"`
	Description    string        `json:"description"`
	Authorization  Authorization `json:"authorization"`
	Capabilities   []string      `json:"capabilities,omitempty"`
	Restrictions   []string      `json:"restrictions,omitempty"`
	TargetTypes    []string      `json:"target_types,omitempty"`
	AwsRoleKey     string        `json:"aws_role_key,omitempty"`
	IAMComposition []string      `json:"iam_composition,omitempty"`
}

// Definition compiles RBAC / AWS wiring for a scope.
type Definition struct {
	Metadata ScopeMetadata
	RBAC     func(targetType string) []actions.RBACRule
}
