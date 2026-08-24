package actions

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type Action interface {
	Metadata() ActionMetadata
	Validate(ctx context.Context, params *ExecutionParams) error
	Execute(ctx context.Context, params *ExecutionParams) (*ActionResult, error)
}

// Params is a shorthand for map[string]string, used for DryRunExtraParams
// to keep TA definitions concise.
type Params = map[string]string

type ActionMetadata struct {
	Name                 string              `json:"name"`
	Scope                string              `json:"scope"`
	Type                 string              `json:"type"`
	ExecutionMode        string              `json:"execution_mode"`
	Description          string              `json:"description"`
	Parameters           []ParameterDef      `json:"parameters"`
	Authorization        AuthorizationConfig `json:"authorization"`
	TimeoutSeconds       int                 `json:"timeout_seconds"`
	WriteCooldownSeconds int                 `json:"write_cooldown_seconds"`
	DryRunAction         string              `json:"dry_run_action,omitempty"`
	DryRunExtraParams    Params              `json:"dry_run_extra_params,omitempty"`
	RBAC                 *RBACConfig         `json:"rbac,omitempty"`
}

type AuthorizationConfig struct {
	Approval string `json:"approval"`
}

type ParameterDef struct {
	Name        string `json:"name"`
	Required    bool   `json:"required"`
	Default     string `json:"default,omitempty"`
	Description string `json:"description"`
}

type RBACConfig struct {
	ClusterScoped   bool       `json:"cluster_scoped"`
	NamespaceParam  string     `json:"namespace_param,omitempty"`
	AllowSecretRead bool       `json:"allow_secret_read,omitempty"`
	Rules           []RBACRule `json:"rules"`
}

type RBACRule struct {
	APIGroups []string `json:"api_groups"`
	Resources []string `json:"resources"`
	Verbs     []string `json:"verbs"`
}

type ExecutionParams struct {
	Params        map[string]string
	ExecutionID   string
	TargetCluster string
	Force         bool
	KubeClient    kubernetes.Interface
	DynamicClient dynamic.Interface
	RESTConfig    *rest.Config
	AWSConfig     *aws.Config
	Logger        *slog.Logger
}

type ActionResult struct {
	Success           bool               `json:"success"`
	Output            interface{}        `json:"output"`
	AffectedResources []AffectedResource `json:"affected_resources,omitempty"`
	Summary           string             `json:"summary"`
}

type AffectedResource struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
	Action    string `json:"action"`
}

// ValidateRequiredParams checks that all required parameters defined in metadata
// are present and non-empty. Call this at the top of Validate() to avoid
// repetitive manual checks.
func ValidateRequiredParams(meta ActionMetadata, params map[string]string) error {
	for _, p := range meta.Parameters {
		if p.Required && params[p.Name] == "" {
			return fmt.Errorf("parameter %q is required", p.Name)
		}
	}
	return nil
}

// ApplyDefaults fills in default values for any missing optional parameters.
// Call this before Validate() or Execute() to ensure defaults are applied.
func ApplyDefaults(meta ActionMetadata, params map[string]string) {
	for _, p := range meta.Parameters {
		if p.Default != "" {
			if _, exists := params[p.Name]; !exists {
				params[p.Name] = p.Default
			}
		}
	}
}
