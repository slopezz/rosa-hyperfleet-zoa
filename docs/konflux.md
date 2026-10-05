# Konflux

ZOA container images build on Konflux (`rosa-tenant` / `kflux-prd-rh02`).

| Component | Containerfile | Quay image |
|-----------|---------------|------------|
| `zoa-lambda` | `Containerfile` | `quay.io/redhat-user-workloads/rosa-tenant/zoa-lambda` |
| `zoa-runner` | `Containerfile.runner` | `quay.io/redhat-user-workloads/rosa-tenant/zoa-runner` |
| `zoa-boundary` | `Containerfile.boundary` | `quay.io/redhat-user-workloads/rosa-tenant/zoa-boundary` (Konflux onboarding planned) |

Local dev builds (before Konflux or for ephemeral): `make image-boundary`, `make image-push-boundary`, or `make images-push` (pushes to `quay.io/rrp-dev-ci/zoa-boundary` by default).

Application: `rosa-hyperfleet-zoa` ([Konflux UI](https://konflux-ui.apps.kflux-prd-rh02.0fk9.p1.openshiftapps.com/ns/rosa-tenant/applications/rosa-hyperfleet-zoa/activity))

The `zoa` CLI is built into the `zoa-runner` image and released via `.github/workflows/release-cli.yml` — no separate Konflux component.

Release-data components `zoa-lambda` and `zoa-runner` are registered in [konflux-release-data MR !21738](https://gitlab.cee.redhat.com/releng/konflux-release-data/-/merge_requests/21738) (merged). Images use UBI9 bases and pass standard Enterprise Contract policy — no custom ECR or RPM signature exceptions are required.

## Pipelines

| PipelineRun | Component | Trigger |
|-------------|-----------|---------|
| `zoa-lambda-on-pull-request` / `zoa-lambda-on-push` | `zoa-lambda` | `Containerfile`, `cmd/zoa-lambda/**`, shared Go paths |
| `zoa-runner-on-pull-request` / `zoa-runner-on-push` | `zoa-runner` | `Containerfile.runner`, `cmd/zoa-runner/**`, `cmd/zoa/**`, shared Go paths |
| `zoa-boundary-on-pull-request` / `zoa-boundary-on-push` | `zoa-boundary` | Planned — `.tekton/zoa-boundary-*`, `Containerfile.boundary`, shared Go paths |

## Dependency updates (Mintmaker / Renovate)

Konflux **Mintmaker** reads [`renovate.json`](../renovate.json) on a schedule and opens PRs (many **automerge** after Prow + Konflux pass).

| What | How it is updated |
|------|-------------------|
| UBI / go-toolset **base digests** | `dockerfile` manager on `Containerfile*` (`ARG BASE_IMAGE`, `BUILDER_IMAGE`, `FROM …`) |
| **Go modules** | `gomod` manager |
| **Tekton** refs | `tekton` manager on `.tekton/` |
| **kubectl** in boundary | `custom.regex` on `Containerfile.boundary` (`KUBECTL_VERSION`) |
| **Claude Code** in boundary | `custom.regex` on `Containerfile.boundary` (`CLAUDE_CODE_VERSION`, release tags without `v` prefix) |
| **AWS CLI v2** zip | Not pinned to a version today — always “current” installer URL; bump manually or add a version ARG + regex later |
| **jq / vim / …** RPMs | Ride along when **ubi-minimal** digest updates |

`zoa-boundary` Konflux pipelines are planned; until `.tekton/zoa-boundary-*` exists, boundary digest/tool PRs still build via `make image-boundary` / local Podman.

## Merge order

1. [konflux-release-data MR !21897](https://gitlab.cee.redhat.com/releng/konflux-release-data/-/merge_requests/21897) — remove legacy `rosa-hyperfleet-zoa` component; finalize `zoa-lambda` / `zoa-runner`
2. Merge this PR (`.tekton/zoa-lambda-*` + `.tekton/zoa-runner-*`)
3. `openshift/release` — update branch protection required Konflux check names
