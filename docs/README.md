# ZOA Documentation

## Reference

| Document | Description |
|----------|-------------|
| [API Reference](api-reference.md) | HTTP endpoints, request/response formats, error codes |
| [CLI Reference](cli-reference.md) | Commands, flags, examples |
| [Trusted Actions Guide](trusted-actions.md) | How to author and test new TAs |

## Architecture

| Document | Description |
|----------|-------------|
| [Lambda Model](architecture/lambda-model.md) | Lambda functions, self-invocation, concurrency model |
| [Timeout Tuning](architecture/timeout-tuning.md) | Three-layer timeout architecture, adjustment procedures |
| [Implementation Details](architecture/implementation.md) | Execution flows, package responsibilities, env vars, safety controls |

## ZOA Boundary

| Document | Description |
|----------|-------------|
| [Boundary overview](boundary/README.md) | Index — sessions, container, SRE access |
| [Boundary architecture](boundary/architecture.md) | Access vs API, SSM, RC/MC |
| [SRE access guide](boundary/sre-access-guide.md) | **Identity & SigV4**, sessions, Exec scope, workflows |
| [Container image](boundary/container-image.md) | `Containerfile.boundary`, tools, build, tests |
| [Session logging](design/boundary-session-logging.md) | CloudWatch groups for Exec vs container |
| [Epic plan](design/zoa-boundary-epic-plan.md) | Stories and future work |

## Development

| Document | Description |
|----------|-------------|
| [Development Guide](development.md) | Build, test, lint, CI, releasing |
| [End-to-End Testing](e2e-testing.md) | Deep/smoke tiers, running locally, CI image injection, AWS credentials |

## Documentation Philosophy

Implementation-level architecture docs live **in this repo** (`docs/architecture/`) because they describe decisions specific to ZOA's code and are maintained by the same team that changes the code. This ensures docs stay in sync with the implementation.

Cross-system design proposals (how ZOA fits into the broader platform) live in the [rosa-hyperfleet](https://github.com/openshift-online/rosa-hyperfleet) repo as ADRs. These are referenced below for historical context but are not the source of truth for implementation details.

## External Design Documents

- [ZOA Architecture](https://github.com/openshift-online/rosa-hyperfleet/blob/main/docs/design/zoa-architecture.md) — infrastructure architecture, Terraform context, and platform integration
