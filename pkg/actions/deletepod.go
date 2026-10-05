package actions

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

func init() {
	Register(&deletePod{})
}

type deletePod struct{}

func (d *deletePod) Metadata() ActionMetadata {
	return ActionMetadata{
		Name:                 "delete_pod",
		Scope:                "kube-api",
		Type:                 "write",
		ExecutionMode:        "sync",
		Description:          "Delete a pod and wait for it to terminate. Refuses to delete standalone pods without owner references.",
		Authorization:        AuthorizationConfig{Approval: "none"},
		DeploymentTargets:    []string{DeploymentTargetRC, DeploymentTargetMC},
		TimeoutSeconds:       60,
		WriteCooldownSeconds: 60,
		DryRunAction:         "get_resource",
		DryRunExtraParams:    Params{"resource": "pods"},
		Examples: []string{
			"zoa run delete_pod -n openshift-ingress --name router-default-abc12 --jira ROSAENG-1234",
			"zoa run delete_pod -n cert-manager --name cert-manager-webhook-xyz --jira ROSAENG-1234 --dry-run",
		},
		Parameters: []ParameterDef{
			{Name: "namespace", Required: true, Description: "Pod namespace"},
			{Name: "name", Required: true, Description: "Pod name"},
		},
		RBAC: &RBACConfig{
			NamespaceParam: "namespace",
			Rules: []RBACRule{
				{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "delete", "watch"}},
			},
		},
	}
}

func (d *deletePod) Validate(ctx context.Context, params *ExecutionParams) error {
	if err := ValidateRequiredParams(d.Metadata(), params.Params); err != nil {
		return err
	}

	ns := params.Params["namespace"]
	name := params.Params["name"]

	pod, err := params.KubeClient.CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("pod %s/%s not found: %w", ns, name, err)
	}

	if len(pod.OwnerReferences) == 0 && !params.Force {
		return fmt.Errorf("refusing to delete standalone pod %s/%s (no owner references) — use --force to override", ns, name)
	}

	return nil
}

func (d *deletePod) Execute(ctx context.Context, params *ExecutionParams) (*ActionResult, error) {
	ns := params.Params["namespace"]
	name := params.Params["name"]

	params.Logger.Info("deleting pod", "namespace", ns, "name", name)

	watcher, err := params.KubeClient.CoreV1().Pods(ns).Watch(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("metadata.name=%s", name),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to watch pod %s/%s: %w", ns, name, err)
	}
	defer watcher.Stop()

	if err := params.KubeClient.CoreV1().Pods(ns).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		return nil, fmt.Errorf("failed to delete pod %s/%s: %w", ns, name, err)
	}

	for {
		select {
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return nil, fmt.Errorf("watch channel closed while waiting for pod %s/%s deletion", ns, name)
			}
			if event.Type == watch.Deleted {
				params.Logger.Info("pod deleted", "namespace", ns, "name", name)
				return &ActionResult{
					Success: true,
					Output:  map[string]string{"status": "deleted"},
					AffectedResources: []AffectedResource{
						{Kind: "Pod", Namespace: ns, Name: name, Action: "deleted"},
					},
					Summary: fmt.Sprintf("Pod %s/%s deleted successfully", ns, name),
				}, nil
			}
		case <-ctx.Done():
			return &ActionResult{
				Success: true,
				Output:  map[string]string{"status": "delete-initiated"},
				AffectedResources: []AffectedResource{
					{Kind: "Pod", Namespace: ns, Name: name, Action: "deleted"},
				},
				Summary: fmt.Sprintf("Pod %s/%s deletion initiated; pod still terminating when deadline reached", ns, name),
			}, nil
		}
	}
}
