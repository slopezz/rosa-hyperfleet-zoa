package breakglass

import (
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"
)

// ScopeDescribe is the operator-facing scope detail (list endpoints omit rbac/composition).
type ScopeDescribe struct {
	ScopeMetadata
	Composition    []string           `json:"composition,omitempty"`
	IAMComposition []string           `json:"iam_composition,omitempty"`
	RBAC           []actions.RBACRule `json:"rbac,omitempty"`
	TargetType     string             `json:"target_type,omitempty"`
}

// DescribeScope builds catalog metadata plus effective RBAC or IAM notes for targetType (rc or mc).
func DescribeScope(def *Definition, targetType string) ScopeDescribe {
	if def == nil {
		return ScopeDescribe{}
	}
	out := ScopeDescribe{
		ScopeMetadata:  def.Metadata,
		Composition:    compositionLines(def.Metadata),
		IAMComposition: append([]string(nil), def.Metadata.IAMComposition...),
		TargetType:     targetType,
	}
	if def.Metadata.Plane == PlaneAWS && targetType != "" {
		out.IAMComposition = append(out.IAMComposition, awsScopeCatalogLines(def.Metadata.ID, targetType)...)
	}
	if def.Metadata.Plane == PlaneKube && def.RBAC != nil && targetType != "" {
		out.RBAC = EffectiveRBAC(def, targetType)
	}
	return out
}

func compositionLines(meta ScopeMetadata) []string {
	if len(meta.Capabilities) > 0 {
		return append([]string(nil), meta.Capabilities...)
	}
	switch meta.ID {
	case "kube-read":
		return []string{"Full cluster read (get/list/watch)"}
	case "kube-write":
		return []string{"Includes all kube-read RBAC", "Adds surgical workload mutations"}
	case "kube-elevated":
		return []string{"Includes all kube-write RBAC", "Adds secret read and kubectl exec", "Default grant ≤ 2h"}
	default:
		return nil
	}
}
