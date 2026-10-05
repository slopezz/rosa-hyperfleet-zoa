package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
)

func init() {
	Register(&getResource{})
}

type getResource struct{}

func (g *getResource) Metadata() ActionMetadata {
	return ActionMetadata{
		Name:              "get_resource",
		Scope:             "kube-api",
		Type:              "read",
		ExecutionMode:     "sync",
		Description:       "Get or list Kubernetes resources by type, namespace, name, or label/field selectors (including CRDs). Does not support Secrets — use get_secret instead (HCP namespace protection).",
		Authorization:     AuthorizationConfig{Approval: "none"},
		DeploymentTargets: []string{DeploymentTargetRC, DeploymentTargetMC},
		TimeoutSeconds:    60,
		Examples: []string{
			"zoa run get_resource --resource pods -n openshift-ingress --jira ROSAENG-1234",
			"zoa run get_resource --resource pods -A -l app=nginx --jira ROSAENG-1234",
			"zoa run get_resource --resource nodes --jira ROSAENG-1234",
			"zoa run get_resource --resource events --param field_selector=involvedObject.name=my-pod -n default --jira ROSAENG-1234",
		},
		Parameters: []ParameterDef{
			{Name: "resource", Required: true, Description: "Resource type (e.g. pods, deployments, hostedclusters, CRDs). Not secrets — use get_secret."},
			{Name: "namespace", Description: "Target namespace (omit for cluster-scoped or use all_namespaces)"},
			{Name: "all_namespaces", Default: "false", Description: "List across all namespaces"},
			{Name: "name", Description: "Get a specific resource by name"},
			{Name: "label_selector", Description: "Label selector (e.g. app=nginx)"},
			{Name: "field_selector", Description: "Field selector (e.g. involvedObject.name=my-pod for events)"},
			{Name: "verbose", Default: "false", Description: "Return full API objects instead of server-side table summary"},
		},
		RBAC: &RBACConfig{
			ClusterScoped:  true,
			NamespaceParam: "namespace",
			Rules: []RBACRule{
				{APIGroups: []string{"*"}, Resources: []string{"*"}, Verbs: []string{"get", "list"}},
			},
		},
	}
}

func (g *getResource) Validate(_ context.Context, params *ExecutionParams) error {
	resource := params.Params["resource"]
	if resource == "" {
		return fmt.Errorf("parameter 'resource' is required")
	}

	resource = strings.ToLower(resource)
	params.Params["resource"] = resource

	if resource == "secrets" || resource == "secret" {
		return fmt.Errorf("use the dedicated 'get_secret' action for secrets (it has HCP namespace protection)")
	}

	if params.DynamicClient == nil {
		return fmt.Errorf("dynamic client is required")
	}

	return nil
}

func (g *getResource) Execute(ctx context.Context, params *ExecutionParams) (*ActionResult, error) {
	resource := params.Params["resource"]
	namespace := params.Params["namespace"]
	allNamespaces := params.Params["all_namespaces"] == "true"
	name := params.Params["name"]
	labelSelector := params.Params["label_selector"]
	fieldSelector := params.Params["field_selector"]
	verbose := params.Params["verbose"] == "true"

	gvr, isClusterScoped, err := g.resolveGVR(params, resource)
	if err != nil {
		return nil, err
	}

	params.Logger.Info("getting resources",
		"resource", resource,
		"gvr", gvr.String(),
		"namespace", namespace,
		"name", name,
	)

	var ns string
	switch {
	case isClusterScoped:
		ns = ""
	case allNamespaces:
		ns = ""
	case namespace != "":
		ns = namespace
	default:
		ns = "default"
	}

	if verbose {
		return g.getVerbose(ctx, params, gvr, ns, name, labelSelector, fieldSelector, resource)
	}
	return g.getTable(ctx, params, gvr, ns, name, labelSelector, fieldSelector, resource)
}

