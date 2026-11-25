# Break-glass profiles — authoring guide

How to add or change **break-glass profile** definitions in Go (parallel to [Trusted Actions](trusted-actions.md)).

## Vocabulary

| Concept | JSON / UX field | Example |
| ------- | --------------- | ------- |
| Catalog id | `name` | `kube-read` |
| Credential surface | `scope` | `kube`, `aws` |
| Privilege | `type` | `read`, `write`, `elevated` |

CLI: `zoa breakglass profiles`, `zoa breakglass request --name <profile>`. API: `GET /api/v0/breakglass/profiles`, POST body `{ "name", "reason" }`.

## Overview

Each profile is a `Definition` in `pkg/breakglass/`: `ProfileMetadata` (`name`, `scope`, `type`, authorization, **capabilities**, **restrictions**) plus kube **RBAC** rules or an AWS **role key** for Terraform-mapped IAM.

- Self-registers via `init()` into the global registry (like `pkg/actions`)
- Snapshotted onto DynamoDB at request create (in-flight requests do not change when code deploys)
- Validated by `pkg/breakglass/conformance_test.go` on every PR
- **Operator bullets** (`capabilities` / `restrictions` in each `*_read.go`, `*_write.go`, `*_elevated.go`) are what `zoa breakglass describe` shows as **composition** — keep them aligned with the RBAC/IAM builders in the same PR

## What RBAC each scope has (shipped today)

**Start in code:** `kube_rbac.go` (verbs, RC/MC platform catalog, shared helpers), `kube_read.go` / `kube_write.go` / `kube_elevated.go`, `aws_*.go` per scope.  
**Live detail:** `zoa breakglass describe <scope> --target-type rc|mc` (composition + full rule list).

| Scope | Plane | Composition (code) |
| ----- | ----- | ------------------ |
| `kube-read` | kube | `kubeReadRBAC` — get/list/watch on core + admission/RBAC/CRD read + platform `*` (`kube_rbac.go` apiGroups); `pods/log` included |
| `kube-write` | kube | `kubeReadRBAC` + `kubeWriteMutationsRBAC` (apps: mutate without delete; pods/configmaps/jobs: full) |
| `kube-elevated` | kube | `kubeWriteRBAC` + elevated core (`verbsMutateFull` delta) + platform `verbsMutate` on `*` + secrets read + `pods/exec` |
| `aws-read` | aws | `AwsRoleKeyRead` → ReadOnlyAccess |
| `aws-write` | aws | `AwsRoleKeyWrite` → read + RC-only reboot/start/stop (MC ≡ read) |
| `aws-elevated` | aws | `AwsRoleKeyElevated` → PowerUser + ZOA immutability Deny |

Kube tiers compose as **read → write delta → elevated delta** (no extra merge layer in API/CLI).

Verb helpers (`kube_rbac.go`): `verbsRead`, `verbsMutate` (no delete), `verbsMutateFull` (incl. delete). Elevated platform CRs use `verbsMutate` on `resources: *` per apiGroup — not delete on HostedCluster-style objects.

## Shipped capabilities and restrictions (mirror Go metadata)

Update this table when you change `Capabilities` / `Restrictions` in scope `init()` blocks.

| Scope | Capabilities (summary) | Restrictions (summary) |
| ----- | ------------------------ | ---------------------- |
| `kube-read` | Core + storage/scheduling/admission read; platform operator apiGroups read (`*`) per RC/MC | No secrets; no `pods/exec`; serviceaccounts metadata-only |
| `kube-write` | All kube-read; apps controllers create/patch/update; pods/configmaps/jobs full mutate | No secrets/exec; no RBAC bind writes; no platform CR mutate |
| `kube-elevated` | All kube-write; platform create/patch/update on `*`; elevated core full mutate (incl. nodes/PVC/PV/HPA/PDB/volumeattachments/NodeClaims); secrets read + exec | No RBAC bind writes; no webhook/CRD/namespace delete; no secret writes; no delete on platform `resources: *` |
| `aws-read` | ReadOnlyAccess in session account | No write/elevated IAM |
| `aws-write` | Read + RC EC2/RDS/ElastiCache reboot-style mutations | MC same as read; no elevated |
| `aws-elevated` | PowerUser (e.g. EC2 terminate on MC) + immutability Deny on ZOA control plane | IAM/org/secrets sidebar per Terraform |

