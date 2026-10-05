package actions

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func init() {
	Register(&rolloutRestart{})
}

const restartAnnotation = "kubectl.kubernetes.io/restartedAt"

var supportedRolloutResources = map[string]bool{
	"deployment":  true,
	"daemonset":   true,
	"statefulset": true,
}

type rolloutRestart struct{}

func (r *rolloutRestart) Metadata() ActionMetadata {
	return ActionMetadata{
		Name:                 "rollout_restart",
		Scope:                "kube-api",
		Type:                 "write",
		ExecutionMode:        "sync",
		Description:          "Restart a workload by patching the pod template annotation, equivalent to kubectl rollout restart. Supports deployments, daemonsets, and statefulsets.",
		Authorization:        AuthorizationConfig{Approval: "none"},
		DeploymentTargets:    []string{DeploymentTargetRC, DeploymentTargetMC},
		TimeoutSeconds:       180,
		WriteCooldownSeconds: 300,
		DryRunAction:         "get_resource",
		Examples: []string{
			"zoa run rollout_restart --resource deployment -n openshift-ingress --name router-default --jira ROSAENG-1234",
			"zoa run rollout_restart --resource deployment -n cert-manager --name cert-manager-webhook --jira ROSAENG-1234 --dry-run",
		},
		Parameters: []ParameterDef{
			{Name: "resource", Required: true, Default: "deployment", Description: "Resource type (deployment, daemonset, statefulset)"},
			{Name: "namespace", Required: true, Description: "Workload namespace"},
			{Name: "name", Required: true, Description: "Workload name"},
		},
		RBAC: &RBACConfig{
			NamespaceParam: "namespace",
			Rules: []RBACRule{
				{APIGroups: []string{"apps"}, Resources: []string{"deployments", "daemonsets", "statefulsets"}, Verbs: []string{"get", "patch"}},
			},
		},
	}
}

func (r *rolloutRestart) Validate(ctx context.Context, params *ExecutionParams) error {
	if err := ValidateRequiredParams(r.Metadata(), params.Params); err != nil {
		return err
	}

	resource := params.Params["resource"]
	ns := params.Params["namespace"]
	name := params.Params["name"]

	if !supportedRolloutResources[resource] {
		return fmt.Errorf("unsupported resource %q — must be deployment, daemonset, or statefulset", resource)
	}
	if params.KubeClient == nil {
		return fmt.Errorf("kubernetes client is required")
	}

	switch resource {
	case "deployment":
		deploy, err := params.KubeClient.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("deployment %s/%s not found: %w", ns, name, err)
		}
		if deploy.Spec.Replicas != nil && *deploy.Spec.Replicas == 0 {
			return fmt.Errorf("refusing to restart deployment %s/%s — it has 0 replicas", ns, name)
		}
	case "daemonset":
		if _, err := params.KubeClient.AppsV1().DaemonSets(ns).Get(ctx, name, metav1.GetOptions{}); err != nil {
			return fmt.Errorf("daemonset %s/%s not found: %w", ns, name, err)
		}
	case "statefulset":
		sts, err := params.KubeClient.AppsV1().StatefulSets(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("statefulset %s/%s not found: %w", ns, name, err)
		}
		if sts.Spec.Replicas != nil && *sts.Spec.Replicas == 0 {
			return fmt.Errorf("refusing to restart statefulset %s/%s — it has 0 replicas", ns, name)
		}
	}

	return nil
}

