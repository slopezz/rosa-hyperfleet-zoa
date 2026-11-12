# rosa-seaplane-zoa

Guidance for AI coding assistants working in this repository.

## What this repo is

**rosa-seaplane-zoa** implements **Zero Operator Access (ZOA)** for ROSA HCP Seaplane: CLI, Lambdas, boundary container, Trusted Actions (TAs), and operator docs. Operators do **not** use `kubectl` or `zoa run` on the laptop — they **`zoa session start`**, work in **ECS Exec** inside an audited **boundary** task, then run TAs from there.

**Tech stack**: Go 1.26, Cobra, AWS SDK v2, AWS Lambda (container images), DynamoDB, S3, EventBridge, ECS Fargate, Kubernetes client-go.

**Platform Terraform** (Lambdas, DynamoDB, ECS, IAM, EventBridge) lives in [rosa-seaplane](https://github.com/openshift-online/rosa-seaplane) (`terraform/modules/zoa`, `zoa-lambda`, `zoa-access`). **Behavior and API/CLI contracts** live here.

## Three pillars (read `README.md` for detail)

| Pillar              | Status  | Role                                                                          |
| ------------------- | ------- | ----------------------------------------------------------------------------- |
| **Boundary**        | Shipped | Ephemeral ECS Fargate shell per target VPC; entry point for all operator work |
| **Trusted Actions** | Shipped | Audited `zoa run` from inside the boundary; ephemeral per-execution RBAC      |
| **Break-glass**     | Planned | Emergency scoped access from boundary — see `docs/break-glass/README.md`      |

## Architecture (short)

**One Lambda image, three modes** (`HANDLER_MODE` on `zoa-lambda`):

| Mode       | Where                         | Caller                             | Purpose                                                |
| ---------- | ----------------------------- | ---------------------------------- | ------------------------------------------------------ |
| **access** | RC account, **no VPC**        | SRE laptop (SigV4)                 | Sessions, targets, `ecs:RunTask` for boundary          |
| **api**    | Per target VPC (RC + each MC) | Boundary task role (`ZOA_API_URL`) | TA dispatch, runs, audit, streaming downloads          |
| **worker** | Per target VPC                | EventBridge + self-invoke          | Reconciler, GC, **session reaper**, async TA execution |

**Three container images** from this repo: `zoa-lambda`, `zoa-runner` (async Jobs on EKS), `zoa-boundary` (operator shell).

**Typical flows**

- **Session**: Laptop → Access Function URL → DynamoDB + ECS boundary → ECS Exec transcript (CloudWatch).
- **TA (sync)**: Boundary → API Function URL → identity bridge (task ARN → session → operator) → EKS impersonation → inline response.
- **TA (async)**: API dispatches Job → `zoa-runner` uploads to S3 → Worker reconciles.

Failure domains are **per target VPC** for API/Worker/boundary; **central Access** for laptop session APIs. See `README.md` (Failure Domains).

## Documentation map

Start at **`docs/README.md`**. Do not duplicate long architecture in agent prompts — link instead.

| Area                        | Path                                                                                                                                               |
| --------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- |
| Operator workflow           | `docs/guides/operator-workflow.md`                                                                                                                 |
| Boundary                    | `docs/boundary/` — architecture, container image, identity bridge, session logging, session reaper                                                 |
| Identity bridge             | `docs/boundary/identity-bridge.md`                                                                                                          |
| Session reaper / logging    | `docs/boundary/session-reaper.md`, `docs/boundary/session-logging.md`                                                                |
| Laptop deployment discovery | `docs/guides/deployments-discovery.md`                                                                                                             |
| Lambda model                | `docs/architecture/lambda-model.md`, `docs/architecture/implementation.md`                                                                         |
| API / CLI                   | `docs/api-reference.md`, `docs/cli-reference.md`                                                                                                   |
| TAs                         | `docs/trusted-actions.md`                                                                                                                          |
| Break-glass scopes          | `docs/break-glass-scopes.md`                                                                                                                       |
| E2E / dev                   | `docs/e2e-testing.md`, `docs/development.md`                                                                                                       |
| Platform ADR                | [rosa-seaplane `docs/design/zoa-architecture.md`](https://github.com/openshift-online/rosa-seaplane/blob/main/docs/design/zoa-architecture.md) |

## Key directories

| Path                                                                     | Purpose                                                       |
| ------------------------------------------------------------------------ | ------------------------------------------------------------- |
| `cmd/zoa/`                                                               | CLI (`session`, `run`, `audit`, …)                            |
| `cmd/zoa-lambda/`                                                        | Lambda entry (`HANDLER_MODE=api\|worker\|access`)             |
| `cmd/zoa-runner/`                                                        | Async Job entrypoint in target EKS                            |
| `internal/cli/`                                                          | Cobra + `APIClient` / Access client for tests                 |
| `internal/client/`                                                       | SigV4 HTTP to Function URLs                                   |
| `pkg/api/`                                                               | HTTP routes: TA plane (`api`) and session plane (`access.go`) |
| `pkg/handler/`                                                           | Lambda event router (HTTP, schedules, execute events)         |
| `pkg/executor/`                                                          | SA/RBAC, sync impersonation, async Job dispatch               |
| `pkg/store/`                                                             | DynamoDB: executions, audit, **sessions**, targets            |
| `pkg/scheduler/`                                                         | Reconciler, GC, **reaper** (`reaper.go`)                      |
| `pkg/actions/`                                                           | TA framework + implementations; `conformance_test.go` gate    |
| `pkg/breakglass/`                                                        | Break-glass scope catalog; `conformance_test.go` gate — [docs/break-glass-scopes.md](docs/break-glass-scopes.md) |
| `pkg/breakglass/active/`                                                 | Boundary `~/.config/zoa/breakglass/active` kube/aws stanzas   |
| `pkg/boundaryexec/`, `pkg/accesscross/`, `pkg/awsecs/`, `pkg/execcreds/` | Boundary / ECS Exec / cross-account session helpers           |
| `pkg/config/`                                                            | Env config; validates mode-specific required vars             |
| `Containerfile`                                                          | `zoa-lambda` image                                            |
| `Containerfile.runner`                                                   | `zoa-runner` image                                            |
| `Containerfile.boundary`                                                 | `zoa-boundary` image                                          |

## Build & test

```bash
make all              # verify → test → build
make build            # bin/zoa only
make test             # go test -race
make verify           # fmt-check + vet + lint
make image-lambda     # make image-runner, image-boundary
make images-push      # dev: push lambda + runner + boundary to Quay (see Makefile)
```

## Important invariants

- **Laptop**: Access Lambda URL (SSM autodiscovery / `zoa` config) — not `ZOA_API_URL`.
- **Boundary**: `ZOA_API_URL` points at **API** Lambda in that VPC; optional `ZOA_REASON` default for `zoa run`.
- **TAs**: `pkg/actions/` register via `init()`; conformance enforces naming, RBAC, timeouts, tests.
- **Scopes**: `kube-api` (RBAC required) vs `aws-api` (no RBAC in metadata).
- **Modes**: sync (in-Lambda) vs async (K8s Job + runner).
- **Security**: no secrets in `cluster-*` namespaces (`get_secret`); no credential logging; STS + SA impersonation only.
- **Timeouts**: Lambda ceiling → `EXECUTION_DEADLINE_SECONDS` → per-TA timeout in code.
- **Commits**: conventional `feat:`, `fix:`, `docs:`, `chore:`, `test:` (Jira prefix when applicable).

## Agent usage

- **Architect**: TA RBAC patterns, boundary/session security, cross-account access design.
- **Code-reviewer**: CLI and API handler changes.
- **Adversary**: `pkg/executor/`, `pkg/api/`, `pkg/actions/`, `pkg/accesscross/`, session/boundary paths.

## Tests

- Test names: `"When ... it should ..."`.
- Mocks for AWS interfaces in `pkg/store/`, `pkg/executor/`; `k8s.io/client-go/kubernetes/fake` for K8s.
- Run `make test` before push; conformance tests are a PR gate.

## Related repositories

| Repository                                                                     | Contents                                                                         |
| ------------------------------------------------------------------------------ | -------------------------------------------------------------------------------- |
| [rosa-seaplane](https://github.com/openshift-online/rosa-seaplane)             | ZOA/boundary Terraform, image tags in `config/defaults.yaml`, rendered `deploy/` |
| [rosa-seaplane-api](https://github.com/openshift-online/rosa-seaplane-api) | Platform API / operator (separate from ZOA)                                      |

When bumping images for environments: commit in **this** repo, then set `zoa_lambda_image_tag`, `zoa_runner_image_tag`, and `zoa_boundary_image_tag` in rosa-seaplane `config/` and run `uv run scripts/render.py`.
`.
