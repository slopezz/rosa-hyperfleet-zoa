// Package profilecatalog (markdown.go) renders the boundary offline break-glass profile catalog.
package profilecatalog

import (
	"fmt"
	"strings"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/breakglass"
)

// Markdown renders ZOA_BREAKGLASS_PROFILES.md — same layout conventions as actioncatalog.Markdown (ZOA_ACTIONS.md).
func Markdown(targetType string) (string, error) {
	breakglass.SetTargetType(targetType)
	defer breakglass.SetTargetType("")

	var b strings.Builder
	b.WriteString("# ZOA break-glass profile catalog\n\n")
	fmt.Fprintf(&b, "**Target type:** `%s` (TYPE column in `zoa targets`; offline catalog from this CLI build).\n\n", targetType)
	b.WriteString("Use `zoa breakglass describe <profile>` when connected for full RBAC/IAM, or the sections below to pick a profile.\n\n")

	b.WriteString("## Quick rules\n\n")
	b.WriteString("- Break-glass is **last resort** after **Trusted Actions** (`ZOA_ACTIONS.md`).\n")
	b.WriteString("- Every request **must** include `--reason TICKET`.\n")
	b.WriteString("- Escalate minimally: `kube_read` / `aws_read` → `*-write` → `*-elevated` only when needed.\n")
	b.WriteString("- Operator runs **`zoa breakglass activate`** themselves after approve (never auto with `--wait`).\n")
	b.WriteString("- Peer approve from laptop: `zoa approve <deployment>/<request-id>` when workflow requires it.\n\n")

	registered := breakglass.List()
	if len(registered) == 0 {
		b.WriteString("_No profiles registered for this target type._\n")
		return b.String(), nil
	}

	for _, meta := range registered {
		profile, err := Get(targetType, meta.Name)
		if err != nil {
			continue
		}
		b.WriteString(FormatCatalogMarkdown(profile, meta.TargetTypes))
	}

	return b.String(), nil
}