package profilecatalog

import (
	"fmt"
	"io"
	"strings"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/cli/parambind"
	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/breakglass"
)

// FormatDescribeHuman prints the same fields as API/CLI describe (stdout).
func FormatDescribeHuman(w io.Writer, p *client.ProfileMetadata) {
	if p == nil {
		return
	}
	fmt.Fprintf(w, "NAME:        %s\n", p.Name)
	fmt.Fprintf(w, "SCOPE:       %s\n", p.Scope)
	fmt.Fprintf(w, "TYPE:        %s\n", p.Type)
	if v := authString(p, "approval"); v != "" {
		fmt.Fprintf(w, "APPROVAL:    %s\n", v)
	}
	if ttl := requestTTL(p); ttl != "" {
		fmt.Fprintf(w, "Request TTL: %s (decision must complete before request expires)\n", ttl)
	}
	if ttl := grantTTL(p); ttl != "" {
		fmt.Fprintf(w, "Grant TTL:   %s after approve\n", ttl)
	}
	if p.Scope != "" {
		fmt.Fprintf(w, "Credential TTL: up to %s per activate (re-activate while grant valid)\n", breakglass.CredentialMaxTTLDisplay(p.Scope))
	}
	fmt.Fprintf(w, "DESCRIPTION: %s\n", p.Description)

	fmt.Fprintf(w, "\nREQUEST (global flags on every zoa breakglass request):\n")
	for _, m := range parambind.RequestModifiers() {
		req := ""
		if m.Required {
			req = " (required)"
		}
		fmt.Fprintf(w, "  %-28s %s%s\n", m.Flags, m.Description, req)
	}

	composition := p.Composition
	if len(composition) == 0 {
		composition = p.Capabilities
	}
	if len(composition) > 0 {
		fmt.Fprintf(w, "\nCAPABILITIES:\n")
		for _, c := range composition {
			fmt.Fprintf(w, "  • %s\n", c)
		}
	}
	if len(p.Restrictions) > 0 {
		fmt.Fprintf(w, "\nRESTRICTIONS:\n")
		for _, r := range p.Restrictions {
			fmt.Fprintf(w, "  ✗ %s\n", r)
		}
	}
	if p.Scope == breakglass.ScopeKube {
		target := p.TargetType
		if target == "" {
			target = actions.TargetTypeRC
		}
		def, ok := breakglass.Get(p.Name)
		if ok && def.RBAC != nil {
			rules := breakglass.EffectiveRBAC(def, target)
			fmt.Fprintf(w, "\nRBAC (target %s, %d rules):\n", target, len(rules))
			for i, rule := range rules {
				fmt.Fprintf(w, "  [%d] apiGroups=%v resources=%v verbs=%v\n", i+1, rule.APIGroups, rule.Resources, rule.Verbs)
			}
		}
	}
	if p.Scope == breakglass.ScopeAWS && len(p.IAMComposition) > 0 {
		fmt.Fprintf(w, "\nIAM:\n")
		for _, line := range p.IAMComposition {
			fmt.Fprintf(w, "  • %s\n", line)
		}
	}
	if len(p.Examples) > 0 {
		fmt.Fprintf(w, "\nEXAMPLES:\n")
		for _, ex := range p.Examples {
			fmt.Fprintf(w, "  %s\n", ex)
		}
	}
}

// FormatCatalogMarkdown is one ## section in ZOA_BREAKGLASS_PROFILES.md (mirrors actioncatalog per-action blocks).
func FormatCatalogMarkdown(p *client.ProfileMetadata, targetTypes []string) string {
	if p == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## %s\n\n", p.Name)
	approval := authString(p, "approval")
	if approval == "" {
		approval = "—"
	}
	fmt.Fprintf(&b, "- **Scope:** %s · **Type:** %s · **Approval:** %s\n", p.Scope, p.Type, approval)
	if len(targetTypes) > 0 {
		fmt.Fprintf(&b, "- **Also on:** %s\n", strings.Join(targetTypes, ", "))
	}
	fmt.Fprintf(&b, "- %s\n\n", p.Description)

	composition := p.Composition
	if len(composition) == 0 {
		composition = p.Capabilities
	}
	for _, c := range composition {
		fmt.Fprintf(&b, "- %s\n", c)
	}
	for _, r := range p.Restrictions {
		fmt.Fprintf(&b, "- ✗ %s\n", r)
	}
	if len(composition) > 0 || len(p.Restrictions) > 0 {
		b.WriteString("\n")
	}

	if p.Scope == breakglass.ScopeKube {
		fmt.Fprintf(&b, "- **RBAC detail:** `zoa breakglass describe %s` (full rules for this profile).\n\n", p.Name)
	}
	if len(p.Examples) > 0 {
		b.WriteString("**Examples:**\n\n")
		for _, ex := range p.Examples {
			fmt.Fprintf(&b, "```bash\n%s\n```\n\n", ex)
		}
	}
	return b.String()
}

func authString(p *client.ProfileMetadata, key string) string {
	if p.Authorization == nil {
		return ""
	}
	raw, ok := p.Authorization[key]
	if !ok || raw == nil {
		return ""
	}
	if v, ok := raw.(string); ok {
		return v
	}
	return fmt.Sprint(raw)
}

func requestTTL(p *client.ProfileMetadata) string {
	h := authInt(p, "default_request_hours")
	if h <= 0 {
		return ""
	}
	return fmt.Sprintf("%dh", h)
}

func grantTTL(p *client.ProfileMetadata) string {
	h := authInt(p, "default_grant_hours")
	if h <= 0 {
		return ""
	}
	return fmt.Sprintf("%dh", h)
}

func authInt(p *client.ProfileMetadata, key string) int {
	if p.Authorization == nil {
		return 0
	}
	raw, ok := p.Authorization[key]
	if !ok || raw == nil {
		return 0
	}
	switch v := raw.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}