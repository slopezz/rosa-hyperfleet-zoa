# End-to-End Testing

Practical guide for ZOA's e2e coverage: what runs where, how to run it yourself against a real
environment (dev-account ephemeral, standing integration, or CI), and how container images flow
from a PR into that environment.

## Break-glass (ROSAENG-75954)

Break-glass e2e will follow the session + TA patterns in `test/e2e/`:

1. Boundary: `breakglass request` → peer approve from laptop → `breakglass use` → read-only `kubectl get ns`.
2. Expiry: worker marks grant expired and GC removes `zoa-bg-<requestId>` RBAC.

Gate on ephemeral when `BREAKGLASS_TABLE` is wired (same RC outputs as executions/sessions).

## Test Suites

ZOA has two independent test suites that compose via Makefile targets:

| Suite          | Path                   | What it validates                                                                               | Requires                                                                                                                                                                   |
| -------------- | ---------------------- | ----------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Functional** | `test/e2e/`            | ZOA CLI, Trusted Actions, session plane e2e (per RC/MC worker), executor, RBAC, cooldown, audit | `ZOA_RC_API_URL` and/or `ZOA_MC_API_URL`; **`ZOA_DEPLOYMENT_NAME`** (required); session specs when name contains `eph` + `ZOA_CENTRAL_AWS_PROFILE` (default `rrp-central`) |
| **Monitoring** | `test/e2e-monitoring/` | Observability pipeline — metrics in Thanos, recording rules, alerts                             | `RHOBS_API_URL` (required)                                                                                                                                                 |

Both suites use **Ginkgo labels** to choose which specs a Makefile target runs:

- **`Label("smoke")`**: tags specs that are safe and fast enough for **consumer presubmits**
  (`rosa-seaplane` / `rosa-seaplane-api` call `make test-e2e-smoke`, which applies
  `--label-filter=smoke` to **both** functional and monitoring suites). Smoke-labeled specs **also run** under **`make test-e2e`** and
  **`make test-e2e-zoa`** (no label filter) — nightlies, ZOA `on-demand-e2e`, and standing-env
  jobs exercise the full functional suite, which includes every smoke spec plus deep TA coverage.
- **`Label("full")`** (monitoring only): identity bridge metrics and recording-rule value checks that require boundary exec-script traffic; skipped by `make test-e2e-smoke`.
- **Full functional** (default ginkgo run): all specs in `test/e2e/`, smoke-labeled or not.

### Makefile Targets

| Target                     | What runs                                       |
| -------------------------- | ----------------------------------------------- |
| `make test-e2e`            | Functional full → Monitoring full               |
| `make test-e2e-smoke`      | Functional smoke → Monitoring smoke             |
| `make test-e2e-zoa`        | Functional full only                            |
| `make test-e2e-zoa-smoke`  | Functional smoke only                           |
| `make test-e2e-monitoring` | Monitoring full only (requires `RHOBS_API_URL`) |

The default targets (`test-e2e`, `test-e2e-smoke`) chain both suites. `RHOBS_API_URL` is
required — missing it fails the suite. Use `test-e2e-zoa` / `test-e2e-zoa-smoke` to run
functional tests only.

### Functional Suite Details

**Smoke-labeled TA specs** (~2 min per parallel worker): `zoa version`, `zoa actions`, kube/aws
reads, `rollout_restart` dry-run, one async `--wait`. These run on consumer PRs (smoke filter)
and again as part of the full functional suite.

**Session plane e2e** (in `test/e2e/`, **`ZOA_DEPLOYMENT_NAME` contains `eph`** until int/stage has **`rrp-central`**):

- **`Label("smoke")`** (consumer **`make test-e2e-smoke`**, **8m** per RC/MC worker): **`zoa deployments`**, **`zoa targets`**, session CLI validation, **`session start --no-connect`** lifecycle (list, history, terminate, list terminated).
- **Full functional only** (**`make test-e2e`**, **20m** per worker): boundary **`start|join --exec-script`** (Exec, **`claude -p`**, **`zoa run`** with **`operator`/`session_id` bridge checks**, MOTD/PS1/reason/exit hints, **`exec_session_ids`**, **`zoa get`**) and the exec-script failure path. TA-plane specs assert **`session_id` absent** on direct **`zoa run`** / **`zoa get`** (worker profile, not boundary).

