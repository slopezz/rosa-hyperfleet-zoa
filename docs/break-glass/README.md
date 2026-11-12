# ZOA Break-Glass

Emergency scoped `kubectl` / `aws` from the **boundary** only. Same patterns as Trusted Actions and boundary sessions.

| Document | Purpose |
| -------- | ------- |
| [Feature proposal](./proposal.md) | Design source of truth (scopes, API, security) |
| [API reference — break-glass](../api-reference.md#break-glass) | HTTP routes |
| [CLI reference — break-glass](../cli-reference.md#break-glass) | Operator commands |

## Quick flow

1. **Boundary:** `zoa breakglass request --scope kube-read --reason ROSAENG-…`
2. **Laptop:** `zoa approve <deployment>/<request-id> [--comment …]`
3. **Boundary:** `zoa breakglass use <request-id>` then stock `kubectl` / `aws`
4. **Expiry:** worker reconciler marks expired grants and GCs `zoa-bg-<id>` RBAC

## Jira (ROSAENG-69)

| Story | Repo | Scope |
| ----- | ---- | ----- |
| [ROSAENG-75951](https://redhat.atlassian.net/browse/ROSAENG-75951) | rosa-seaplane-zoa | Core |
| [ROSAENG-75952](https://redhat.atlassian.net/browse/ROSAENG-75952) | rosa-seaplane | Infra |
| [ROSAENG-75953](https://redhat.atlassian.net/browse/ROSAENG-75953) | Both | Docs |
| [ROSAENG-75954](https://redhat.atlassian.net/browse/ROSAENG-75954) | rosa-seaplane-zoa | E2E |
| [ROSAENG-75955](https://redhat.atlassian.net/browse/ROSAENG-75955) | rosa-seaplane | Observability |
