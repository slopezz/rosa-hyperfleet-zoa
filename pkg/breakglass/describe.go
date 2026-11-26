package breakglass

import (
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"
)

// ProfileDescribe is operator-facing profile detail (list endpoints omit rbac).
type ProfileDescribe struct {
	ProfileMetadata
	IAMComposition []string           `json:"iam_composition,omitempty"`
	RBAC           []actions.RBACRule `json:"rbac,omitempty"`
	TargetType     string             `json:"target_type,omitempty"`
}

// DescribeProfile builds catalog metadata plus effective RBAC or IAM notes for targetType (rc or mc).
func DescribeProfile(def *Definition, targetType string) ProfileDescribe {
	if def == nil {
		return ProfileDescribe{}
	}
	out := ProfileDescribe{
		ProfileMetadata: def.Metadata,
		IAMComposition:  append([]string(nil), def.Metadata.IAMComposition...),
		TargetType:      targetType,
	}
	if def.Metadata.Scope == ScopeKube && def.RBAC != nil && targetType != "" {
		out.RBAC = EffectiveRBAC(def, targetType)
	}
	return out
}

// EffectiveRBAC returns the profile rules for a target (rc or mc).
func EffectiveRBAC(def *Definition, targetType string) []actions.RBACRule {
	if def == nil || def.RBAC == nil {
		return nil
	}
	return def.RBAC(targetType)
}

// DefaultDescribeTargetType picks rc or mc when rendering RBAC on describe without Lambda target context.
func DefaultDescribeTargetType(meta ProfileMetadata) string {
	if len(meta.TargetTypes) == 1 {
		return meta.TargetTypes[0]
	}
	return actions.TargetTypeMC
}
