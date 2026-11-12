package executor

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/breakglass"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/labels"
)

func breakglassResourceName(requestID string) string {
	return fmt.Sprintf("zoa-bg-%s", requestID)
}

func breakglassManagedLabels(requestID string) map[string]string {
	return map[string]string{
		labels.KeyRequestID:            requestID,
		"app.kubernetes.io/managed-by": "zoa-lambda",
	}
}

// EnsureBreakglassKube creates cluster-scoped RBAC for an approved break-glass kube scope.
func (e *Executor) EnsureBreakglassKube(ctx context.Context, requestID, scopeID string) (string, error) {
	def, ok := breakglass.Get(scopeID)
	if !ok || def.RBAC == nil {
		return "", fmt.Errorf("unknown kube scope %q", scopeID)
	}
	if def.Metadata.Plane != breakglass.PlaneKube {
		return "", fmt.Errorf("scope %q is not a kube plane scope", scopeID)
	}
	if err := e.ensureNamespace(ctx); err != nil {
		return "", fmt.Errorf("ensuring namespace: %w", err)
	}
	rbac := &actions.RBACConfig{
		ClusterScoped: true,
		Rules:         def.RBAC(e.targetType),
	}
	saName := breakglassResourceName(requestID)
	if err := e.createBreakglassResourcesIdempotent(ctx, requestID, saName, rbac); err != nil {
		e.cleanupBreakglassResources(ctx, requestID, saName, rbac)
		return "", err
	}
	return saName, nil
}

// CleanupBreakglass removes K8s resources for a break-glass grant (worker GC).
func (e *Executor) CleanupBreakglass(ctx context.Context, requestID string) {
	saName := breakglassResourceName(requestID)
	rbac := &actions.RBACConfig{ClusterScoped: true}
	e.cleanupBreakglassResources(ctx, requestID, saName, rbac)
}

func (e *Executor) createBreakglassResourcesIdempotent(ctx context.Context, requestID, saName string, rbac *actions.RBACConfig) error {
	if rbac != nil {
		if err := actions.ValidateRBACRules(rbac); err != nil {
			return fmt.Errorf("RBAC validation failed: %w", err)
		}
	}
	objectLabels := breakglassManagedLabels(requestID)
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      saName,
			Namespace: jobNamespace,
			Labels:    objectLabels,
		},
	}
	if err := e.withRetry(ctx, "create-breakglass-service-account", func() error {
		_, err := e.kubeClient.CoreV1().ServiceAccounts(jobNamespace).Create(ctx, sa, metav1.CreateOptions{})
		return err
	}); err != nil {
		return fmt.Errorf("creating service account: %w", err)
	}

	if rbac == nil {
		return nil
	}

	rules := make([]rbacv1.PolicyRule, 0, len(rbac.Rules))
	for _, r := range rbac.Rules {
		rules = append(rules, rbacv1.PolicyRule{
			APIGroups: r.APIGroups,
			Resources: r.Resources,
			Verbs:     r.Verbs,
		})
	}

	roleName := breakglassResourceName(requestID)
	if rbac.ClusterScoped {
		cr := &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name:   roleName,
				Labels: objectLabels,
			},
			Rules: rules,
		}
		if err := e.withRetry(ctx, "create-breakglass-cluster-role", func() error {
			_, err := e.kubeClient.RbacV1().ClusterRoles().Create(ctx, cr, metav1.CreateOptions{})
			return err
		}); err != nil {
			return fmt.Errorf("creating cluster role: %w", err)
		}

		crb := &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:   roleName,
				Labels: objectLabels,
			},
			Subjects: []rbacv1.Subject{{
				Kind:      "ServiceAccount",
				Name:      saName,
				Namespace: jobNamespace,
			}},
			RoleRef: rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "ClusterRole",
				Name:     roleName,
			},
		}
		if err := e.withRetry(ctx, "create-breakglass-cluster-role-binding", func() error {
			_, err := e.kubeClient.RbacV1().ClusterRoleBindings().Create(ctx, crb, metav1.CreateOptions{})
			return err
		}); err != nil {
			return fmt.Errorf("creating cluster role binding: %w", err)
		}
	}

	return nil
}

func (e *Executor) cleanupBreakglassResources(ctx context.Context, requestID, saName string, rbac *actions.RBACConfig) {
	logger := e.logger.With("request_id", requestID)

	if err := e.kubeClient.CoreV1().ServiceAccounts(jobNamespace).Delete(ctx, saName, metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
		logger.Warn("failed to delete breakglass service account", "error", err)
	}

	if rbac == nil {
		return
	}

	roleName := breakglassResourceName(requestID)
	if rbac.ClusterScoped {
		if err := e.kubeClient.RbacV1().ClusterRoleBindings().Delete(ctx, roleName, metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
			logger.Warn("failed to delete breakglass cluster role binding", "error", err)
		}
		if err := e.kubeClient.RbacV1().ClusterRoles().Delete(ctx, roleName, metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
			logger.Warn("failed to delete breakglass cluster role", "error", err)
		}
	}
}
