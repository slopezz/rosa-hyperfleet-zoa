# rosa-hyperfleet-zoa

This file provides guidance to AI coding assistants when working with this repository.

## Project Overview

**rosa-hyperfleet-zoa** is a serverless Zero Operator Access (ZOA) implementation for the ROSA HCP Hyperfleet platform. It contains the API server, Lambda execution engine, CLI, and all Trusted Action implementations. ZOA ensures operators have no persistent, interactive, or unaudited access to customer infrastructure — all operational actions execute through pre-defined, audited Trusted Actions.

**Tech stack**: Go 1.26, Cobra, AWS SDK v2, AWS Lambda, DynamoDB, S3, EventBridge, Kubernetes client-go

## Architecture

ZOA deploys **2 Lambda functions per target VPC** (same binary, `HANDLER_MODE` env var), with failure domain isolation per cluster:

- **API Lambda** — Function URL with native Go streaming, handles CLI requests, sync TA execution
- **Worker Lambda** — EventBridge-triggered, handles reconciler, GC, async TA execution via self-invocation

Per-VPC deployment means a failure in one cluster's ZOA cannot cascade to another. The entire data path (Lambda, DynamoDB, S3, EventBridge, KMS) is serverless with zero persistent compute.

Execution flow: CLI → SigV4-signed HTTPS → API Lambda Function URL → DynamoDB + EKS + S3

## Key Directories

| Path | Purpose |
|------|---------|
| `cmd/zoa/` | CLI binary entry point |
| `cmd/zoa-lambda/` | Lambda function entry point (api + worker modes) |
| `cmd/zoa-runner/` | Async Job runner (K8s Job entrypoint) |
| `internal/cli/` | Cobra commands + APIClient interface for testability |
| `internal/client/` | SigV4-signed HTTP client (talks to Lambda Function URL) |
| `internal/output/` | Table + JSON formatting |
| `internal/eksauth/` | EKS token generation (presigned STS URL) |
| `pkg/actions/` | Go TA framework: interface, registry, implementations, conformance tests |
| `pkg/api/` | HTTP route handlers (dispatch, list, get, audit, version) |
| `pkg/handler/` | Lambda event router (HTTP events, scheduled events, execution events) |
| `pkg/executor/` | K8s SA/RBAC creation, sync execution (impersonation), async Job dispatch |
| `pkg/store/` | DynamoDB persistence (ExecutionStore, AuditStore interfaces) |
| `pkg/scheduler/` | Reconciler and GC (EventBridge-triggered worker tasks) |
| `pkg/config/` | Env-var based configuration with per-mode validation |
| `pkg/metrics/` | CloudWatch EMF metric emission |
| `Containerfile` | Lambda container image (UBI-minimal + zoa-lambda) |
| `Containerfile.runner` | Async runner image (UBI9 + zoa-runner + zoa CLI) |
| `Containerfile.boundary` | ZOA Boundary ECS task image (UBI9 + zoa CLI + SRE tooling) |

## Build & Test Commands

```bash
make all              # verify → test → build
make build            # Build bin/zoa (CLI only)
make test             # Unit tests (go test -race -coverprofile)
make lint             # golangci-lint
make verify           # fmt-check + vet + lint (CI-safe, read-only)
make fmt              # Format code
make image-lambda          # Build zoa-lambda container image
make image-runner          # Build zoa-runner container image
make image-boundary        # Build zoa-boundary container image (Containerfile.boundary)
make image-push-lambda     # Build + push zoa-lambda (latest + git commit tag)
make image-push-runner     # Build + push zoa-runner (latest + git commit tag)
make image-push-boundary   # Build + push zoa-boundary (latest + git commit tag)
make images-push           # Build + push lambda, runner, and boundary (dev workflow)
```

## Important Context

- **Environment**: Set `ZOA_API_URL` to the Lambda Function URL (not API Gateway)
- **Trusted Actions**: Go packages in `pkg/actions/` implementing the `Action` interface, self-registering via `init()`
- **Conformance gate**: `pkg/actions/conformance_test.go` enforces metadata, RBAC, naming, timeout, and test coverage on every PR
- **Two execution modes**: sync (in-Lambda, ephemeral SA/RBAC) and async (K8s Job with STS Secret)
- **Two scopes**: `kube-api` (K8s operations, per-execution SA impersonation) and `aws-api` (AWS operations, STS AssumeRole)
- **Security invariant**: `get_secret` TA rejects access to HCP namespaces (`cluster-*`)
- **Conventional commits**: Use `feat:`, `fix:`, `docs:`, `chore:`, `test:` prefixes
- **Three-layer timeouts**: Lambda ceiling → code deadline (env var) → per-TA timeout (Go code)
- **Architecture docs**: Live in `docs/` (self-contained), not in the hyperfleet repo

## Development Guidelines

### Agent Usage

- **Use the architect agent** for changes to Trusted Action RBAC patterns or security-sensitive code
- **Use the code-reviewer agent** for CLI code quality review
- **Use adversary agent** for security review of changes touching `pkg/executor/`, `pkg/api/`, or `pkg/actions/`

### Testing

- Use `"When ... it should ..."` format for test case names
- Use interface mocks for AWS SDK clients (DynamoDBAPI, S3API in `pkg/store/`, `pkg/executor/`)
- Use `k8s.io/client-go/kubernetes/fake` for K8s unit tests
- Run `make test` before pushing (race detection enabled)
- Conformance tests in `pkg/actions/conformance_test.go` are a PR gate

### Security Guidelines

- **Never** access secrets in `cluster-*` namespaces from TAs
- **Always** declare least-privilege RBAC in TA metadata
- **Never** hardcode credentials — use STS AssumeRole for AWS, SA impersonation for K8s
- **Never** log sensitive data (tokens, credentials, customer content)

### Trusted Action Conventions

- Scope: `kube-api` (requires RBAC declaration) or `aws-api` (must NOT declare RBAC)
- Type: `read` (no cooldown) or `write` (requires cooldown > 0 and DryRunAction)
- Names: snake_case (enforced by conformance test)
- Timeout: must not exceed `EXECUTION_DEADLINE_SECONDS` (default 295s)
- Each TA must have a corresponding test file

## Related Repositories

| Repository | What it contains |
|---|---|
| [rosa-hyperfleet](https://github.com/openshift-online/rosa-hyperfleet) | Terraform infra (Lambda, DynamoDB, S3, KMS, IAM, EventBridge), ArgoCD configs |