See [Access boundary sessions](#access-boundary-sessions-zoa-access-e2e).

**Full functional** (beyond the smoke subset): every registered Trusted Action, both scopes (`kube-api`/`aws-api`),
read+write+dry-run, sync+async, RC and MC, real (non-dry-run) `delete_pod`/`rollout_restart`
against `kube-system/coredns`, cooldown enforcement, all CLI commands (`get`, `runs`, `audit`,
`output`, `logs`, `download`), generic param validation, e2e conformance checks.

### Monitoring Suite Details

**Smoke** (`make test-e2e-smoke`, functional and monitoring both use `--label-filter=smoke`): recording rules loaded in Thanos Ruler, alerting rules loaded, infrastructure metrics present, execution metrics from smoke TAs and Access session lifecycle, recording rule **values** that do not require boundary exec-script. **`Label("full")` only:** identity bridge metric/value checks (boundary `session start --exec-script` + in-task `zoa run`) and **ECS Container Insights `TaskCount`** for `*-zoa-boundary` (smoke `session start --no-connect` + immediate terminate leaves tasks too short-lived for CloudWatch CI to emit).

**Full** (`make test-e2e`, monitoring runs without label filter): all smoke monitoring specs plus identity bridge raw metrics, `zoa:identity_bridge_hit_rate` recording rule values, boundary Container Insights TaskCount, and no critical alerts in `firing` state when those tests exist.

See [docs/observability.md](observability.md) for the metrics catalog and alerting philosophy.

**Runner:** Functional and monitoring e2e use **`ginkgo -vv`** (spec names, steps, `GinkgoWriter`
output — same style as `rosa-seaplane-api` `test-e2e-platform-monitoring`). Monitoring also writes
**`junit-zoa-monitoring.xml`** to `TEST_OUTPUT_DIR` (`ARTIFACT_DIR` in Prow, `./test-results`
locally). Suite timeout defaults to **`15m`** (`E2E_MONITORING_TIMEOUT`). Extra ginkgo flags for
functional tests only: `GINKGO_FLAGS='--focus=must_gather' make test-e2e-zoa`.

### Metric Lag

The CloudWatch pipeline latency is 5–7 minutes. Monitoring tests run after functional tests
and use `Eventually("5m", "15s")`, so the total window from first TA execution to metric
assertion is sufficient. Infrastructure metrics (Lambda invocations, reconciler ticks) are
always-on and do not depend on test-driven TA execution.

## Developer Workflow: Testing Code Changes

When you change ZOA code (API, Trusted Actions, executor, etc.) and want to validate against a
live ephemeral environment, you must **build and deploy your own images first**. The e2e test
suite only exercises the `zoa` CLI — the Lambda running on the ephemeral must already contain
your code changes.

> **Important:** Automatic image injection (CI builds your PR's images and deploys them) only
> happens in CI via the `on-demand-e2e` Prow job. For local development, you manage the image
> lifecycle manually.

### Image Naming Reference

| Source                   | Image Tag Format           | Registry                                                             |
| ------------------------ | -------------------------- | -------------------------------------------------------------------- |
| **PR builds** (Tekton)   | `on-pr-<commit-sha>`       | `quay.io/redhat-user-workloads/rosa-tenant/rosa-seaplane-zoa-lambda` |
| **Main branch** (Tekton) | `<commit-sha>`             | `quay.io/redhat-user-workloads/rosa-tenant/rosa-seaplane-zoa-lambda` |
| **Dev builds** (local)   | `latest` or `<commit-sha>` | `quay.io/rrp-dev-ci/zoa-lambda`                                      |
| **Manual**               | Any tag you choose         | Your quay.io namespace                                               |

PR images expire after 5 days. Main-branch and dev images persist indefinitely.

### Step-by-step (Using PR Images)

The recommended approach is to push your changes to a branch and let Tekton build the images automatically:

```bash
# 1. Push your code to trigger PR image build
cd rosa-seaplane-zoa
git push origin HEAD:feat/my-feature   # or push to main

# 2. Open a PR on GitHub in normal mode (NOT draft) with hold label
# Use --label to prevent accidental merges during testing
gh pr create \
  --title "feat: your feature description" \
  --body "Description of changes..." \
  --label "do-not-merge/hold" \
  --base main

# 3. Trigger the Konflux build (required when pushing from a fork)
# Add this comment to the PR to start the build
gh pr comment <pr-number> --body "/ok-to-test"

# 4. Wait for the build check to pass in the PR
# Tekton will build: quay.io/redhat-user-workloads/rosa-tenant/rosa-seaplane-zoa-lambda:on-pr-<commit>

# 5. Get your commit SHA
git rev-parse --short HEAD   # e.g. 1fa64b3

# 6. Configure ephemeral override in rosa-seaplane
cd ../rosa-seaplane
mkdir -p .ephemeral-env

# Create override config pointing to your PR images
# Note: If testing sync actions only, you can omit zoa_runner_* overrides
# The runner image is only needed for async actions
cat > .ephemeral-env/defaults.yaml <<EOF
regional_cluster:
  zoa_lambda_image_tag: "on-pr-1fa64b3"
  zoa_runner_image_tag: "on-pr-1fa64b3"
  zoa_boundary_image_tag: "on-pr-1fa64b3"
  zoa_lambda_source_image: "quay.io/redhat-user-workloads/rosa-tenant/rosa-seaplane-zoa-lambda"
  zoa_runner_source_image: "quay.io/redhat-user-workloads/rosa-tenant/rosa-seaplane-zoa-runner"
  zoa_boundary_source_image: "quay.io/redhat-user-workloads/rosa-tenant/rosa-seaplane-zoa-boundary"
EOF

# Alternative: Override only Lambda image (if runner build is still pending)
# cat > .ephemeral-env/defaults.yaml <<EOF
# regional_cluster:
#   zoa_lambda_image_tag: "on-pr-1fa64b3"
#   zoa_lambda_source_image: "quay.io/redhat-user-workloads/rosa-tenant/rosa-seaplane-zoa-lambda"
# EOF

# Create region file (required for .ephemeral-env/)
cat > .ephemeral-env/us-east-1.yaml <<EOF
provision_mcs:
  mc01: {}
EOF

# 7. Resync the ephemeral to deploy the new Lambda
make ephemeral-resync ID=<your-env-id>

# 8. Run e2e tests (picks up your code changes via the new Lambda)
make ephemeral-zoa-e2e ID=<your-env-id> \
  ZOA_REF=feat/my-feature \
  ZOA_REPO=https://github.com/openshift-online/rosa-seaplane-zoa.git
```

### Alternative: Manual Image Build and Push

If you need to test uncommitted changes or don't have PR access, you can build and push manually:

```bash
# 1. Build + push both images to Quay (requires quay.io login and write access)
cd rosa-seaplane-zoa
podman login quay.io
make images-push IMAGE_REPO=quay.io/your-username/zoa-lambda \
                RUNNER_IMAGE_REPO=quay.io/your-username/zoa-runner \
                IMAGE_TAG=test-my-feature

# 2. Configure ephemeral override to use your images
cd ../rosa-seaplane
mkdir -p .ephemeral-env

cat > .ephemeral-env/defaults.yaml <<EOF
regional_cluster:
  zoa_lambda_image_tag: "test-my-feature"
  zoa_runner_image_tag: "test-my-feature"
  zoa_lambda_source_image: "quay.io/your-username/zoa-lambda"
  zoa_runner_source_image: "quay.io/your-username/zoa-runner"
EOF

cat > .ephemeral-env/us-east-1.yaml <<EOF
provision_mcs:
  mc01: {}
EOF

# 3. Resync and test
make ephemeral-resync ID=<your-env-id>
make ephemeral-zoa-e2e ID=<your-env-id>
```

If you are only changing **test code** (files in `test/e2e/` or `test/e2e-monitoring/`) and
not API/TA code, skip the image build steps entirely — the existing Lambda is fine, you just
need the tests to use your branch (via `ZOA_REF`).

**Notes:**

- The `.ephemeral-env/` directory is gitignored and only affects your local machine
- PR images expire after 5 days (sufficient for testing)
- **Do not create draft PRs** - Konflux builds may not trigger for drafts; use `do-not-merge/hold` label instead
- When pushing from a fork, you must add `/ok-to-test` comment to trigger builds
- **Lambda vs Runner builds**: If testing sync actions, you only need the Lambda image (runner builds can be skipped or left at default). Runner is only needed for async actions.
- Konflux builds Lambda and Runner in parallel, but Lambda usually finishes first - you can start testing sync actions immediately
- If you need to iterate quickly on uncommitted changes, use the manual build approach
- Remember to remove or update `.ephemeral-env/defaults.yaml` after testing to avoid using stale images
- Remove the `do-not-merge/hold` label when ready to merge

### When is image management automatic?

| Scenario                        | Image management                                                                          |
| ------------------------------- | ----------------------------------------------------------------------------------------- |
| **CI `on-demand-e2e`** (ZOA PR) | Automatic — ci-operator builds images, pushes to Quay, and overrides config before deploy |
| **CI `nightly-ephemeral`**      | None — tests whatever tags are already pinned in `rosa-seaplane@main` config              |
| **Local dev testing**           | **Manual** — you build, push, configure, and resync (steps above)                         |

## Running the Tests

Both suites drive against a real, already-provisioned environment. They never need a specific
environment shape beyond "give me a URL" — the same suites run unmodified against a dev-account
ephemeral env, a CI ephemeral env, or (in principle) a standing integration environment.

### From `rosa-seaplane` (Recommended)

You need an ephemeral environment already provisioned (`make ephemeral-provision` in that repo —
see [`rosa-seaplane/docs/development-environment.md`](https://github.com/openshift-online/rosa-seaplane/blob/main/docs/development-environment.md)).

```bash
cd rosa-seaplane

# Full suite (functional + monitoring) — clones rosa-seaplane-zoa@main inside the test container.
# AWS credentials (rrp-rc, rrp-mc profiles) and RHOBS_API_URL are set up automatically.
make ephemeral-zoa-e2e \
  ID=<your-env-id> \
  ZOA_REF=my-feature-branch \
  ZOA_REPO=https://github.com/my-fork/rosa-seaplane-zoa.git

# Smoke only (~2min)
make ephemeral-zoa-e2e-smoke ID=<your-env-id>

# Verbose output
GINKGO_FLAGS='--focus=must_gather' make ephemeral-zoa-e2e ID=<your-env-id>

# Only functional tests (skip monitoring)
ZOA_MAKE_TARGET=test-e2e-zoa make ephemeral-zoa-e2e ID=<your-env-id>

# Only monitoring tests
ZOA_MAKE_TARGET=test-e2e-monitoring make ephemeral-zoa-e2e ID=<your-env-id>
```

`ZOA_REF` defaults to `main`, `ZOA_REPO` defaults to the upstream repo. To test uncommitted
changes, push them to a branch first — there is no local-checkout-mount option (same convention
as `E2E_REF`/`E2E_REPO` for API tests and `CLI_REF`/`CLI_REPO` for CLI tests).

This is the recommended path because:

- AWS credentials are handled automatically (the container maps your host profiles to
  `rrp-rc`/`rrp-mc` inside the container).
- ZOA Lambda URLs and RHOBS API URL are resolved from Terraform outputs.
- No manual env var setup needed.

### From This Repo Directly

If you are iterating on test code and want fast feedback without container overhead,
you can run the suite directly against an existing ephemeral environment. You need the
Lambda Function URLs and AWS credentials for both the RC and MC accounts.

**Step 1 — Get the Lambda URLs.** From the `rosa-seaplane` checkout:

```bash
make ephemeral-list ID=<your-env-id>
# Look for the ZOA RC and MC API URLs and RHOBS API URL in the output
```

**Step 2 — Run both RC and MC together.** Your host `~/.aws/config` has profiles named
`rrp-regional-dev` and `rrp-management-dev`, but the test suite defaults to `rrp-rc` and `rrp-mc`
(the names used inside CI containers). Override with `ZOA_RC_AWS_PROFILE` and `ZOA_MC_AWS_PROFILE`:

```bash
cd rosa-seaplane-zoa

export ZOA_RC_API_URL="https://yyyy.lambda-url.us-east-1.on.aws/"
export ZOA_MC_API_URL="https://zzzz.lambda-url.us-east-1.on.aws/"
export ZOA_RC_AWS_PROFILE="rrp-regional-dev"
export ZOA_MC_AWS_PROFILE="rrp-management-dev"
export RHOBS_API_URL="https://xxxxx.execute-api.us-east-1.amazonaws.com/prod"

make test-e2e              # full: functional + monitoring
make test-e2e-smoke        # smoke: functional + monitoring
make test-e2e-zoa          # full: functional only
make test-e2e-monitoring   # monitoring only (ginkgo -vv + junit)
```

RC and MC run in **parallel** automatically (separate `go test` processes, one per target). Each
process runs its specs sequentially. If you only set one URL, only that target runs.

Each e2e `target` carries a `TargetType` field (`"rc"` or `"mc"`) derived from which URL env var was set. Specs
use this to pick the correct `--gather` value and expected tarball layout without hard-coding RC vs
MC — so the same test file is safe under parallel execution (each process only sees one target).

**Step 3 (optional) — Run only RC or only MC.**

```bash
# RC only
export ZOA_RC_API_URL="https://yyyy.lambda-url.us-east-1.on.aws/"
export ZOA_RC_AWS_PROFILE="rrp-regional-dev"
make test-e2e

# MC only
export ZOA_MC_API_URL="https://zzzz.lambda-url.us-east-1.on.aws/"
export ZOA_MC_AWS_PROFILE="rrp-management-dev"
make test-e2e
```

### `must_gather` platform dumps

`ta_mustgather_test.go` exercises async platform collection on the endpoint under test:

| Target | CLI           | Tarball marker                |
| ------ | ------------- | ----------------------------- |
| RC     | `--gather rc` | `rc/namespaces/platform-api/` |
| MC     | `--gather mc` | `mc/namespaces/kube-applier/` |

The happy-path spec runs `must_gather --wait` (up to 20m), downloads `output.tar.gz` via
`zoa download`, and asserts the deployment-specific namespace path exists while the opposite
deployment tree (`mc/` or `rc/`) does not. HCP gather (`gather=hcp`) is covered separately later
via `E2E_HCP_CLUSTER_ID` — not part of the default deep suite yet.

Run only must_gather specs:

```bash
GINKGO_FLAGS='--focus=must_gather' make test-e2e-zoa
```

## AWS Credentials

RC and MC are **separate AWS accounts** with separate IAM principals. The test suite handles this
by setting `AWS_PROFILE` on each subprocess call (not globally), so both targets can run in the
same session without conflicting.

### Profile Names

The test suite defaults to `rrp-rc` and `rrp-mc` (the names used inside CI containers and
`make ephemeral-zoa-e2e`). On your host, ephemeral dev accounts use different names:

| Test suite default | Your host profile    | Purpose                           |
| ------------------ | -------------------- | --------------------------------- |
| `rrp-rc`           | `rrp-regional-dev`   | RC account — invoke RC ZOA Lambda |
| `rrp-mc`           | `rrp-management-dev` | MC account — invoke MC ZOA Lambda |

Override with `ZOA_RC_AWS_PROFILE` / `ZOA_MC_AWS_PROFILE` when running from your host.
Inside CI containers or `make ephemeral-zoa-e2e`, the defaults (`rrp-rc`/`rrp-mc`) are correct —
no override needed.

The monitoring suite reuses `ZOA_RC_AWS_PROFILE` for SigV4 signing against the RHOBS API
Gateway (which requires RC account credentials).

### How the Test Suite Uses Profiles

The test suite (`test/e2e/helpers_test.go`) sets `AWS_PROFILE` per-subprocess, filtering out any
inherited `AWS_PROFILE`/`ZOA_API_URL` to avoid duplicate keys:

```go
func runZoa(tgt target, args ...string) (string, error) {
    cmd := exec.Command(zoaBin, args...)
    cmd.Env = append(filterEnv(os.Environ(), "AWS_PROFILE", "ZOA_API_URL"),
        "ZOA_API_URL="+tgt.APIURL,
        "AWS_PROFILE="+tgt.AWSProfile,  // rrp-rc or rrp-mc
    )
    out, err := cmd.CombinedOutput()
    return string(out), err
}
```

This ensures:

- RC specs use `rrp-rc` (can invoke RC Lambda)
- MC specs use `rrp-mc` (can invoke MC Lambda)
- No race conditions even with parallel spec execution

### Credential Setup per Scenario

| Scenario                                               | Who manages credentials                                                                                                                              |
| ------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------- |
| `make ephemeral-zoa-e2e ID=...` (from `rosa-seaplane`) | Automatic — `ephemeral-env.sh` maps your host profiles → `rrp-rc`/`rrp-mc` inside the container                                                      |
| `make test-e2e` (from this repo directly)              | Manual — set `ZOA_RC_AWS_PROFILE=rrp-regional-dev` and `ZOA_MC_AWS_PROFILE=rrp-management-dev`                                                       |
| CI (`nightly-ephemeral`, `on-demand-e2e`)              | Automatic — Vault `aws_config` includes `rrp-central`, `rrp-rc`, and `rrp-mc`; today's Lambda e2e uses only `rrp-rc`/`rrp-mc` (direct Function URLs) |

### Access boundary sessions (ZOA Access e2e)

The functional suite already covers **Trusted Actions** from the RC/MC API profile (`zoa run` against
Lambda URLs). **Session-plane e2e** adds proof that an **SRE operator workflow through ZOA Access**
works end to end: discover deployments and targets from the laptop, start and audit sessions, tear
them down, and — in a second ordered chain — do what a human does after **`zoa session join`**: Exec
into the boundary, run tooling and **`zoa run`** against the session cluster, reconnect, and leave an
audit trail Access can list.

Implementation detail lives in **`test/e2e/`** (spec names and scripts). This section covers **intent**,
**when tests run**, **exec-script harness markers**, and **how to extend** coverage without duplicating the suite.

#### What we are trying to prove

**Laptop (central profile, smoke):** **`zoa deployments`** / **`zoa targets`** autodiscovery;
**`session start --no-connect`**; validation errors for bad **`--reason`** or unknown target;
**`session list`**, **`session history`**, **`session terminate`**, list after terminate.

**Inside the boundary (Exec script, full suite only):** **`session start --exec-script`** runs a real operator script
(bootstrap checks, **`claude -p`**, sync **`zoa run`**); **`join --exec-script`** opens a **second** Exec on
the **same** session and checks MOTD, PS1, reason, and post-Exec hints; **`session list`** shows two
**`exec_session_ids`**; **`zoa get`** from the RC/MC worker profile sees the TA execution; **`terminate`**
cleans up. Also **`join --exec-script 'false'`** after **`start --no-connect -o json`** (non-zero CLI exit).

Each parallel Ginkgo worker (RC vs MC) runs the same stories against its target (`regional` vs `mc01`).
**`session start|join --exec-script`** avoids an interactive PTY on the CI runner; the behavior under test is
still “SRE inside the boundary task.” Session id format and CLI output for automation are in
[CLI reference](cli-reference.md#session-join) and [operator workflow](guides/operator-workflow.md).

#### When these specs run

| Gate                                     | Behavior                                                                                                                    |
| ---------------------------------------- | --------------------------------------------------------------------------------------------------------------------------- |
| **`ZOA_CENTRAL_AWS_PROFILE`**            | Default **`rrp-central`** — Access/session commands, not RC/MC Lambda URLs.                                                 |
| **`ZOA_DEPLOYMENT_NAME` contains `eph`** | Temporary skip outside ephemeral CI until int/stage ship central **`aws_config`**.                                          |
| **`session-manager-plugin` on `PATH`**   | Required for exec-script; consumer CI image installs the plugin under `/usr/local/sessionmanagerplugin/bin`.                |
| **CI ephemeral invoker**                 | Central role **`ci`** must be trusted (Terraform **`ci=true`** on render).                                                  |
| **Ordering**                             | Exec (full): per target, ordered start → join → list → get → terminate. Laptop session specs ordered within their Describe. |

Exec script chain (**full functional only**): **`start --exec-script`** → **`join --exec-script`** → **`session list`**
(two exec ids) → **`zoa get`** → **`terminate`**. Tests use CLI **`--exec-script-timeout`** **`5m`** / **`2m`**
(start vs reconnect). Suite ceilings: **`make test-e2e-smoke`** **8m**, **`make test-e2e`** **20m** per RC/MC worker (Makefile).

#### Why harness markers exist (`__ZOA_E2E_*__`)

**`session … --exec-script`** attaches a PTY. MOTD, **`session-manager-plugin`** banners, **`claude`**
output, and **`zoa run`** tables all land in **one stdout stream** — not structured JSON, and lines
shift when the image or plugin changes.

To assert **`zoa run`** (status, action, execution id) without brittle regex over the whole
transcript, the in-boundary script prints **e2e-only anchors** (not a product API):

| Marker                                     | Role                                                                                                      |
| ------------------------------------------ | --------------------------------------------------------------------------------------------------------- |
| **`__ZOA_E2E_EXEC_JSON__:<compact-json>`** | Single line the suite greps; Go **re-asserts** the execution fields so MOTD/plugin noise does not matter. |
| **`__ZOA_E2E_BOUNDARY_OK__`**              | Script reached the end even when Claude’s reply wording varies.                                           |

The reconnect step (**`join --exec-script`**, **`zoa version`**) relies on **transcript assertions** (MOTD,
metadata rows, PS1, reason, exit hints), not these markers.

When extending the exec script, use **`set -euo pipefail`**, emit **`__ZOA_E2E_EXEC_JSON__`** only
when the test must read TA output from the PTY, and prefer **Go assertions** over parsing Claude or
plugin text.

#### Adding or changing session-plane e2e

- Read neighboring specs under **`test/e2e/`** and match their Ginkgo layout (ordered chains,
  **`Label("smoke")`** only when the spec belongs in consumer presubmits).
- **Central profile** for Access/session CLI; **RC/MC profile** for TA persistence checks (same split
  as the rest of functional e2e).
- **Laptop-only** checks do not need boundary Exec; **in-boundary** checks extend the exec-script operator
  script or a small reconnect script.
- Keep the **`eph`** deployment-name gate until standing env CI has central credentials.
- Use harness markers sparingly — only to anchor JSON in noisy exec-script stdout.

Functional suite overview for TA depth stays in [Functional Suite Details](#functional-suite-details)
above.

## CI: How It Runs

### ZOA's Own Jobs (`openshift/release`)

Config:
[`ci-operator/config/openshift-online/rosa-seaplane-zoa/`](https://github.com/openshift/release/blob/master/ci-operator/config/openshift-online/rosa-seaplane-zoa/openshift-online-rosa-seaplane-zoa-main.yaml)
in `openshift/release`.

**`on-demand-e2e`** (presubmit, `/test on-demand-e2e` on a ZOA PR, `always_run: false`):

ci-operator builds both images (`Containerfile` → `zoa-lambda`, `Containerfile.runner` → `zoa-runner`),
the `rosa-seaplane-image-push` step mirrors them to `quay.io/rrp-dev-ci/zoa-{lambda,runner}` with
a `ci-<PR>-<BUILD_ID>` tag, and the provision step overrides `zoa_lambda_image_tag`,
`zoa_runner_image_tag`, `zoa_lambda_source_image`, and `zoa_runner_source_image` in
`config/defaults.yaml` before deploying. The e2e step auto-detects the PR's ZOA repo/branch so
the deep suite runs against the PR's own code and images.

**`nightly-ephemeral`** (periodic): provisions with whatever image tags are pinned in
`rosa-seaplane@main`'s config (Konflux-built). Catches infrastructure/platform regressions.

### Consumer Repo Jobs (`rosa-seaplane` / `rosa-seaplane-api`)

Their `nightly-ephemeral`, `on-demand-e2e`, and `rosa-regionality-compatibility-e2e` jobs call
`rosa-seaplane/ci/e2e-tests.sh`, which clones `rosa-seaplane-zoa@main` and runs:

- PRs (`JOB_TYPE=presubmit`): `make test-e2e-smoke` (smoke TA + Access laptop session specs when `eph`; **no** boundary Exec script)
- Nightlies (`JOB_TYPE=periodic`): `make test-e2e` (full functional + monitoring — includes boundary Exec script and all smoke specs)

These jobs always test against whatever image tags are pinned in `rosa-seaplane` — they never
inject a custom ZOA image.

## Image Management

### How Images Are Published

| System      | When                       | Registry                                                                               | Tag format                           | Expiry           |
| ----------- | -------------------------- | -------------------------------------------------------------------------------------- | ------------------------------------ | ---------------- |
| Konflux     | Every push to `main`       | `quay.io/redhat-user-workloads/rosa-tenant/rosa-seaplane-zoa-{lambda,runner,boundary}` | Full commit SHA (e.g. `8af92e65...`) | Never            |
| ci-operator | ZOA PR `on-demand-e2e` job | `quay.io/rrp-dev-ci/zoa-{lambda,runner}`                                               | `ci-<PR>-<BUILD_ID>`                 | No expiry policy |

Konflux builds are the **production** images (with attestation/SAST scans). The `config/defaults.yaml`
pin in `rosa-seaplane` always points at a Konflux-built tag.

ci-operator builds are used **only** for pre-merge CI testing.

### How `rosa-seaplane` Consumes Image Tags

Six config keys in `rosa-seaplane`'s `config/defaults.yaml` control which ZOA images get
deployed: `zoa_lambda_image_tag`, `zoa_runner_image_tag`, `zoa_boundary_image_tag`,
`zoa_lambda_source_image`, `zoa_runner_source_image`, and `zoa_boundary_source_image`. These
flow through `render.py` into Terraform inputs. Lambda images are mirrored to ECR at deploy
time; runner and boundary images are pulled directly from Quay (ECS / Kubernetes).

See [`rosa-seaplane/docs/adding-component-pre-merge.md`](https://github.com/openshift-online/rosa-seaplane/blob/main/docs/adding-component-pre-merge.md#multi-image--terraform-deployed-components-zoa)
for the full image override mechanism.

## Troubleshooting

| Symptom                                                                      | Cause                                                              | Fix                                                                                                                               |
| ---------------------------------------------------------------------------- | ------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------- |
| `no target configured` test failure                                          | Neither `ZOA_RC_API_URL` nor `ZOA_MC_API_URL` is set               | Export at least one URL before running `make test-e2e`                                                                            |
| MC specs don't run, only RC                                                  | `ZOA_MC_API_URL` not set — env may not have an MC provisioned      | Set `ZOA_MC_API_URL`; if using `make ephemeral-zoa-e2e`, MC should always be available                                            |
| `InvalidSignatureException` or `AccessDeniedException` on MC calls           | Using the RC profile to call the MC Lambda (different AWS account) | Set `ZOA_MC_AWS_PROFILE=rrp-management-dev` (or your MC profile); verify with `AWS_PROFILE=<profile> aws sts get-caller-identity` |
| `InvalidSignatureException` or `AccessDeniedException` on RC calls           | Wrong profile for the RC account                                   | Set `ZOA_RC_AWS_PROFILE=rrp-regional-dev` (or your RC profile); verify with `AWS_PROFILE=<profile> aws sts get-caller-identity`   |
| `make ephemeral-zoa-e2e` fails to clone with a branch-not-found error        | `ZOA_REF` points at a branch that hasn't been pushed               | Push your branch first — this clones from the remote, not your local working tree                                                 |
| Monitoring suite fails                                                       | `RHOBS_API_URL` not set                                            | Export `RHOBS_API_URL`; get it from `make ephemeral-list` in `rosa-seaplane`. Use `test-e2e-zoa` to skip monitoring.              |
| Monitoring specs fail with `403 Forbidden`                                   | SigV4 signing failed — wrong profile or expired session            | Verify with `awscurl --service execute-api --region us-east-1 "$RHOBS_API_URL/api/v1/query?query=up"` using the RC profile        |
| Monitoring specs timeout on `Eventually` for metrics                         | CloudWatch pipeline lag > 5 min (transient)                        | Retry; if persistent, check YACE pod health: `oc get pod -n cloudwatch-exporter`                                                  |
| Monitoring log shows `panic: test timed out` with only dots (older Makefile) | Former `go test -timeout 7m` killed the package mid-`Eventually`   | Use ginkgo runner on `main`; logs show failing spec; tune `E2E_MONITORING_TIMEOUT` if needed                                      |
