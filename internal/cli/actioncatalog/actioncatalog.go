// Package actioncatalog builds offline Trusted Action views from the embedded pkg/actions registry.
package actioncatalog

import (
	"fmt"
	"strings"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/cli/parambind"
	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/client"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/actions"
)

// OfflineNotice is printed to stderr when using --offline catalog/actions output.
const OfflineNotice = "Offline catalog: embedded in this zoa CLI build (filtered by target type). " +
	"The live API is the source of truth when connected — run `zoa actions` or `zoa describe` without --offline to verify."

// List returns API-shaped action metadata for the given target type (rc or mc).
func List(targetType string) ([]client.Action, error) {
	actions.SetDeploymentTarget(targetType)
	defer actions.SetDeploymentTarget("")

	registered := actions.List()
	out := make([]client.Action, 0, len(registered))
	for _, a := range registered {
		out = append(out, ClientActionFromMetadata(a.Metadata()))
	}
	return out, nil
}

// Get returns one action from the embedded registry for targetType, or an error if unknown/hidden.
func Get(targetType, name string) (*client.Action, error) {
	actions.SetDeploymentTarget(targetType)
	defer actions.SetDeploymentTarget("")

	a, ok := actions.Get(name)
	if !ok {
		return nil, fmt.Errorf("action %q not found for target type %q", name, targetType)
	}
	action := ClientActionFromMetadata(a.Metadata())
	return &action, nil
}

// Markdown renders the boundary / Claude catalog document for targetType.
func Markdown(targetType string) (string, error) {
	actions.SetDeploymentTarget(targetType)
	defer actions.SetDeploymentTarget("")

	var b strings.Builder
	b.WriteString("# ZOA Trusted Actions catalog\n\n")
	fmt.Fprintf(&b, "**Target type:** `%s` (TYPE column in `zoa targets`; offline catalog from this CLI build).\n\n", targetType)
	writeEnvironmentContext(&b, targetType)
	b.WriteString("Use `zoa describe <action>` when connected for the live API view, or `zoa run` with the flags below.\n\n")
	b.WriteString("## Quick rules\n\n")
	b.WriteString("- Every `zoa run` **must** include `--jira TICKET`.\n")
	b.WriteString("- Parameters map to CLI flags where listed; otherwise `--param name=value`.\n")
	b.WriteString("- **kube-api** TAs talk to **this session's EKS cluster** (RC or MC). **aws-api** TAs talk to **that cluster's AWS account/region**.\n")
	b.WriteString("- Clusters are **EKS**, not in-cluster OpenShift control planes — discover namespaces with `get_resource --resource namespaces` before guessing.\n\n")

	list := actions.List()
	if len(list) == 0 {
		b.WriteString("_No actions registered for this target type._\n")
		return b.String(), nil
	}

	for _, a := range list {
		meta := a.Metadata()
		targets := strings.Join(meta.DeploymentTargets, ", ")
		fmt.Fprintf(&b, "## %s\n\n", meta.Name)
		fmt.Fprintf(&b, "- **Scope:** %s · **Type:** %s · **Mode:** %s\n", meta.Scope, meta.Type, meta.ExecutionMode)
		fmt.Fprintf(&b, "- **Also on:** %s\n", targets)
		fmt.Fprintf(&b, "- %s\n\n", meta.Description)

		if len(meta.Examples) > 0 {
			b.WriteString("**Examples:**\n\n")
			for _, ex := range meta.Examples {
				fmt.Fprintf(&b, "```bash\n%s\n```\n\n", ex)
			}
		}

		if len(meta.Parameters) > 0 {
			b.WriteString("| Parameter | Required | CLI |\n|-----------|----------|-----|\n")
			for _, p := range meta.Parameters {
				req := ""
				if p.Required {
					req = "yes"
				}
				fmt.Fprintf(&b, "| `%s` | %s | %s |\n", p.Name, req, parambind.CLILineForParam(p.Name))
			}
			b.WriteString("\n")
		}
	}
	return b.String(), nil
}

// ClientActionFromMetadata maps registry metadata to the API/CLI Action shape.
func ClientActionFromMetadata(m actions.ActionMetadata) client.Action {
	params := make([]client.ActionParam, 0, len(m.Parameters))
	for _, p := range m.Parameters {
		params = append(params, client.ActionParam{
			Name:        p.Name,
			Description: p.Description,
			Required:    p.Required,
			Default:     p.Default,
		})
	}
	return client.Action{
		Name:                 m.Name,
		Scope:                m.Scope,
		Type:                 m.Type,
		ExecutionMode:        m.ExecutionMode,
		Description:          m.Description,
		Examples:             m.Examples,
		Params:               params,
		Authorization:        client.ActionAuthorization{Approval: m.Authorization.Approval},
		DryRunAction:         m.DryRunAction,
		WriteCooldownSeconds: m.WriteCooldownSeconds,
		TimeoutSeconds:       m.TimeoutSeconds,
	}
}

func writeEnvironmentContext(b *strings.Builder, targetType string) {
	b.WriteString("## Environment context (HyperFleet)\n\n")
	b.WriteString("ROSA HyperFleet runs **Amazon EKS** for platform clusters. ")
	switch targetType {
	case actions.DeploymentTargetRC:
		b.WriteString("This catalog is for a **Regional Cluster (RC)** boundary: one RC per region per deployment, running regional services (Platform API, hyperfleet-operator, GitOps, observability).\n\n")
		b.WriteString("**Discovery example** (find where a component runs):\n\n")
		b.WriteString("```bash\nzoa run get_resource --resource namespaces --jira TICKET\nzoa run get_resource --resource pods -n platform-api --jira TICKET\n```\n\n")
		b.WriteString("**Namespaces often relevant on RC (hints only):** `platform-api`, `hyperfleet`, `argocd`, `thanos`, `grafana`, `loki`, `monitoring`, `vector`, `cert-manager`.\n\n")
	case actions.DeploymentTargetMC:
		b.WriteString("This catalog is for a **Management Cluster (MC)** boundary: a region may have **multiple MCs**; this session is attached to **one** MC EKS cluster running HyperShift and hosted control planes.\n\n")
		b.WriteString("**Discovery example:**\n\n")
		b.WriteString("```bash\nzoa run get_resource --resource namespaces --jira TICKET\nzoa run get_resource --resource pods -n hypershift --jira TICKET\n```\n\n")
		b.WriteString("**Namespaces often relevant on MC (hints only):** `hypershift`, `hypershift-install`, `kube-applier`, `vector`, `external-secrets`, and customer HCP namespaces `cluster-*`. `get_secret` refuses HCP namespaces.\n\n")
	default:
		b.WriteString("Unknown target type.\n\n")
	}
	b.WriteString("**aws-api** read TAs (`list_eks_clusters`, `describe_vpc_endpoint`, …) operate in the **AWS account/region where this RC or MC EKS cluster lives**, not in customer worker accounts.\n\n")
}
