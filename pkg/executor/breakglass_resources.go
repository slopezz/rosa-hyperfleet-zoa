package executor

import (
	"context"
	"fmt"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/scopes"
)

// EnsureBreakglassKube creates cluster-scoped RBAC for an approved break-glass kube scope.
func (e *Executor) EnsureBreakglassKube(ctx context.Context, requestID, scopeID string) (string, error) {
	def, ok := scopes.Get(scopeID)
	if !ok || def.RBAC == nil {
		return "", fmt.Errorf("unknown kube scope %q", scopeID)
	}
	if def.Metadata.Plane != scopes.PlaneKube {
		return "", fmt.Errorf("scope %q is not a kube plane scope", scopeID)
	}
	if err := e.ensureNamespace(ctx); err != nil {
		return "", fmt.Errorf("ensuring namespace: %w", err)
	}
	rbac := &actions.RBACConfig{
		ClusterScoped: true,
		Rules:         def.RBAC(e.targetType),
	}
	saName := fmt.Sprintf("zoa-bg-%s", requestID)
	if err := e.createResourcesIdempotent(ctx, requestID, saName, rbac, nil); err != nil {
		e.cleanupResources(ctx, requestID, saName, rbac, nil)
		return "", err
	}
	return saName, nil
}

// CleanupBreakglass removes K8s resources for a break-glass grant (worker GC).
func (e *Executor) CleanupBreakglass(ctx context.Context, requestID string) {
	saName := fmt.Sprintf("zoa-bg-%s", requestID)
	rbac := &actions.RBACConfig{ClusterScoped: true}
	e.cleanupResources(ctx, requestID, saName, rbac, nil)
}
