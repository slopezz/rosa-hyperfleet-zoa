# ZOA Boundary

Audited ECS Fargate containers in each target VPC (RC and MC). SREs connect with **ECS Exec**; operational work runs through **`zoa run`** (Trusted Actions), not standing cluster admin.

**Operators:** [Operator workflow](../guides/operator-workflow.md). **Terraform:** boundary ECS and per-VPC Lambdas in [rosa-hyperfleet `zoa-lambda`](https://github.com/openshift-online/rosa-hyperfleet/tree/main/terraform/modules/zoa-lambda); Access Lambda in [`zoa-access`](https://github.com/openshift-online/rosa-hyperfleet/tree/main/terraform/modules/zoa-access). Platform: [ZOA architecture ADR](https://github.com/openshift-online/rosa-hyperfleet/blob/main/docs/design/zoa-architecture.md).

| Document                                                 | Description                                                        |
| -------------------------------------------------------- | ------------------------------------------------------------------ |
| [Operator workflow](../guides/operator-workflow.md)        | **Start here** — laptop → session → `zoa run` → terminate          |
| [Architecture](architecture.md)                          | Access vs API Lambda, sessions, SSM discovery, per-VPC placement   |
| [Container image](container-image.md)                    | `Containerfile.boundary`, build, Konflux                           |
| [Session logging](../design/boundary-session-logging.md) | CloudWatch container vs ECS Exec log groups                        |
| [Session reaper](../design/boundary-session-reaper.md)   | Deadline vs idle enforcement, exec-attached, env vars              |
