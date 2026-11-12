# Break-glass scopes — authoring guide

How to add or change **break-glass scope** definitions in Go (parallel to [Trusted Actions](trusted-actions.md)).

## Overview

Each scope is a `Definition` in `pkg/breakglass/`: metadata (id, plane, type, authorization, capabilities) plus kube **RBAC** rules or an AWS **role key** for Terraform-mapped IAM.

- Self-registers via `init()` into the global registry (like `pkg/actions`)
- Snapshotted onto DynamoDB at request create (in-flight requests do not change when code deploys)
- Validated by `pkg/breakglass/conformance_test.go` on every PR

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
			Authorization: Authorization{
				Approval:            "none",
				DefaultGrantHours:   4,
				DefaultPendingHours: 24,
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

`go test ./pkg/breakglass/...` runs conformance. Today it requires:

- Every shipped scope uses `approval: "none"`
- `DefaultGrantHours > 0`; elevated scopes `<= 2` hours

Extend conformance when the approval epic adds stricter rules.

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
| `pkg/executor/breakglass_*` | RBAC + STS on `use` |
