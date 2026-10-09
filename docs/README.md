# ZOA documentation

**Zero Operator Access (ZOA)** is audited operator work on ROSA HyperFleet: no standing cluster admin; **boundary sessions** and **Trusted Actions** record who, what, and **reason**.

| Piece | Role |
| ----- | ---- |
| **ZOA Boundary** | Ephemeral **ECS Fargate** in the target VPC — **only supported place to run TAs** |
| **Trusted Actions** | `zoa run` via API/Worker Lambdas (sync or async) |
| **Access Lambda** | Laptop: sessions, targets, `RunTask` for boundary (RC account) |
| **Break-glass** | Not implemented — [break-glass/README.md](break-glass/README.md) (planned stub) |

Platform Terraform and accounts: [ZOA architecture (rosa-hyperfleet)](https://github.com/openshift-online/rosa-hyperfleet/blob/main/docs/design/zoa-architecture.md). Behavior and CLI/API contracts: **this repo**.

## Start here

| You are… | Read |
| -------- | ---- |
| **Operator (investigation)** | [Operator workflow](guides/operator-workflow.md) |
| **TA author** | [Trusted Actions](trusted-actions.md) |
| **Contributor** | [Development](development.md) · [E2E testing](e2e-testing.md) |

## Reference

| Document | Description |
| -------- | ----------- |
| [CLI reference](cli-reference.md) | Commands, sessions, discovery, offline catalog |
| [API reference](api-reference.md) | TA API + Access API |

## Architecture

| Document | Description |
| -------- | ----------- |
| [Lambda model](architecture/lambda-model.md) | API, Worker, Access — modes and routing |
| [Implementation](architecture/implementation.md) | Packages, flows, env by `HANDLER_MODE` |
| [Storage](architecture/storage.md) | DynamoDB, S3, TTL, GSIs |
| [Timeout tuning](architecture/timeout-tuning.md) | Lambda, code, and TA timeouts |

## Boundary

| Document | Description |
| -------- | ----------- |
| [Boundary overview](boundary/README.md) | Index |
| [Boundary architecture](boundary/architecture.md) | Access vs API, SSM, lifecycle |
| [Container image (build)](boundary/container-image.md) | `Containerfile.boundary`, Konflux |
| [Session logging](design/boundary-session-logging.md) | CloudWatch: container vs Exec |
| [Session reaper](design/boundary-session-reaper.md) | Hard vs inactivity termination |
| [Identity and storage](design/boundary-identity-and-storage.md) | Identity bridge, session schema |

Legacy path: [boundary/sre-access-guide.md](boundary/sre-access-guide.md) → use **operator workflow** above.

## Engineering

| Document | Description |
| -------- | ----------- |
| [Development](development.md) | Build, test, lint, images |
| [E2E testing](e2e-testing.md) | Functional + monitoring suites, Makefile targets |
| [Observability](observability.md) | TA metrics; session metrics TBD |
| [Konflux](konflux.md) | `zoa-lambda`, `zoa-runner`, `zoa-boundary` images |

## Philosophy

- **Guides** — end-to-end tasks.
- **Reference** — CLI/HTTP contracts (keep in sync with code).
- **Architecture / design** — how it works and decisions that survive refactors.

Platform ADRs live in **rosa-hyperfleet**; implementation detail lives here with the code.
