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
		ProfileMetadata:  def.Metadata,
		Composition:    compositionLines(def.Metadata),
		IAMComposition: append([]string(nil), def.Metadata.IAMComposition...),
		TargetType:     targetType,
	}
	if def.Metadata.Scope == ScopeAWS && targetType != "" {
		out.IAMComposition = append(out.IAMComposition, awsScopeCatalogLines(def.Metadata.Name, targetType)...)
	}
	if def.Metadata.Scope == ScopeKube && def.RBAC != nil && targetType != "" {
		out.RBAC = EffectiveRBAC(def, targetType)
	}
	return out
}

func compositionLines(meta ProfileMetadata) []string {
	if len(meta.Capabilities) > 0 {
		return append([]string(nil), meta.Capabilities...)
	}
	switch meta.Name {
	case "kube-read":
		return []string{"Full cluster read (get/list/watch)"}
	case "kube-write":
		return []string{"Includes all kube-read RBAC", "Surgical workload mutate (see scope capabilities)"}
	case "kube-elevated":
		return []string{"Includes all kube-write RBAC", "Platform/core elevated delta, secrets read, pods/exec"}
	default:
		return nil
	}
}

// EffectiveRBAC returns the rules materialized for a target (rc or mc).
func EffectiveRBAC(def *Definition, targetType string) []actions.RBACRule {
	if def == nil || def.RBAC == nil {
		return nil
	}
	return def.RBAC(targetType)
}
