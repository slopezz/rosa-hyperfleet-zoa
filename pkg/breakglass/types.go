package breakglass

import (
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"
	catalogpkg "github.com/openshift-online/rosa-seaplane-zoa/pkg/catalog"
)

const (
	ScopeKube = catalogpkg.ScopeKube
	ScopeAWS  = catalogpkg.ScopeAWS

	TypeRead     = catalogpkg.TypeRead
	TypeWrite    = catalogpkg.TypeWrite
	TypeElevated = catalogpkg.TypeElevated
)

// Authorization mirrors profile approval workflow defaults (snapshotted on create).
type Authorization struct {
	Approval            string `json:"approval"`
	DefaultGrantHours   int    `json:"default_grant_hours"`
	DefaultRequestHours int    `json:"default_request_hours"`
	MinApprovers        int    `json:"min_approvers,omitempty"`
	RequireDecisionNote bool   `json:"require_decision_note,omitempty"`
	PageOnRequest       bool   `json:"page_on_request,omitempty"`
}

// ProfileMetadata is returned by list/describe profile APIs.
type ProfileMetadata struct {
	Name           string        `json:"name"`
	Scope          string        `json:"scope"`
	Type           string        `json:"type"`
	Description    string        `json:"description"`
	Authorization  Authorization `json:"authorization"`
	Capabilities   []string      `json:"capabilities,omitempty"`
	Restrictions   []string      `json:"restrictions,omitempty"`
	TargetTypes    []string      `json:"target_types,omitempty"`
	IAMComposition []string      `json:"iam_composition,omitempty"`
	Examples       []string      `json:"examples,omitempty"`
}

// Definition compiles RBAC / AWS wiring for a break-glass profile.
type Definition struct {
	Metadata ProfileMetadata
	RBAC     func(targetType string) []actions.RBACRule
}
