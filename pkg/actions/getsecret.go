package actions

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/labels"
)

func init() {
	Register(&getSecretAction{})
}

var blockedNamespacePrefixes = []string{
	labels.ClusterNamespacePrefix,
}

var blockedSecretNamePatterns = []string{
	"kubeconfig",
	"kubeadmin",
	"pull-secret",
	"etcd-encryption",
}

type getSecretAction struct{}

func (a *getSecretAction) Metadata() ActionMetadata {
	return ActionMetadata{
		Name:              "get_secret",
		Scope:             "kube-api",
		Type:              "read",
		ExecutionMode:     "sync",
		Description:       "Get Kubernetes secrets with HCP namespace protection. Shows secret metadata and data keys by default; use verbose for base64 values.",
		Authorization:     AuthorizationConfig{Approval: "none"},
		DeploymentTargets: []string{DeploymentTargetRC, DeploymentTargetMC},
		TimeoutSeconds:    60,
		Examples: []string{
			"zoa run get_secret -n openshift-config --name pull-secret --jira ROSAENG-1234",
			"zoa run get_secret -n grafana -l app=grafana --jira ROSAENG-1234",
		},
		Parameters: []ParameterDef{
			{Name: "namespace", Required: true, Description: "Target namespace (HCP namespaces blocked)"},
			{Name: "name", Required: false, Description: "Secret name (omit to list all secrets in namespace)"},
			{Name: "label_selector", Required: false, Description: "Label selector to filter secrets"},
			{Name: "verbose", Required: false, Default: "false", Description: "Include base64-encoded data values (default: keys only)"},
		},
		RBAC: &RBACConfig{
			ClusterScoped:   false,
			NamespaceParam:  "namespace",
			AllowSecretRead: true,
			Rules: []RBACRule{
				{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"get", "list"}},
			},
		},
	}
}

func (a *getSecretAction) Validate(_ context.Context, params *ExecutionParams) error {
	ns := params.Params["namespace"]
	if ns == "" {
		return fmt.Errorf("namespace is required")
	}

	if isBlockedNamespace(ns) {
		return fmt.Errorf("access to secrets in namespace %q is blocked: HCP namespace protection", ns)
	}

	name := params.Params["name"]
	if name != "" && isBlockedSecretName(name) {
		return fmt.Errorf("access to secret %q is blocked: sensitive HCP secret", name)
	}

	return nil
}

func (a *getSecretAction) Execute(ctx context.Context, params *ExecutionParams) (*ActionResult, error) {
	ns := params.Params["namespace"]
	name := params.Params["name"]
	verbose := params.Params["verbose"] == "true"
	labelSelector := params.Params["label_selector"]

	client := params.KubeClient

	if name != "" {
		secret, err := client.CoreV1().Secrets(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("getting secret %s/%s: %w", ns, name, err)
		}

		if isBlockedSecretName(secret.Name) {
			return nil, fmt.Errorf("access to secret %q is blocked: sensitive HCP secret", secret.Name)
		}

		output := secretToOutput(secret.Name, secret.Namespace, string(secret.Type), secret.Data, secret.Labels, verbose)
		return &ActionResult{
			Success: true,
			Output:  output,
			Summary: fmt.Sprintf("Secret %s/%s (type: %s, keys: %d)", ns, name, secret.Type, len(secret.Data)),
		}, nil
	}

	opts := metav1.ListOptions{}
	if labelSelector != "" {
		opts.LabelSelector = labelSelector
	}

	secrets, err := client.CoreV1().Secrets(ns).List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("listing secrets in %s: %w", ns, err)
	}

	var results []map[string]interface{}
	for _, s := range secrets.Items {
		if isBlockedSecretName(s.Name) {
			continue
		}
		results = append(results, secretToOutput(s.Name, s.Namespace, string(s.Type), s.Data, s.Labels, verbose))
	}

	return &ActionResult{
		Success: true,
		Output:  results,
		Summary: fmt.Sprintf("Found %d secrets in namespace %s", len(results), ns),
	}, nil
}

func secretToOutput(name, namespace, secretType string, data map[string][]byte, labels map[string]string, verbose bool) map[string]interface{} {
	output := map[string]interface{}{
		"name":      name,
		"namespace": namespace,
		"type":      secretType,
	}

	if len(labels) > 0 {
		output["labels"] = labels
	}

	if verbose {
		dataMap := make(map[string]string, len(data))
		for k, v := range data {
			dataMap[k] = string(v)
		}
		output["data"] = dataMap
	} else {
		keys := make([]string, 0, len(data))
		for k := range data {
			keys = append(keys, k)
		}
		output["keys"] = keys
		output["key_count"] = len(keys)
	}

	return output
}

func isBlockedNamespace(ns string) bool {
	for _, prefix := range blockedNamespacePrefixes {
		if strings.HasPrefix(ns, prefix) {
			return true
		}
	}
	return false
}

func isBlockedSecretName(name string) bool {
	lower := strings.ToLower(name)
	for _, pattern := range blockedSecretNamePatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}