func (r *rolloutRestart) Execute(ctx context.Context, params *ExecutionParams) (*ActionResult, error) {
	resource := params.Params["resource"]
	ns := params.Params["namespace"]
	name := params.Params["name"]

	params.Logger.Info("restarting workload", "resource", resource, "namespace", ns, "name", name)

	restartTime := time.Now().UTC().Format(time.RFC3339)
	patch := fmt.Sprintf(
		`{"spec":{"template":{"metadata":{"annotations":{%q:%q}}}}}`,
		restartAnnotation, restartTime,
	)
	patchBytes := []byte(patch)

	switch resource {
	case "deployment":
		_, err := params.KubeClient.AppsV1().Deployments(ns).Patch(
			ctx, name, types.StrategicMergePatchType, patchBytes, metav1.PatchOptions{},
		)
		if err != nil {
			return nil, fmt.Errorf("failed to patch deployment %s/%s: %w", ns, name, err)
		}
	case "daemonset":
		_, err := params.KubeClient.AppsV1().DaemonSets(ns).Patch(
			ctx, name, types.StrategicMergePatchType, patchBytes, metav1.PatchOptions{},
		)
		if err != nil {
			return nil, fmt.Errorf("failed to patch daemonset %s/%s: %w", ns, name, err)
		}
	case "statefulset":
		_, err := params.KubeClient.AppsV1().StatefulSets(ns).Patch(
			ctx, name, types.StrategicMergePatchType, patchBytes, metav1.PatchOptions{},
		)
		if err != nil {
			return nil, fmt.Errorf("failed to patch statefulset %s/%s: %w", ns, name, err)
		}
	}

	params.Logger.Info("waiting for rollout", "resource", resource, "namespace", ns, "name", name)

	kind := resource
	if err := r.waitForRollout(ctx, params, resource, ns, name); err != nil {
		return &ActionResult{
			Success: true,
			Output: map[string]string{
				"status":       "restart-initiated",
				"restartedAt":  restartTime,
				"rolloutReady": "false",
			},
			AffectedResources: []AffectedResource{
				{Kind: kind, Namespace: ns, Name: name, Action: "restarted"},
			},
			Summary: fmt.Sprintf("%s %s/%s restart initiated but rollout not yet complete: %v", resource, ns, name, err),
		}, nil
	}

	return &ActionResult{
		Success: true,
		Output: map[string]string{
			"status":       "restarted",
			"restartedAt":  restartTime,
			"rolloutReady": "true",
		},
		AffectedResources: []AffectedResource{
			{Kind: kind, Namespace: ns, Name: name, Action: "restarted"},
		},
		Summary: fmt.Sprintf("%s %s/%s restarted successfully", resource, ns, name),
	}, nil
}

func (r *rolloutRestart) waitForRollout(ctx context.Context, params *ExecutionParams, resource, ns, name string) error {
	timeout := time.After(5 * time.Minute)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ready, err := r.isReady(ctx, params, resource, ns, name)
			if err != nil {
				return fmt.Errorf("failed to check rollout status: %w", err)
			}
			if ready {
				return nil
			}
		case <-timeout:
			return fmt.Errorf("timed out waiting for rollout to complete")
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (r *rolloutRestart) isReady(ctx context.Context, params *ExecutionParams, resource, ns, name string) (bool, error) {
	switch resource {
	case "deployment":
		deploy, err := params.KubeClient.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return isDeploymentRolloutComplete(deploy), nil
	case "daemonset":
		ds, err := params.KubeClient.AppsV1().DaemonSets(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return ds.Status.UpdatedNumberScheduled == ds.Status.DesiredNumberScheduled &&
			ds.Status.NumberReady == ds.Status.DesiredNumberScheduled &&
			ds.Generation == ds.Status.ObservedGeneration, nil
	case "statefulset":
		sts, err := params.KubeClient.AppsV1().StatefulSets(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		desired := int32(1)
		if sts.Spec.Replicas != nil {
			desired = *sts.Spec.Replicas
		}
		return sts.Status.UpdatedReplicas == desired &&
			sts.Status.ReadyReplicas == desired &&
			sts.Generation == sts.Status.ObservedGeneration, nil
	default:
		return false, fmt.Errorf("unsupported resource: %s", resource)
	}
}

func isDeploymentRolloutComplete(deploy *appsv1.Deployment) bool {
	if deploy.Generation > deploy.Status.ObservedGeneration {
		return false
	}
	desired := int32(1)
	if deploy.Spec.Replicas != nil {
		desired = *deploy.Spec.Replicas
	}
	return deploy.Status.UpdatedReplicas == desired &&
		deploy.Status.ReadyReplicas == desired &&
		deploy.Status.AvailableReplicas == desired
}
