// Package profilecatalog builds offline break-glass profile views from pkg/breakglass.
package profilecatalog

import (
	"fmt"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/breakglass"
)

// OfflineNotice is printed to stderr when using --offline break-glass catalog output.
const OfflineNotice = "Offline profile catalog: embedded in this zoa CLI build (filtered by target type). " +
	"The live API is the source of truth when connected — run `zoa breakglass profiles` without --offline to verify."

// List returns API-shaped profile metadata for the given target type (rc or mc).
func List(targetType string) ([]client.ProfileMetadata, error) {
	breakglass.SetTargetType(targetType)
	defer breakglass.SetTargetType("")

	registered := breakglass.List()
	out := make([]client.ProfileMetadata, 0, len(registered))
	for _, meta := range registered {
		out = append(out, ClientProfileFromMetadata(meta))
	}
	return out, nil
}

// Get returns one profile from the embedded registry for targetType, or an error if unknown/hidden.
func Get(targetType, profileID string) (*client.ProfileMetadata, error) {
	breakglass.SetTargetType(targetType)
	defer breakglass.SetTargetType("")

	def, ok := breakglass.Get(profileID)
	if !ok {
		return nil, fmt.Errorf("profile %q not found for target type %q", profileID, targetType)
	}
	desc := breakglass.DescribeProfile(def, targetType)
	profile := ClientProfileFromDescribe(desc)
	return &profile, nil
}

// Describe returns full profile detail for offline describe (includes RBAC / IAM composition).
func Describe(targetType, profileID string) (*client.ProfileMetadata, error) {
	return Get(targetType, profileID)
}

// ClientProfileFromMetadata maps registry metadata to the API/CLI ProfileMetadata shape.
func ClientProfileFromMetadata(m breakglass.ProfileMetadata) client.ProfileMetadata {
	auth := map[string]interface{}{
		"approval":              m.Authorization.Approval,
		"default_grant_hours":   m.Authorization.DefaultGrantHours,
		"default_request_hours": m.Authorization.DefaultRequestHours,
	}
	if m.Authorization.MinApprovers > 0 {
		auth["min_approvers"] = m.Authorization.MinApprovers
	}
	if m.Authorization.RequireDecisionNote {
		auth["require_decision_note"] = true
	}
	if m.Authorization.PageOnRequest {
		auth["page_on_request"] = true
	}
	return client.ProfileMetadata{
		Name:          m.Name,
		Scope:         m.Scope,
		Type:          m.Type,
		Description:   m.Description,
		Authorization: auth,
		Capabilities:  m.Capabilities,
		Restrictions:  m.Restrictions,
		Examples:      append([]string(nil), m.Examples...),
	}
}

// ClientProfileFromDescribe maps DescribeProfile output to the CLI/API client shape.
func ClientProfileFromDescribe(d breakglass.ProfileDescribe) client.ProfileMetadata {
	profile := ClientProfileFromMetadata(d.ProfileMetadata)
	profile.IAMComposition = d.IAMComposition
	profile.TargetType = d.TargetType
	if len(d.RBAC) > 0 {
		profile.RBAC = make([]client.ProfileRBACRule, 0, len(d.RBAC))
		for _, rule := range d.RBAC {
			profile.RBAC = append(profile.RBAC, client.ProfileRBACRule{
				APIGroups: append([]string(nil), rule.APIGroups...),
				Resources: append([]string(nil), rule.Resources...),
				Verbs:     append([]string(nil), rule.Verbs...),
			})
		}
	}
	return profile
}