// getTable uses the Kubernetes server-side Table API — the same mechanism kubectl uses.
// The API server returns pre-formatted columns for any resource (including CRDs via
// additionalPrinterColumns). No per-resource code needed.
func (g *getResource) getTable(
	ctx context.Context,
	params *ExecutionParams,
	gvr schema.GroupVersionResource,
	namespace, name, labelSelector, fieldSelector, resource string,
) (*ActionResult, error) {
	if params.RESTConfig == nil {
		return g.getVerbose(ctx, params, gvr, namespace, name, labelSelector, fieldSelector, resource)
	}

	cfg := rest.CopyConfig(params.RESTConfig)
	cfg.APIPath = apiPathForGVR(gvr)
	cfg.GroupVersion = &schema.GroupVersion{Group: gvr.Group, Version: gvr.Version}
	if cfg.GroupVersion.Group == "" {
		cfg.APIPath = "/api"
	}
	scheme := runtime.NewScheme()
	cfg.NegotiatedSerializer = serializer.NewCodecFactory(scheme).WithoutConversion()

	restClient, err := rest.RESTClientFor(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating REST client for table request: %w", err)
	}

	req := restClient.Get().
		Resource(gvr.Resource).
		SetHeader("Accept", "application/json;as=Table;v=v1;g=meta.k8s.io,application/json")

	if namespace != "" {
		req = req.Namespace(namespace)
	}
	if name != "" {
		req = req.Name(name)
	}
	if labelSelector != "" {
		req = req.Param("labelSelector", labelSelector)
	}
	if fieldSelector != "" {
		req = req.Param("fieldSelector", fieldSelector)
	}

	resp := req.Do(ctx)
	if err := resp.Error(); err != nil {
		return nil, fmt.Errorf("table request failed: %w", err)
	}

	raw, err := resp.Raw()
	if err != nil {
		return nil, fmt.Errorf("reading table response: %w", err)
	}

	rows, count, err := parseTableResponse(raw)
	if err != nil {
		params.Logger.Warn("table parse failed, falling back to verbose", "error", err)
		return g.getVerbose(ctx, params, gvr, namespace, name, labelSelector, fieldSelector, resource)
	}

	scope := namespace
	if scope == "" {
		scope = "all namespaces"
	}

	var output interface{}
	var summary string
	if name != "" {
		summary = fmt.Sprintf("Retrieved %s/%s", resource, name)
		if len(rows) == 1 {
			output = rows[0]
		} else {
			output = rows
		}
	} else {
		summary = fmt.Sprintf("Found %d %s in %s", count, resource, scope)
		output = rows
	}

	return &ActionResult{
		Success: true,
		Output:  output,
		Summary: summary,
	}, nil
}

// parseTableResponse converts the Kubernetes Table API JSON into a slice of
// maps (one per row) with column names as keys — structured, jq-friendly.
func parseTableResponse(raw []byte) ([]map[string]interface{}, int, error) {
	var table struct {
		Kind              string `json:"kind"`
		ColumnDefinitions []struct {
			Name string `json:"name"`
		} `json:"columnDefinitions"`
		Rows []struct {
			Cells []interface{} `json:"cells"`
		} `json:"rows"`
	}

	if err := json.Unmarshal(raw, &table); err != nil {
		return nil, 0, fmt.Errorf("unmarshal table: %w", err)
	}

	if table.Kind != "Table" {
		return nil, 0, fmt.Errorf("unexpected kind %q (expected Table)", table.Kind)
	}

	rows := make([]map[string]interface{}, 0, len(table.Rows))
	for _, row := range table.Rows {
		entry := make(map[string]interface{}, len(table.ColumnDefinitions))
		for i, col := range table.ColumnDefinitions {
			if i < len(row.Cells) {
				entry[col.Name] = row.Cells[i]
			}
		}
		rows = append(rows, entry)
	}

	return rows, len(rows), nil
}

