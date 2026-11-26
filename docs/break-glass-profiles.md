# Break-glass profiles — authoring guide

How to add or change **break-glass profile** definitions in Go (parallel to [Trusted Actions](trusted-actions.md)).

## Vocabulary

| Concept | JSON / UX field | Example |
| ------- | --------------- | ------- |
| Catalog id | `name` | `kube_read` |
| Credential surface | `scope` | `kube`, `aws` |
| Privilege | `type` | `read`, `write`, `elevated` |

CLI: `zoa breakglass profiles`, `zoa breakglass request <profile> --reason <ticket>`. API: `GET /api/v0/breakglass/profiles`, POST body `{ "profile", "reason" }`.

## Overview

Each profile is a `Definition` in `pkg/breakglass/`: `ProfileMetadata` (`name`, `scope`, `type`, authorization, **capabilities**, **restrictions**) plus kube **RBAC** rules or an AWS **role key** for Terraform-mapped IAM.

- Self-registers via `init()` into the global registry (like `pkg/actions`)
- Snapshotted onto DynamoDB at request create (in-flight requests do not change when code deploys)
- Validated by `pkg/breakglass/conformance_test.go` on every PR
- **Operator bullets** (`capabilities` / `restrictions` in each `*_read.go`, `*_write.go`, `*_elevated.go`) are what `zoa breakglass describe` lists under **CAPABILITIES** — keep them aligned with the RBAC/IAM builders in the same PR

## What RBAC each profile has (shipped today)

**Start in code:** `kube_rbac.go` (verb vocabulary only), `kube_read.go` / `kube_write.go` / `kube_elevated.go` (each profile’s full RBAC including platform apiGroups), `aws_*.go` per profile.  
**Live detail:** `zoa breakglass describe <profile> --target-type rc|mc` (capabilities + full rule list).

| Profile | Scope | Where defined |
| ----- | ----- | ------------- |
| `kube_read` | kube | `kubeReadRBAC` in `kube_read.go` |
| `kube_write` | kube | `kubeWriteRBAC` in `kube_write.go` |
| `kube_elevated` | kube | `kubeElevatedRBAC` in `kube_elevated.go` |
| `aws_read` | aws | Profile name → `BREAKGLASS_AWS_READ_ROLE_ARN` (ReadOnlyAccess) |
| `aws_write` | aws | Profile name → `BREAKGLASS_AWS_WRITE_ROLE_ARN` (RC reboot/start/stop; MC ≡ read) |
| `aws_elevated` | aws | Profile name → `BREAKGLASS_AWS_ELEVATED_ROLE_ARN` (PowerUser + Deny guards) |

Kube profiles are **self-contained** (no tier append or rule merge). Product copy still describes read ⊂ write ⊂ elevated; change each profile file when permissions change.

Shared vocabulary (`kube_rbac.go` only): `verbsRead`, `verbsMutate`, `verbsMutateFull`, `verbsExecCreate`. Every APIGroup and resource is explicit in the profile file (`kube_*Platform*APIGroups` duplicates the RC/MC platform inventory per profile; keep the three lists aligned).

## Shipped capabilities and restrictions (mirror Go metadata)

Update this table when you change `Capabilities` / `Restrictions` in profile `init()` blocks.

| Profile | Capabilities (summary) | Restrictions (summary) |
| ----- | ------------------------ | ---------------------- |
| `kube_read` | Core + storage/scheduling/admission read; platform operator apiGroups read (`*`) per RC/MC | No secrets; no `pods/exec`; serviceaccounts metadata-only |
| `kube_write` | All kube_read; apps controllers create/patch/update; pods/configmaps/jobs full mutate | No secrets/exec; no RBAC bind writes; no platform CR mutate |
| `kube_elevated` | All kube_write; platform create/patch/update on `*`; elevated core full mutate (incl. nodes/PVC/PV/HPA/PDB/volumeattachments/NodeClaims); secrets read + exec | No RBAC bind writes; no webhook/CRD/namespace delete; no secret writes; no delete on platform `resources: *` |
| `aws_read` | ReadOnlyAccess in session account | No write/elevated IAM |
| `aws_write` | Read + RC EC2/RDS/ElastiCache reboot-style mutations | MC same as read; no elevated |
| `aws_elevated` | PowerUser (e.g. EC2 terminate on MC) + immutability Deny on ZOA control plane | IAM/org/secrets sidebar per Terraform |

## Creating a new profile

### 1. Add files

```
pkg/breakglass/my_scope.go
pkg/breakglass/my_scope_test.go   # when logic warrants tests beyond conformance
```

Use underscores in the filename and in the profile **`name`** (`kube_read`, `aws_write`, …).

### 2. Register metadata and RBAC

```go
package breakglass

import (
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"
)

func init() {
	register(&Definition{
		Metadata: ProfileMetadata{
			Name:        "kube_read",
			Scope:       ScopeKube,
			Type:        TypeRead,
			Description: "Read-only kubectl (get/list/watch); no secrets or exec",
			Capabilities: []string{
				"…", // shown in describe CAPABILITIES — keep in sync with RBAC builder
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

AWS profiles have no `RBAC` func; the executor maps profile **`name`** to `BREAKGLASS_AWS_*_ROLE_ARN` (like TA `scope`+`type` → `AWS_*_ROLE_ARN`). IAM policy text lives in Terraform; **`iam_composition`** in Go is operator-facing only.

### 3. Approval metadata

Use the same `authorization.approval` field as Trusted Actions (`none` in v1). See [approval-workflow.md](approval-workflow.md).

### 4. Conformance

`go test ./pkg/breakglass/...` is the PR gate (parallel to `pkg/actions/conformance_test.go`):

- Subtest per registered profile id (catalog size is discovered from the registry, not hardcoded)
- Metadata, authorization clocks, kube vs aws shape (`RBAC` vs `iam_composition`)
- `ValidateProfile` / `ValidateAllProfiles` — reuses TA forbidden RBAC patterns plus break-glass type rules (secrets read only on **elevated**; read/write without write verbs on read; only elevated `pods/exec`; no RBAC bind mutations; `serviceaccounts` get/list/watch on read+)
- One `*_test.go` per profile id (`kube_read_test.go` → `kube_read`)
- One `.go` implementation file per profile id (`aws_write.go` → `aws_write`)

Extend `shippedApproval` in `conformance_test.go` when the approval workflow epic adds peer approve to the catalog.

## Catalog dimensions

| Field | Values | Notes |
| ----- | ------ | ----- |
| `scope` | `kube`, `aws` | Credential surface; same meaning as TA list filter `--scope` |
| `type` | `read`, `write`, `elevated` | Same vocabulary as TA `type` (TA API rejects `elevated`) |
| `name` | `kube_read`, … | Profile id for CLI/API request body |

## Discovering profiles

From the boundary: `zoa breakglass profiles`, `zoa breakglass describe <profile>`. Offline (embedded catalog, filtered by `ZOA_TARGET_TYPE`): `zoa breakglass profiles --offline --target-type rc` — same pattern as `internal/cli/actioncatalog` and `internal/cli/profilecatalog`.

## Related code

| Package | Role |
| ------- | ---- |
| `pkg/breakglass` | Profile catalog (this guide) |
| `pkg/breakglass/active` | Boundary `active` JSON + credential paths |
| `pkg/api/breakglass_*` | HTTP API |
| `pkg/executor/breakglass_*` | RBAC + STS on activate |

**Design archive:** long-form RBAC tables in [break-glass/proposal.md](break-glass/proposal.md) §5.8.3 are not the implementation source of truth — trust `pkg/breakglass/` + this guide.