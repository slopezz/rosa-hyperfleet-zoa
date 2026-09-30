# Development Guide

## Prerequisites

- **Go 1.26+** — install from [go.dev/dl](https://go.dev/dl/) or via GVM
- **golangci-lint v2.12+** — `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2`
- **Make**
- **AWS CLI v2** — with a profile configured for your environment

## Quick Start

```bash
git clone https://github.com/openshift-online/rosa-hyperfleet-zoa.git
cd rosa-hyperfleet-zoa
make all                    # fmt → vet → lint → test → build
```

## Build Targets

| Target | What it does |
|--------|-------------|
| `make all` | fmt → vet → lint → test → build (run before pushing) |
| `make build` | Build `./bin/zoa` |
| `make dist` | Cross-compile CLI for GitHub Releases (`./dist`) |
| `make install` | Install to `$GOBIN` |
| `make fmt` | Format code (`gofmt -w -s`) |
| `make vet` | Static analysis (`go vet`) |
| `make lint` | Lint (`golangci-lint`) |
| `make test` | Unit tests with coverage and race detection |
| `make verify` | Read-only checks (`fmt-check + vet + lint`) |
| `make tidy` | Clean up `go.mod` / `go.sum` |
| `make test-e2e` | E2E deep suite (requires `ZOA_RC_API_URL` / `ZOA_MC_API_URL`) |
| `make test-e2e-smoke` | E2E smoke subset only (~2min) |
| `make image-lambda` | Build Lambda container image |
| `make image-runner` | Build runner container image |
| `make image-boundary` | Build ZOA Boundary ECS task image (`Containerfile.boundary`) |
| `make image-push-lambda` | Build + push Lambda image (`:latest` + `:commit`) |
| `make image-push-runner` | Build + push runner image (`:latest` + `:commit`) |
| `make image-push-boundary` | Build + push boundary image (`:latest` + `:commit`) |
| `make images-push` | Build + push lambda, runner, and boundary (single command for dev workflow) |

## Binaries

This repository produces three binaries:

| Binary | Entry Point | Purpose |
|--------|-------------|---------|
| `zoa` | `cmd/zoa/main.go` | CLI for operators |
| `zoa-lambda` | `cmd/zoa-lambda/main.go` | Lambda function (API + Worker modes, deployed per VPC) |
| `zoa-runner` | `cmd/zoa-runner/main.go` | Async Job runner (executes inside K8s Jobs for async TAs) |

## Testing

```bash
make test                    # All tests with race detection
go test ./pkg/actions/...    # Action tests only
go test ./pkg/handler/...    # Handler tests only
go test ./pkg/executor/...   # Executor tests only
```

### Test Patterns

- Use `k8s.io/client-go/kubernetes/fake` for Kubernetes unit tests
- Use interface mocking for DynamoDB/S3 operations
- Test names follow `"When ... it should ..."` format
- All tests run with `-race` flag

### End-to-End Tests

`test/e2e/` is a Ginkgo suite (build-tagged `e2e`, excluded from `make test`) that drives the
built `zoa` CLI against a real, already-provisioned RC and/or MC ZOA Lambda API and exercises
every registered Trusted Action.

```bash
# Set URLs (from `make ephemeral-list ID=...` in rosa-hyperfleet)
export ZOA_RC_API_URL="https://<id>.lambda-url.<region>.on.aws/"
export ZOA_MC_API_URL="https://<id>.lambda-url.<region>.on.aws/"

# Override AWS profiles — your host uses rrp-regional-dev / rrp-management-dev,
# but the suite defaults to rrp-rc / rrp-mc (used inside CI containers).
export ZOA_RC_AWS_PROFILE="rrp-regional-dev"
export ZOA_MC_AWS_PROFILE="rrp-management-dev"

make test-e2e                          # full deep suite
make test-e2e-smoke                    # Label("smoke") subset (~2min)
GINKGO_FLAGS=-ginkgo.v make test-e2e   # verbose output
```

RC and MC run in parallel automatically when both URLs are set. If only one URL is set, only
that target runs.

**If you changed API/TA code** (not just tests), you must build and deploy your images to the
ephemeral before running e2e — see the
[Developer Workflow](e2e-testing.md#developer-workflow-testing-code-changes) section in
`e2e-testing.md` for the full step-by-step (`make images-push` → configure tag → resync → test).

See **[`docs/e2e-testing.md`](e2e-testing.md)** for the full guide: running against a dev-account
ephemeral environment (from this repo or from `rosa-hyperfleet`), CI image injection, AWS
credentials.

## CI

CI runs via [OpenShift CI (Prow)](https://prow.ci.openshift.org/).
Config: [`openshift/release` — `ci-operator/config/openshift-online/rosa-hyperfleet-zoa/`](https://github.com/openshift/release/blob/master/ci-operator/config/openshift-online/rosa-hyperfleet-zoa/openshift-online-rosa-hyperfleet-zoa-main.yaml).

| Job | What it checks |
|-----|----------------|
| `lint` | `make fmt-check` + `make lint` |
| `test` | `make test` + coverage artifacts |
| `verify` | `make verify` |
| `on-demand-e2e` | Full e2e against an ephemeral env with this PR's images (`/test on-demand-e2e`) |

## Commit Conventions

Use [conventional commits](https://www.conventionalcommits.org/):

```
feat: add new trusted action
fix: handle timeout in dispatch request
docs: update development guide
chore: bump golangci-lint to v2.13.0
```

## Releasing

The CLI version is defined in the `VERSION` variable in the `Makefile`. Bump it
and merge to `main`. The [Release CLI](../.github/workflows/release-cli.yml)
workflow then:

1. Skips if GitHub Release `v$(VERSION)` already exists (Makefile-only changes
   with an unchanged version are no-ops).
2. Cross-compiles `zoa` for `linux/amd64`, `linux/arm64`, `darwin/amd64`, and
   `darwin/arm64` (`make dist`).
3. Writes `SHA256SUMS` next to those binaries.
4. Creates the git tag and GitHub Release with generated notes plus the assets.

Install instructions for end users are in the
[CLI Reference](cli-reference.md#install).

Lambda, runner, and boundary images are separate; they are not GitHub Release assets.

## Container Images

| Image | Containerfile | Purpose |
|-------|---------------|---------|
| `zoa-lambda` | `Containerfile` | Lambda container (UBI-minimal + `zoa-lambda` binary) |
| `zoa-runner` | `Containerfile.runner` | Async runner (UBI9 + `zoa-runner` + `zoa` CLI, runs inside K8s Jobs) |
| `zoa-boundary` | `Containerfile.boundary` | Boundary ECS task (UBI9 + `zoa` CLI, aws/kubectl/jq, Claude Code via Bedrock) |

Default Quay repos for local push: `quay.io/rrp-dev-ci/zoa-{lambda,runner,boundary}` (override with `IMAGE_REPO`, `RUNNER_IMAGE_REPO`, `BOUNDARY_IMAGE_REPO`).

```bash
make image-lambda          # Build Lambda image only
make image-runner          # Build runner image only
make image-boundary        # Build boundary image only
make image-push-lambda     # Build + push Lambda (:latest + :commit)
make image-push-runner     # Build + push runner (:latest + :commit)
make image-push-boundary   # Build + push boundary (:latest + :commit)
make images-push           # Build + push all three images (dev workflow)
```

## GVM Users

If using GVM and `make test` fails with `go: no such tool "covdata"`:

```bash
chmod u+w $(go env GOROOT)/pkg/tool/linux_amd64/
GOWORK=off go build -o $(go env GOROOT)/pkg/tool/linux_amd64/covdata cmd/covdata
```
