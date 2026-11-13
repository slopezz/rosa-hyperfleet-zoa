// Package scopecatalog (markdown.go) renders the boundary offline break-glass scope catalog.
package scopecatalog

import (
	"fmt"
	"strings"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/breakglass"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/reason"
)

// Markdown renders the boundary / Claude break-glass scope catalog for targetType.
func Markdown(targetType string) (string, error) {
	breakglass.SetTargetType(targetType)
	defer breakglass.SetTargetType("")

	var b strings.Builder
	b.WriteString("# ZOA break-glass scope catalog\n\n")
	fmt.Fprintf(&b, "**Target type:** `%s` (TYPE column in `zoa targets`; offline catalog from this CLI build).\n\n", targetType)
	b.WriteString("Live API: `zoa breakglass scopes` · `zoa breakglass describe <scope>` when connected.\n\n")

	b.WriteString("## Agent rules (mandatory)\n\n")
	b.WriteString("1. **Never suggest break-glass first.** Use **Trusted Actions** from `ZOA_ACTIONS.md` for all work that a TA can do.\n")
	b.WriteString("2. Break-glass is **last resort** — only when TAs are insufficient for the incident and the operator agrees.\n")
	b.WriteString("3. **Escalate minimally:** prefer `kube-read` / `aws-read`, then `*-write`, then `*-elevated` only when required (e.g. secrets or exec).\n")
	b.WriteString("4. Use this catalog (RBAC / IAM composition per scope) to pick the **smallest** scope that can solve the problem.\n")
	b.WriteString("5. **Do not run `zoa breakglass activate` for the operator** unless they explicitly ask. Flow is: request → peer approve (laptop) → operator runs **activate** themselves.\n")
	b.WriteString("6. While break-glass is active, remind the operator to **file a follow-up Trusted Action** so the same ops do not need break-glass again. Suggest a concrete `zoa run …` command or a **new TA name** to implement.\n\n")

	b.WriteString("## Operator workflow (three steps)\n\n")
	b.WriteString("1. `zoa breakglass request --scope <id> --reason TICKET`\n")
	b.WriteString("2. Peer (laptop): `zoa approve <deployment>/<request-id>`\n")
	b.WriteString("3. Operator (boundary): `zoa breakglass activate <request-id>` — explicit; not automatic with `--wait`.\n\n")
	b.WriteString("`request --wait` and `get --wait` only poll until **approved / rejected / expired**, then print next steps.\n\n")

	registered := breakglass.List()
	if len(registered) == 0 {
		b.WriteString("_No scopes registered for this target type._\n")
		return b.String(), nil
	}

	for _, meta := range registered {
		def, ok := breakglass.Get(meta.ID)
		if !ok {
			continue
		}
		desc := breakglass.DescribeScope(def, targetType)
		scope := ClientScopeFromDescribe(desc)

		fmt.Fprintf(&b, "## %s\n\n", scope.ID)
		fmt.Fprintf(&b, "- **Plane:** %s · **Type:** %s\n", scope.Plane, scope.Type)
		fmt.Fprintf(&b, "- **Approval (catalog default):** %v\n", scope.Authorization["approval"])
		fmt.Fprintf(&b, "- %s\n\n", scope.Description)

		composition := scope.Composition
		if len(composition) == 0 {
			composition = scope.Capabilities
		}
		if len(composition) > 0 {
			b.WriteString("**Capabilities / composition:**\n\n")
			for _, line := range composition {
				fmt.Fprintf(&b, "- %s\n", line)
			}
			b.WriteString("\n")
		}
		for _, r := range scope.Restrictions {
			fmt.Fprintf(&b, "- ✗ %s\n", r)
		}
		if len(scope.Restrictions) > 0 {
			b.WriteString("\n")
		}

		if scope.Plane == breakglass.PlaneKube && len(scope.RBAC) > 0 {
			b.WriteString("**Effective RBAC (summary):**\n\n")
			for i, rule := range scope.RBAC {
				fmt.Fprintf(&b, "- rule %d: apiGroups=%v resources=%v verbs=%v\n", i+1, rule.APIGroups, rule.Resources, rule.Verbs)
			}
			b.WriteString("\n")
		}
		if scope.Plane == breakglass.PlaneAWS {
			if scope.AwsRoleKey != "" {
				fmt.Fprintf(&b, "- **AWS role key:** %s\n", scope.AwsRoleKey)
			}
			for _, line := range scope.IAMComposition {
				fmt.Fprintf(&b, "- %s\n", line)
			}
			if scope.AwsRoleKey != "" || len(scope.IAMComposition) > 0 {
				b.WriteString("\n")
			}
		}

		fmt.Fprintf(&b, "**Example request:** `zoa breakglass request --scope %s --reason %s`\n\n", scope.ID, reason.ExampleJiraIssue)
	}

	return b.String(), nil
}
