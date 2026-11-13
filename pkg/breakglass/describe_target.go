package breakglass

import "github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"

// DefaultDescribeTargetType picks rc or mc when rendering RBAC on describe without Lambda target context.
func DefaultDescribeTargetType(meta ScopeMetadata) string {
	if len(meta.TargetTypes) == 1 {
		return meta.TargetTypes[0]
	}
	return actions.TargetTypeMC
}
