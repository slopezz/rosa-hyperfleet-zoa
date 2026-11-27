# ZOA Break-Glass

Emergency scoped `kubectl` / `aws` from the **boundary** only. Same patterns as Trusted Actions and boundary sessions.

| Document                                                       | Purpose                                                                                                        |
| -------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- |
| **[Credential architecture](credential-architecture.md)**      | How activate provisions kube/AWS creds (diagrams; like TA [implementation](../architecture/implementation.md)) |
| [Data-plane evidence (by request id)](data-plane-evidence.md)  | CloudTrail + EKS audit queries keyed on `zoa-bg-<request-id>` (interim until a TA wraps this)                  |
| [Profile authoring guide](../break-glass-profiles.md)          | `pkg/breakglass/` catalog — **capabilities/restrictions** + RBAC/IAM (like [trusted-actions.md](../trusted-actions.md)) |
| [Approval workflow (TA + BG)](../approval-workflow.md)         | Approval policy (`none` today; gist/OPA planned)                                                               |
| [API reference — break-glass](../api-reference.md#break-glass) | HTTP routes                                                                                                    |
| [CLI reference — break-glass](../cli-reference.md#break-glass) | Operator commands                                                                                              |
| [HTTP errors (break-glass)](../api-reference.md#break-glass-http-errors) | Consolidated operator error matrix (create, activate, approve)                                       |

## List and target (deployment table)

`zoa breakglass list` and `history` read the **regional DynamoDB table** for the whole deployment. By default you see rows for **every target** (RC and all MCs) unless you pass **`-t` / `--target`**. That is not an IAM leak — activate still runs only on the API Lambda for **your** boundary target (**403 `target_mismatch`** if the grant's `target_cluster` does not match). From an MC session, filter with `-t <mc-target-id>` when you only care about that cluster.

## Quick flow

1. **Boundary:** `zoa breakglass request kube_read --reason ROSAENG-…` (all v1 profiles use **`approval: none`** → immediate approve via same workflow as future peer approve)
2. **Laptop:** `zoa approve <deployment>/<request-id>` — for future workflows; not required while profiles stay `none` ([approval workflow](../approval-workflow.md))
3. **Boundary:** `zoa breakglass activate <request-id>` **in the same target** you used for `request` (RC grant → RC boundary session; MC grant → that MC’s boundary). The API Lambda in each VPC only issues credentials for its own cluster; cross-target activate returns **403 `target_mismatch`**. Then stock `kubectl` / `aws` (see **Two clocks** — re-activate same id ~hourly while grant valid; at most one active kube + one active aws in `~/.config/zoa/breakglass/active`)
4. **Discovery:** `zoa breakglass describe <profile>` shows composition bullets plus effective **RBAC** (kube) or **IAM** notes (aws) — same data the API returns for that target type
5. **Expiry:** worker reconciler marks expired grants and GCs `zoa-bg-<id>` RBAC

## Two clocks (grant vs credentials)

| Clock            | What it means                                                               | What you see                                                                            |
| ---------------- | --------------------------------------------------------------------------- | --------------------------------------------------------------------------------------- |
| **Grant**        | How long lifecycle `status=active` stays valid (per scope, from approve)    | PS1 `grant~3h`; `grant_expires` / `GRANT_EXPIRES` in `get` / `list` / `history`         |
| **Credentials** | Kube **ServiceAccount token** and **STS** session per activate (platform max in `pkg/breakglass/credential.go`) | `kubectl` / `aws` stop when creds expire **even if grant still has hours left**; activate fails with **HTTP 409 `grant_too_short`** if grant remaining is below cloud API minimum (kube 10m, aws 15m) |

**Refresh:** run `zoa breakglass activate <same-request-id>` again before creds expire (while `status=active`, `approval=approved`, and before grant end). You do **not** need a new request. Lifecycle vs decision: [approval-workflow.md](../approval-workflow.md#break-glass-row-status-vs-approval-implemented).

**Shell wiring (boundary):**

| Tool      | Credentials                                                                                                                                                                                                   |
| --------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `kubectl` | `KUBECONFIG` → break-glass kubeconfig                                                                                                                                                                         |
| `aws`     | Break-glass only: with an aws grant, `AWS_SHARED_CREDENTIALS_FILE` → `~/.config/zoa/breakglass/aws-credentials` (`[default]`). No grant → file unset (do not use stock `aws` for task-role work in boundary). |
| `zoa`     | **ECS task role only** (Lambda / TAs); ignores break-glass credential files.                                                                                                                                  |

`PROMPT_COMMAND` sets/unsets env from `active` — the SRE never runs manual `unset`. No `AWS_PROFILE`. `AWS_PAGER` is empty. PS1 uses `|` between kube and aws; cred countdown uses `credentialExpiresAt` from `activate`.

## Jira (ROSAENG-69)

| Story                                                              | Repo              | Scope |
| ------------------------------------------------------------------ | ----------------- | ----- |
| [ROSAENG-75951](https://redhat.atlassian.net/browse/ROSAENG-75951) | rosa-seaplane-zoa | Core  |
| [ROSAENG-75952](https://redhat.atlassian.net/browse/ROSAENG-75952) | rosa-seaplane     | Infra |
| [ROSAENG-75953](https://redhat.atlassian.net/browse/ROSAENG-75953) | Both              | Docs  |
