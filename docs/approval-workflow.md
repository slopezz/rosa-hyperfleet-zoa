# ZOA approval workflow (Trusted Actions + break-glass)

Decisions and behavior for **peer approval** on TAs and break-glass. Target architecture: [ZOA Trusted Actions Authorization (gist)](https://gist.github.com/jmelis/cf4d564ff34909f208557315fd71eed0) (OPA/Rego, approval records, Rover snapshots).

## Decisions (current)

1. **Single vocabulary for “no workflow”:** `authorization.approval: "none"` — same field name in `pkg/actions` (TAs) and `pkg/breakglass` (break-glass scopes).
2. **v1 catalog:** Every shipped TA and every break-glass scope is **`none`**. A **reason** (Jira / PagerDuty) is still required via `pkg/reason` / CLI `ZOA_REASON`.
3. **No deployment-wide approval bypass env var.** When scope metadata is `none`, create handler calls `ApproveBreakglassOnCreateWhenNoPeerWorkflow` (same `ApproveBreakglassRequest` as laptop approve), not a Lambda toggle.
4. **Break-glass still creates `pending_approval` first**, then transitions to `approved` in the same request when `none`, using the same `ApproveBreakglassRequest` path as laptop approve (synthetic approver `auto` in audit).
5. **Peer approve/reject (v1):** one opaque UUID on `POST /api/v0/approve/{id}` and `POST /api/v0/reject/{id}` (API + Access). Handler **looks up break-glass first**, then executions (TA). TA execution approve returns **501**. Split per-pillar routes are planned (below).
6. **`ZOA_ENVIRONMENT`** (platform tier from Terraform/GitOps) is **deferred** until the workflow epic; operators do not set it.
7. **Future strictness** is expressed as an **approval object** (`min_count`, `approver`, `ttl`, …) per the gist — not as named tiers on metadata.

## Approve / reject: same UUID, how does the system know?

Break-glass **request IDs** and TA **execution IDs** are both random UUIDs. They are not required to be unique across tables; in practice a collision is negligible.

**Today:** there is no `kind` query param or path prefix. On approve/reject the handler resolves `{id}` (or Access compound `{deployment}/{id}`) and:

1. If `BREAKGLASS_TABLE` is configured and `Get(id)` returns a row → **break-glass** approve/reject.
2. Else if `EXECUTIONS_TABLE` has a row → **TA** path (501 for approve/reject until the workflow epic).
3. Else **404**.

Audit, metrics, and DynamoDB keys remain pillar-specific (`requestId` vs `executionId`); only the **shared laptop route** is polymorphic.

## Approve / reject: API surfaces and CLI (current + planned)

### HTTP (planned split — workflow epic)

Explicit pillars instead of table probing:

| Pillar | Planned approve | Planned reject | Lambda |
| ------ | ---------------- | -------------- | ------ |
| Break-glass | `POST /api/v0/breakglass/requests/{id}/approve` | `POST …/reject` | Access (laptop) and/or API |
| Trusted Action | `POST /api/v0/executions/{id}/approve` | `POST …/reject` | TBD (likely Access + policy) |

v1 **legacy** `/approve/{id}` on Access may stay as a thin wrapper (deployment in path) or deprecate after migration.

### CLI

| Where | Talks to | ID / routing |
| ----- | -------- | ------------- |
| **Laptop** | **Access** Lambda (SigV4, config / SSM deployment) | `zoa approve <deployment>/<request-id>` — **deployment** picks which regional Access URL to call; required because one laptop sees many deployments. |
| **Boundary** | **API** Lambda (`ZOA_API_URL`; deployment implicit) | Break-glass: `zoa breakglass request|get|activate` use **request UUID only** — no `deployment/` prefix. Peer approve from inside the boundary is rare; prefer **subcommands** `zoa breakglass approve|reject <request-id>` (planned) over top-level `zoa approve` with a redundant deployment. |
| **TA (future)** | API for dispatch; Access or API for approve | `zoa run … --wait` vs `zoa executions approve <id>` (or similar) — pillar-specific, not shared opaque `zoa approve` in the boundary. |

**Rationale:** `deployment/` is an **Access routing key**, not part of the grant identity. Inside ECS boundary, `ZOA_API_URL` already scopes the target VPC; forcing `deployment/request-id` on approve would duplicate session metadata and invite mistakes.

## Behavior today

### Trusted Actions

- Metadata: `approval: "none"` on all actions.
- Dispatch: does not gate on approval; executions go to `dispatched` after reason + validation.

### Break-glass

- Metadata: **`approval: "none"` on all scopes in v1** (workflow epic will require peer approval). Shorter `DefaultGrantHours` on elevated scopes is **grant duration**, not approval policy.
- Create: `POST /breakglass/requests` → row starts `pending_approval`; if catalog `approval: none`, **`ApproveBreakglassOnCreateWhenNoPeerWorkflow`** transitions to `approved` in the same request (synthetic approver `auto` in audit). When scopes require peer workflow, that call is a no-op and status stays pending.
- Logic lives in `pkg/api/breakglass_workflow.go` — driven only by **scope catalog metadata**, like TA dispatch ignoring approval when `none`.
- **Three-step UX (always):** request (boundary) → approve (peer / auto when `none`) → **`zoa breakglass activate <id>`** (operator, boundary). CLI **`--wait`** on `request` / `get` polls with a **stderr progress line** (like `zoa run --wait`) until **`approved`**, **`rejected`**, or **`expired`**, then prints **`✓`/`✗` + elapsed** on stderr and **metadata on stdout** (or JSON envelope with `next_step`); it **never** calls activate.
- **Task binding:** only the **same boundary ECS task** that created the request may call `POST …/requests/{id}/activate`. Stored `creatorTaskId` is the full **32-character ECS task id** from the boundary task role session name.

### Operators

| Surface | Flow |
| ------- | ---- |
| TA | `zoa run … --reason TICKET` |
| Break-glass | `zoa breakglass request --scope … --reason TICKET` → peer `zoa approve …` when needed → **`zoa breakglass activate <id>`** (explicit) |
| Track approval | `zoa breakglass get <id> --wait` (decision only; then activate manually) |
| Laptop approve | `zoa approve <deployment>/<request-id>` — required when effective approval is not `none` (v1: auto-approve when `none`). |

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

Shared evaluation for TA and break-glass: Rego + approval records (LDAP snapshot at approve time). BG binds to `requestId` + grant window + creator task; TA binds to `executionId` (or pre-dispatch request).

### Open questions

1. Per-environment override vs dedicated test action/scope for approval CI on shared ephemeral.
2. Approvals storage: shared table vs fields on execution/break-glass rows for multi-approver v1.

## Code map

| Location | Role |
| -------- | ---- |
| `pkg/actions` | TA `Authorization.Approval` |
| `pkg/breakglass/approval.go` | `NeedsApproval` (`approval != "none"`) |
| `pkg/api/dispatch.go` | TA create (workflow hook TBD) |
| `pkg/api/breakglass_workflow.go` | Approve on create when catalog skips peer workflow |
| `pkg/api/breakglass_service.go` | Peer approve / reject |
| `pkg/api/breakglass_approval.go` | HTTP approve/reject (BG, then TA) |
| `pkg/store/breakglass.go` | BG status + expiry GSIs |
