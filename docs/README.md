# ZOA Documentation

**Zero Operator Access (ZOA)** is the audited operator access model for ROSA Seaplane. No standing cluster admin, no laptop-side `kubectl` — every action flows through an audited, reason-tagged path.

Platform Terraform and accounts: [ZOA architecture (rosa-seaplane)](https://github.com/openshift-online/rosa-seaplane/blob/main/docs/design/zoa-architecture.md). Behavior and CLI/API contracts: **this repo** (this page is the only doc index).

## Start here

| You are…               | Read                                                                                                                                                                           |
| ---------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **Operator** (session) | [Operator workflow](guides/operator-workflow.md) — laptop → TA sequence diagram in [Operator workflow #4](guides/operator-workflow.md#4-start-session-join-exec-and-ownership) |
| **TA author**          | [Trusted Actions](trusted-actions.md) — writing, testing, conformance gates                                                                                                    |
| **Contributor**        | [Development](development.md) · [E2E testing](e2e-testing.md)                                                                                                                  |

## Guides

Operator procedures and laptop-side behavior.

| Document                                                | Description                                                   |
| ------------------------------------------------------- | ------------------------------------------------------------- |
| [Operator workflow](guides/operator-workflow.md)        | Login → deployments → session → `zoa run` → terminate         |
| [Deployment discovery](guides/deployments-discovery.md) | Failover, `--deployments-region`, DR `--access-url` overrides |

## Boundary

The boundary is the shell. Everything else happens inside it.

| Document                                                      | Description                                                |
| ------------------------------------------------------------- | ---------------------------------------------------------- |
| [ZOA Boundary — architecture](boundary/architecture.md)       | Control plane, **SSM topology** (Central vs RC), lifecycle |
| [ZOA Boundary — container image](boundary/container-image.md) | `Containerfile.boundary`, Konflux pipeline                 |
| [ZOA Boundary — identity bridge](boundary/identity-bridge.md) | Task ARN → session → **`operator`** (TA attribution)       |
| [ZOA Boundary — session logging](boundary/session-logging.md) | CloudWatch log groups, Grafana Explorer forensics          |
| [ZOA Boundary — session reaper](boundary/session-reaper.md)   | Deadline, stale `creating`, idle, and operator terminate   |

## Trusted Actions

TAs run from inside the boundary via `zoa run`. They are the steady-state operational model.

| Document                                 | Description                                        |
| ---------------------------------------- | -------------------------------------------------- |
| [TA authoring guide](trusted-actions.md) | Writing a new TA, conformance gates, RBAC patterns |
| [API reference](api-reference.md)        | TA dispatch + Access session APIs                  |
| [CLI reference](cli-reference.md)        | Commands, sessions, discovery, offline catalog     |

## Break-glass

Emergency scoped access from the boundary (see [break-glass/README.md](break-glass/README.md)).

**Policy:** break-glass is a one-time escape hatch. Every break-glass use must produce a follow-up TA that automates the same operation — the TA catalog grows monotonically so the same emergency never requires raw access twice.

## Architecture

Cross-cutting concerns that span all three pillars.

| Document                                         | Description                                            |
| ------------------------------------------------ | ------------------------------------------------------ |
| [Lambda model](architecture/lambda-model.md)     | API, Worker, Access — three modes from one image       |
| [Implementation](architecture/implementation.md) | Package layout, execution flows, env by `HANDLER_MODE` |
| [Storage](architecture/storage.md)               | DynamoDB tables, S3, TTL, GSIs                         |
| [Timeout tuning](architecture/timeout-tuning.md) | TA deadlines; links to Access/boundary session limits  |

## Engineering

| Document                          | Description                                                                          |
| --------------------------------- | ------------------------------------------------------------------------------------ |
| [Development](development.md)     | Build, test, lint, images                                                            |
| [E2E testing](e2e-testing.md)     | Functional + monitoring suites, Makefile targets                                     |
| [Observability](observability.md) | EMF metrics; Lambda logs in Grafana                                                  |
| [Konflux](konflux.md)             | `rosa-seaplane-zoa-lambda`, `rosa-seaplane-zoa-runner`, `rosa-seaplane-zoa-boundary` |

## Doc conventions

- **`docs/README.md`** — single index for all ZOA docs (no per-directory READMEs).
- **`guides/`** — end-to-end operator procedures.
- **`boundary/`** — boundary pillar: architecture, image, identity, logging, reaper.
- **`architecture/`** — runtime and storage shared across API, Worker, and Access.
- **Reference** (`api-reference.md`, `cli-reference.md`) — contracts; keep in sync with code.

Platform ADRs live in **rosa-seaplane**; implementation detail lives here with the code.
