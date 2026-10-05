# ZOA Boundary

Audited ECS Fargate containers in each target VPC (RC and MC). SREs connect with **ECS Exec**; operational work runs through **`zoa run`** (Trusted Actions), not standing cluster admin.

**Infrastructure** (Terraform, Bedrock agreements, KMS): [rosa-hyperfleet `terraform/modules/zoa-boundary`](https://github.com/openshift-online/rosa-hyperfleet/tree/main/terraform/modules/zoa-boundary) and [platform ZOA architecture](https://github.com/openshift-online/rosa-hyperfleet/blob/main/docs/design/zoa-architecture.md).

| Document                                                 | Description                                                        |
| -------------------------------------------------------- | ------------------------------------------------------------------ |
| [Architecture](architecture.md)                          | Access vs API Lambda, sessions, SSM discovery, per-VPC placement   |
| [SRE access guide](sre-access-guide.md)                  | Identity (SigV4, Exec scope, identity bridge), sessions, workflows |
| [Container image](container-image.md)                    | `Containerfile.boundary`, tools, completions, build                |
| [Session logging](../design/boundary-session-logging.md) | CloudWatch container vs ECS Exec log groups                        |
| [Session reaper](../design/boundary-session-reaper.md)   | Deadline vs idle enforcement, exec-attached, env vars              |