// getVerbose returns full unstructured objects (fallback when Table API unavailable).
func (g *getResource) getVerbose(
	ctx context.Context,
	params *ExecutionParams,
	gvr schema.GroupVersionResource,
	namespace, name, labelSelector, fieldSelector, resource string,
) (*ActionResult, error) {
	if name != "" {
		var result *unstructured.Unstructured
		var err error
		if namespace != "" {
			result, err = params.DynamicClient.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		} else {
			result, err = params.DynamicClient.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
		}
		if err != nil {
			return nil, fmt.Errorf("failed to get %s/%s: %w", resource, name, err)
		}
		return &ActionResult{
			Success: true,
			Output:  result.Object,
			Summary: fmt.Sprintf("Retrieved %s/%s", resource, name),
		}, nil
	}

	opts := metav1.ListOptions{
		LabelSelector: labelSelector,
		FieldSelector: fieldSelector,
	}
	var list interface{}
	var count int
	var err error

	if namespace != "" {
		result, e := params.DynamicClient.Resource(gvr).Namespace(namespace).List(ctx, opts)
		if e == nil {
			items := make([]map[string]interface{}, len(result.Items))
			for i := range result.Items {
				items[i] = result.Items[i].Object
			}
			list, count = items, len(items)
		}
		err = e
	} else {
		result, e := params.DynamicClient.Resource(gvr).List(ctx, opts)
		if e == nil {
			items := make([]map[string]interface{}, len(result.Items))
			for i := range result.Items {
				items[i] = result.Items[i].Object
			}
			list, count = items, len(items)
		}
		err = e
	}
	if err != nil {
		return nil, fmt.Errorf("failed to list %s: %w", resource, err)
	}

	scope := namespace
	if scope == "" {
		scope = "all namespaces"
	}
	return &ActionResult{
		Success: true,
		Output:  list,
		Summary: fmt.Sprintf("Found %d %s in %s", count, resource, scope),
	}, nil
}

func (g *getResource) resolveGVR(params *ExecutionParams, resource string) (schema.GroupVersionResource, bool, error) {
	if params.KubeClient == nil {
		return schema.GroupVersionResource{}, false, fmt.Errorf("kube client required for resource discovery")
	}

	gvr, namespaced, err := discoverGVR(params.KubeClient.Discovery(), resource)
	if err != nil {
		return schema.GroupVersionResource{}, false, fmt.Errorf("resource %q not found via API discovery: %w", resource, err)
	}
	return gvr, !namespaced, nil
}

// discoverGVR uses the Kubernetes discovery API to resolve any resource name
// (plural, singular, or short name) to its GVR. Works for all resources including CRDs.
// Prefers core API group ("") over extension groups when the same resource name
// exists in multiple groups, matching kubectl's resolution behavior.
func discoverGVR(disco discovery.DiscoveryInterface, resource string) (schema.GroupVersionResource, bool, error) {
	_, apiResourceLists, err := disco.ServerGroupsAndResources()
	if err != nil {
		if !discovery.IsGroupDiscoveryFailedError(err) {
			return schema.GroupVersionResource{}, false, fmt.Errorf("server discovery failed: %w", err)
		}
	}

	resource = strings.ToLower(resource)

	type match struct {
		gvr        schema.GroupVersionResource
		namespaced bool
	}
	var coreMatch, extensionMatch *match

	for _, list := range apiResourceLists {
		gv, parseErr := schema.ParseGroupVersion(list.GroupVersion)
		if parseErr != nil {
			continue
		}
		for _, r := range list.APIResources {
			if strings.Contains(r.Name, "/") {
				continue
			}
			if r.Name == resource || r.SingularName == resource || containsStr(r.ShortNames, resource) {
				m := &match{
					gvr: schema.GroupVersionResource{
						Group:    gv.Group,
						Version:  gv.Version,
						Resource: r.Name,
					},
					namespaced: r.Namespaced,
				}
				if gv.Group == "" {
					coreMatch = m
				} else if extensionMatch == nil {
					extensionMatch = m
				}
			}
		}
	}

	if coreMatch != nil {
		return coreMatch.gvr, coreMatch.namespaced, nil
	}
	if extensionMatch != nil {
		return extensionMatch.gvr, extensionMatch.namespaced, nil
	}

	return schema.GroupVersionResource{}, false, fmt.Errorf("no resource matches %q", resource)
}

func apiPathForGVR(gvr schema.GroupVersionResource) string {
	if gvr.Group == "" {
		return "/api"
	}
	return "/apis"
}

func containsStr(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
