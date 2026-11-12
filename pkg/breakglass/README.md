# Break-glass scope catalog

Go definitions for **break-glass scopes** — emergency `kubectl` / `aws` privilege bundles compiled into the Lambda, boundary CLI, and worker.

## Taxonomy (ZOA-wide)

| Term | Meaning | Example |
| ---- | ------- | ------- |
| **Scope id** | Catalog entry an operator requests | `kube-read`, `aws-write` |
| **Plane** | API surface (filter dimension; aligns with TA `scope` over time) | `kube`, `aws` (today); may shorten TA `kube-api` → `kube` later) |
| **Type** | Blast-radius class | `read`, `write`, `elevated` |

Trusted Actions use the same **plane + type** dimensions for audit and `zoa runs --scope` / `--type` filters. A TA **action** is a discrete operation (`get_resource`); a break-glass **scope id** is a standing template for raw CLI access.

HTTP handlers live in `pkg/api/breakglass_*`. Boundary shell state (`~/.config/zoa/breakglass/active`) is `pkg/breakglass/active`.

## Layout

```
pkg/breakglass/
  types.go, registry.go, approval.go
  kube_read.go, kube_write.go, kube_elevated.go
  aws_scopes.go
  conformance_test.go
  active/          # local kube/aws stanza (not catalog)
```

One file per scope id (`kube_read.go` → id `kube-read`). Each file calls `register` in `init()`.

## Authoring

See [Break-glass scopes — authoring guide](../../docs/break-glass-scopes.md).
