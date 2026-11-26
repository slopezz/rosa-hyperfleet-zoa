package breakglass

import (
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"
)

// ProfileDescribe is the operator-facing scope detail (list endpoints omit rbac/composition).
type ProfileDescribe struct {
	ProfileMetadata
	Composition    []string           `json:"composition,omitempty"`
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
		Composition:     append([]string(nil), def.Metadata.Capabilities...),
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
