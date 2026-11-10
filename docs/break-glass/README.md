# Break-glass (not implemented)

Scoped emergency **kubectl** / **AWS** access from the boundary task is **not** available yet. All production work uses **Trusted Actions** from [ZOA Boundary](../boundary/architecture.md).

**Design (draft):** [proposal.md](./proposal.md) — scopes (`kube-read` / `kube-write` / `kube-elevated`, `aws-*`), approval tiers, RBAC/IAM (§5.8), RC/MC differences. English; for review, not shipped product behavior.

Platform context: [ZOA architecture (rosa-seaplane)](https://github.com/openshift-online/rosa-hyperfleet/blob/main/docs/design/zoa-architecture.md).
