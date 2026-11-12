# ZOA approval workflow (Trusted Actions + break-glass)

Decisions and behavior for **peer approval** on TAs and break-glass. Target architecture: [ZOA Trusted Actions Authorization (gist)](https://gist.github.com/jmelis/cf4d564ff34909f208557315fd71eed0) (OPA/Rego, approval records, Rover snapshots).

## Decisions (current)

1. **Single vocabulary for “no workflow”:** `authorization.approval: "none"` — same field name in `pkg/actions` (TAs) and `pkg/breakglass` (break-glass scopes).
2. **v1 catalog:** Every shipped TA and every break-glass scope is **`none`**. A **reason** (Jira / PagerDuty) is still required via `pkg/reason` / CLI `ZOA_REASON`.
3. **No deployment-wide approval bypass env var.** Immediate break-glass grant when metadata is `none` is implemented in code (`MaybeAutoApproveBreakglassAfterCreate`), not a Lambda toggle.
4. **Break-glass still creates `pending_approval` first**, then transitions to `approved` in the same request when `none`, using the same `ApproveBreakglassRequest` path as laptop approve (synthetic approver `auto` in audit).
5. **Peer approve/reject routes** (`zoa approve` / `reject`, shared handler for BG + TA IDs) stay in place for the workflow epic; TA execution approve remains not implemented (`501`).
6. **`ZOA_ENVIRONMENT`** (platform tier from Terraform/GitOps) is **deferred** until the workflow epic; operators do not set it.
7. **Future strictness** is expressed as an **approval object** (`min_count`, `approver`, `ttl`, …) per the gist — not as named tiers on metadata.

## Behavior today

### Trusted Actions

- Metadata: `approval: "none"` on all actions.
- Dispatch: does not gate on approval; executions go to `dispatched` after reason + validation.

### Break-glass

- Metadata: `approval: "none"` on all scopes. Shorter `DefaultGrantHours` on elevated scopes is **grant duration**, not approval policy.
- Create: `POST /breakglass/requests` → if `none`, immediate approve → operator can `use` / `connect` without laptop approve.
- Elevated vs read/write affects **RBAC / IAM / TTL**, not approval tier in v1.

### Operators

| Surface | Flow |
| ------- | ---- |
| TA | `zoa run … --reason TICKET` |
| Break-glass | `zoa breakglass request --scope … --reason TICKET` → then `zoa breakglass use <id>` |
| Laptop approve | Required only when a resource’s effective approval is not `none` (not the case in v1). |

## Planned (workflow epic)

### `ZOA_ENVIRONMENT`

API Lambda will receive tier from deploy config (`ephemeral`, `integration`, `stage`, `production`). Used to merge resource defaults with per-environment overrides (tighten in prod; optional relax rules in dev — TBD).

### Effective approval (gist shape)

```yaml
approval: none
# or
approval:
  min_count: 1
  ttl: 4h
  approver:
    not_requester: true
    any_group: ["…"]
```

Shared evaluation for TA and break-glass: Rego + approval records (LDAP snapshot at approve time). BG binds to `requestId` + grant window; TA binds to `executionId` (or pre-dispatch request).

### Open questions

1. Per-environment override vs dedicated test action/scope for approval CI on shared ephemeral.
2. Approvals storage: shared table vs fields on execution/break-glass rows for multi-approver v1.

## Code map

| Location | Role |
| -------- | ---- |
| `pkg/actions` | TA `Authorization.Approval` |
| `pkg/breakglass/approval.go` | `NeedsApproval` (`approval != "none"`) |
| `pkg/api/dispatch.go` | TA create (workflow hook TBD) |
| `pkg/api/breakglass_workflow.go` | Auto-approve after create when `none` |
| `pkg/api/breakglass_service.go` | Peer approve / reject |
| `pkg/api/breakglass_approval.go` | HTTP approve/reject (BG, then TA) |
| `pkg/store/breakglass.go` | BG status + expiry GSIs |
