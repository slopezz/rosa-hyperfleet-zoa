// Package catalogdoc renders Trusted Action catalogs for the boundary image (build-time).
package catalogdoc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/cli/parambind"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/actions"
)

// WriteBoundaryCatalogFiles writes ZOA_ACTIONS.rc.md and ZOA_ACTIONS.mc.md under dir.
func WriteBoundaryCatalogFiles(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, target := range []string{actions.DeploymentTargetRC, actions.DeploymentTargetMC} {
		body, err := RenderMarkdown(target)
		if err != nil {
			return err
		}
		path := filepath.Join(dir, "ZOA_ACTIONS."+target+".md")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// RenderMarkdown builds a static catalog for deployment target rc or mc.
func RenderMarkdown(deploymentTarget string) (string, error) {
	actions.SetDeploymentTarget(deploymentTarget)
	defer actions.SetDeploymentTarget("")

	var b strings.Builder
	b.WriteString("# ZOA Trusted Actions catalog\n\n")
	fmt.Fprintf(&b, "**Deployment target:** `%s` (baked at image build from source registry).\n\n", deploymentTarget)
	b.WriteString("Use `zoa describe <action>` inside the boundary for the live API view, or `zoa run` with the flags below.\n\n")
	b.WriteString("## Quick rules\n\n")
	b.WriteString("- Every `zoa run` **must** include `--jira TICKET`.\n")
	b.WriteString("- Parameters map to CLI flags where listed; otherwise `--param name=value`.\n\n")

	list := actions.List()
	if len(list) == 0 {
		b.WriteString("_No actions registered for this target._\n")
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
