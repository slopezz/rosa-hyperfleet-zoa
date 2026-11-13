# ZOA Break-Glass

Emergency scoped `kubectl` / `aws` from the **boundary** only. Same patterns as Trusted Actions and boundary sessions.

| Document | Purpose |
| -------- | ------- |
| [Scope authoring guide](../break-glass-scopes.md) | `pkg/breakglass/` catalog (like [trusted-actions.md](../trusted-actions.md)) |
| [Approval workflow (TA + BG)](../approval-workflow.md) | Approval policy (`none` today; gist/OPA planned) |
| [API reference — break-glass](../api-reference.md#break-glass) | HTTP routes |
| [CLI reference — break-glass](../cli-reference.md#break-glass) | Operator commands |

## Quick flow

1. **Boundary:** `zoa breakglass request --scope kube-read --reason ROSAENG-…` (all v1 scopes use **`approval: none`** → immediate approve via same workflow as future peer approve)
2. **Laptop:** `zoa approve <deployment>/<request-id>` — for future workflows; not required while scopes stay `none` ([approval workflow](../approval-workflow.md))
3. **Boundary:** `zoa breakglass activate <request-id>` then stock `kubectl` / `aws` (at most **one active kube** + **one active aws**; a new activate on the same plane replaces the previous scope in `~/.config/zoa/breakglass/active` and the PS1)
4. **Discovery:** `zoa breakglass describe <scope>` shows composition bullets plus effective **RBAC** (kube) or **IAM** notes (aws) — same data the API returns for that target type
5. **Expiry:** worker reconciler marks expired grants and GCs `zoa-bg-<id>` RBAC

## Jira (ROSAENG-69)

| Story | Repo | Scope |
| ----- | ---- | ----- |
| [ROSAENG-75951](https://redhat.atlassian.net/browse/ROSAENG-75951) | rosa-seaplane-zoa | Core |
| [ROSAENG-75952](https://redhat.atlassian.net/browse/ROSAENG-75952) | rosa-seaplane | Infra |
| [ROSAENG-75953](https://redhat.atlassian.net/browse/ROSAENG-75953) | Both | Docs |