## Creating a new scope

### 1. Add files

```
pkg/breakglass/my_scope.go
pkg/breakglass/my_scope_test.go   # when logic warrants tests beyond conformance
```

Use underscores in the filename; the scope **id** uses hyphens (`my-custom`).

### 2. Register metadata and RBAC

```go
package breakglass

import (
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"
)

func init() {
	register(&Definition{
		Metadata: ScopeMetadata{
			ID:          "kube-read",
			Plane:       PlaneKube,
			Type:        TypeRead,
			Description: "Read-only kubectl (get/list/watch); no secrets or exec",
			Capabilities: []string{
				"…", // shown in describe composition — keep in sync with RBAC builder
			},
			Restrictions: []string{
				"…",
			},
			Authorization: Authorization{
				Approval:            "none",
				DefaultGrantHours:   4,
				DefaultRequestHours: 24,
			},
			TargetTypes: []string{actions.TargetTypeRC, actions.TargetTypeMC},
		},
		RBAC: kubeReadRBAC,
	})
}

func kubeReadRBAC(targetType string) []actions.RBACRule {
	// targetType is rc or mc when rules differ; v1 often ignores it
	return []actions.RBACRule{ /* ... */ }
}
```

AWS scopes set `AwsRoleKey` instead of `RBAC`; ARNs come from env/Terraform, not hardcoded in Go.

### 3. Approval metadata

Use the same `authorization.approval` field as Trusted Actions (`none` in v1). See [approval-workflow.md](approval-workflow.md).

### 4. Conformance

`go test ./pkg/breakglass/...` is the PR gate (parallel to `pkg/actions/conformance_test.go`):

- Subtest per registered scope id (catalog size is discovered from the registry, not hardcoded)
- Metadata, authorization clocks, kube vs aws shape (`RBAC` vs `aws_role_key`)
- `ValidateScope` / `ValidateAllScopes` — reuses TA forbidden RBAC patterns plus break-glass type rules (secrets read only on **elevated**; read/write without write verbs on read; only elevated `pods/exec`; no RBAC bind mutations; `serviceaccounts` get/list/watch on read+)
- One `*_test.go` per scope id (`kube_read_test.go` → `kube-read`)
- One `.go` implementation file per scope id (`aws_write.go` → `aws-write`)

Extend `shippedApproval` in `conformance_test.go` when the approval workflow epic adds peer approve to the catalog.

## Scope dimensions

| Field | Values | Notes |
| ----- | ------ | ----- |
| `Plane` | `kube`, `aws` | Audit/metrics plane; align with TA scope labels when we drop the `-api` suffix |
| `Type` | `read`, `write`, `elevated` | Same vocabulary as TA `type` |
| `ID` | `kube-read`, … | CLI/API `--scope` and Dynamo `scope` |

## Discovering scopes

From the boundary: `zoa breakglass scopes`, `zoa breakglass describe <id>`. Offline (embedded catalog, filtered by `ZOA_TARGET_TYPE`): `zoa breakglass scopes --offline --target-type rc` — same pattern as `internal/cli/actioncatalog` and `internal/cli/scopecatalog`.

## Related code

| Package | Role |
| ------- | ---- |
| `pkg/breakglass` | Scope catalog (this guide) |
| `pkg/breakglass/active` | Boundary `active` JSON + credential paths |
| `pkg/api/breakglass_*` | HTTP API |
| `pkg/executor/breakglass_*` | RBAC + STS on activate |

**Design archive:** long-form RBAC tables in [break-glass/proposal.md](break-glass/proposal.md) §5.8.3 are not the implementation source of truth — trust `pkg/breakglass/` + this guide.