package parambind

import (
	"strings"

	"github.com/spf13/cobra"
)

// RunTAParams holds zoa run flags that map to Trusted Action API parameter keys.
type RunTAParams struct {
	Namespace     string
	ClusterID     string
	Gather        string
	AllNamespaces bool
	LabelSelector string
	Verbose       bool
	Name          string
	Resource      string
}

type taParamKind int

const (
	taParamString   taParamKind = 0
	taParamBoolTrue taParamKind = 1 // API value "true" when flag is set
)

type taParamBinding struct {
	apiParam    string
	flagName    string
	shorthand   string
	describeCLI string
	shortHelp   string
	kind        taParamKind
	apply       func(p *RunTAParams) (string, bool)
	register    func(cmd *cobra.Command, p *RunTAParams, help string)
}

// taParamBindings is the single source of truth for TA param ↔ CLI flag ↔ API map keys.
var taParamBindings = []taParamBinding{
	{
		apiParam:    "namespace",
		flagName:    "namespace",
		shorthand:   "n",
		describeCLI: "-n, --namespace",
		shortHelp:   "Target namespace (omit for cluster-scoped resources)",
		kind:        taParamString,
		apply: func(p *RunTAParams) (string, bool) {
			if p.Namespace != "" {
				return p.Namespace, true
			}
			return "", false
		},
		register: func(cmd *cobra.Command, p *RunTAParams, help string) {
			cmd.Flags().StringVarP(&p.Namespace, "namespace", "n", "", help)
		},
	},
	{
		apiParam:    "cluster_id",
		flagName:    "cluster-id",
		describeCLI: "--cluster-id",
		shortHelp:   "Hosted cluster UUID (must_gather when --gather includes hcp)",
		kind:        taParamString,
		apply: func(p *RunTAParams) (string, bool) {
			if p.ClusterID != "" {
				return p.ClusterID, true
			}
			return "", false
		},
		register: func(cmd *cobra.Command, p *RunTAParams, help string) {
			cmd.Flags().StringVar(&p.ClusterID, "cluster-id", "", help)
		},
	},
	{
		apiParam:    "gather",
		flagName:    "gather",
		describeCLI: "--gather",
		shortHelp:   "must_gather scopes: hcp, mc, rc (required for that TA; must match this endpoint)",
		kind:        taParamString,
		apply: func(p *RunTAParams) (string, bool) {
			if p.Gather != "" {
				return p.Gather, true
			}
			return "", false
		},
		register: func(cmd *cobra.Command, p *RunTAParams, help string) {
			cmd.Flags().StringVar(&p.Gather, "gather", "", help)
		},
	},
	{
		apiParam:    "all_namespaces",
		flagName:    "all-namespaces",
		shorthand:   "A",
		describeCLI: "-A, --all-namespaces (sets true)",
		shortHelp:   "List resources across all namespaces",
		kind:        taParamBoolTrue,
		apply: func(p *RunTAParams) (string, bool) {
			if p.AllNamespaces {
				return "true", true
			}
			return "", false
		},
		register: func(cmd *cobra.Command, p *RunTAParams, help string) {
			cmd.Flags().BoolVarP(&p.AllNamespaces, "all-namespaces", "A", false, help)
		},
	},
	{
		apiParam:    "label_selector",
		flagName:    "selector",
		shorthand:   "l",
		describeCLI: "-l, --selector",
		shortHelp:   "Kubernetes label selector (e.g. app=nginx)",
		kind:        taParamString,
		apply: func(p *RunTAParams) (string, bool) {
			if p.LabelSelector != "" {
				return p.LabelSelector, true
			}
			return "", false
		},
		register: func(cmd *cobra.Command, p *RunTAParams, help string) {
			cmd.Flags().StringVarP(&p.LabelSelector, "selector", "l", "", help)
		},
	},
	{
		apiParam:    "verbose",
		flagName:    "verbose",
		shorthand:   "v",
		describeCLI: "-v, --verbose (sets true)",
		shortHelp:   "Return full API objects instead of a compact table summary",
		kind:        taParamBoolTrue,
		apply: func(p *RunTAParams) (string, bool) {
			if p.Verbose {
				return "true", true
			}
			return "", false
		},
		register: func(cmd *cobra.Command, p *RunTAParams, help string) {
			cmd.Flags().BoolVarP(&p.Verbose, "verbose", "v", false, help)
		},
	},
	{
		apiParam:    "name",
		flagName:    "name",
		describeCLI: "--name",
		shortHelp:   "Resource name (get one object instead of a list)",
		kind:        taParamString,
		apply: func(p *RunTAParams) (string, bool) {
			if p.Name != "" {
				return p.Name, true
			}
			return "", false
		},
		register: func(cmd *cobra.Command, p *RunTAParams, help string) {
			cmd.Flags().StringVar(&p.Name, "name", "", help)
		},
	},
	{
		apiParam:    "resource",
		flagName:    "resource",
		describeCLI: "--resource",
		shortHelp:   "Kubernetes resource type (e.g. pods, deployments, nodes)",
		kind:        taParamString,
		apply: func(p *RunTAParams) (string, bool) {
			if p.Resource != "" {
				return p.Resource, true
			}
			return "", false
		},
		register: func(cmd *cobra.Command, p *RunTAParams, help string) {
			cmd.Flags().StringVar(&p.Resource, "resource", "", help)
		},
	},
}

// TAParamBindings returns the registered TA parameter bindings (for tests).
func TAParamBindings() []struct {
	APIParam, FlagName, DescribeCLI, ShortHelp string
} {
	out := make([]struct {
		APIParam, FlagName, DescribeCLI, ShortHelp string
	}, len(taParamBindings))
	for i, b := range taParamBindings {
		out[i] = struct {
			APIParam, FlagName, DescribeCLI, ShortHelp string
		}{b.apiParam, b.flagName, b.describeCLI, b.shortHelp}
	}
	return out
}

// ParamFlagHelp returns Cobra help for a dedicated TA parameter flag.
func ParamFlagHelp(flagName string) string {
	for _, b := range taParamBindings {
		if b.flagName == flagName {
			return b.shortHelp
		}
	}
	return ""
}

// RegisterTAParamFlags wires dedicated TA parameter flags on zoa run.
func RegisterTAParamFlags(cmd *cobra.Command, p *RunTAParams) {
	for _, b := range taParamBindings {
		b.register(cmd, p, b.shortHelp)
	}
}

// ToAPIParams builds the dispatch params map from dedicated flags and --param extras.
// Dedicated flags win over duplicate keys in extras.
func ToAPIParams(p RunTAParams, extras []string) map[string]string {
	params := make(map[string]string)
	for _, b := range taParamBindings {
		if v, ok := b.apply(&p); ok {
			params[b.apiParam] = v
		}
	}
	for _, raw := range extras {
		key, val, ok := strings.Cut(raw, "=")
		if !ok {
			continue
		}
		if _, exists := params[key]; !exists {
			params[key] = val
		}
	}
	if len(params) == 0 {
		return nil
	}
	return params
}

func describeCLILine(apiParam string) string {
	for _, b := range taParamBindings {
		if b.apiParam == apiParam {
			return b.describeCLI
		}
	}
	return ""
}
