# CLI Reference

The `zoa` CLI communicates with ZOA via SigV4-signed HTTPS (Access Lambda for sessions and discovery, API Lambda for Trusted Actions inside the boundary).

## Table of contents

- [Design philosophy](#design-philosophy)
- [Install](#install)
- [Configuration](#configuration)
  - [Environment variables and precedence](#environment-variables-and-precedence)
  - [Global flags](#global-flags)
- [Commands](#commands)
- [Examples](#examples)
  - [Sessions](#session-examples)
  - [Trusted Actions](#trusted-action-examples)
- [Special actions](#special-actions)
  - [Seaplane DB](#seaplane-db-actions)
- [Command flags reference](#command-flags-reference)
- [Time formats](#time-formats)
- [Shell completion](#shell-completion)

## Design philosophy

The CLI is deliberately built around **SRE muscle memory** — the patterns and conventions that operators have internalized from years of daily use of `kubectl`, `aws-cli`, and `oc`. This means:

- **Familiar flags**: `-n` for namespace, `-o json` for machine output, `-A` for all-namespaces, `-v` for verbose — no learning curve
- **Composable output**: table by default (human). For `zoa run`, piping to `jq` usually needs **`-v`** (full objects). With **`-o json`**, stdout is the execution envelope — use `jq '.output[]…'`; without **`-o json`**, stdout is TA output only (`jq '.[]…'`).
- **Subcommand structure**: verb-first (`run`, `get`, `describe`, `logs`) mirrors kubectl's mental model
- **Predictable behavior**: `--dry-run` previews, `--force` overrides safety, `--wait` blocks until done — exactly what you'd expect
- **Shell completion**: full zsh/bash/fish completion so discoverability is instant
- **Zero configuration**: inherit AWS credentials from the environment (same as aws-cli), single env var for endpoint

The goal: an SRE who has never seen ZOA should be productive within 30 seconds of reading `zoa --help`.

## Install

Download a pinned GitHub Release binary and verify the checksum.

Linux x86_64:

```bash
VERSION=v0.3.0  # see https://github.com/openshift-online/rosa-seaplane-zoa/releases
curl -fsSL -O "https://github.com/openshift-online/rosa-seaplane-zoa/releases/download/${VERSION}/zoa-linux-amd64"
curl -fsSL -O "https://github.com/openshift-online/rosa-seaplane-zoa/releases/download/${VERSION}/SHA256SUMS"
sha256sum -c SHA256SUMS --ignore-missing
chmod +x zoa-linux-amd64
sudo mv zoa-linux-amd64 /usr/local/bin/zoa
zoa version
```

Other platforms — replace the binary name in the commands above:

| OS / Arch           | Binary             |
| ------------------- | ------------------ |
| Linux x86_64        | `zoa-linux-amd64`  |
| Linux ARM64         | `zoa-linux-arm64`  |
| macOS Intel         | `zoa-darwin-amd64` |
| macOS Apple Silicon | `zoa-darwin-arm64` |

On macOS, `sha256sum` is not installed by default. Use the built-in equivalent:

```bash
shasum -a 256 -c SHA256SUMS --ignore-missing
```

### Development builds

If you work on this repo, use `make build` (or `make install`). These inject
full version metadata via ldflags, so `zoa version` reports the exact commit
and build date — useful for debugging which build is deployed.

## Configuration

The CLI talks to **two different Lambdas** depending on the command. No manual URL configuration is needed for normal use.

### Laptop (sessions, discovery)

```bash
# No env vars needed — autodiscovery via SSM
zoa deployments                    # reads /zoa/deployments/* from Central SSM
zoa targets us-east-1              # resolves Access Lambda URL from SSM
zoa session start us-east-1 regional --reason ROSAENG-1234
```

Use your normal **Jump/Central** AWS login (same default credential chain as `aws` CLI). ZOA reads `/zoa/deployments/<name>` from **Central account SSM** (default: **`us-east-1`**, then **`eu-west-1`** if the first read fails; no `[default] region` or `AWS_REGION` required for discovery). It assumes the Access **invoker** role and calls the Access Lambda. **Target** metadata (`/zoa/targets/...`) is read by that Lambda in the deployment’s AWS region (RC account), not from your laptop. ECS Exec uses **scoped credentials** returned on `session join` — not deployment-account admin roles.

### Boundary (Trusted Actions)

```bash
# ZOA_API_URL is pre-set by the ECS task definition — no manual config
zoa run get_resource --reason ROSAENG-1234 --resource pods
```

Inside the boundary, `ZOA_API_URL` points to the **API Lambda** in the target VPC. **`ZOA_TARGET_TYPE`** (`rc` or `mc`) is injected when the session task starts (and is already set on the API Lambda in Terraform). Operators do not configure either.

### Deployment discovery (usually no flags)

Normal use: **`zoa deployments`**, **`zoa targets <deployment>`**, **`zoa session …`** with **no** discovery flags. The CLI loads deployment metadata from Central SSM with **automatic failover**: **`us-east-1` first**, then **`eu-west-1`** if the first read errors.

To use **one** Central region only (no failover), add **`--deployments-region <aws-region>`** on `deployments`, `targets`, and `session`. **`--access-url`** + **`--invoker-role-arn`** on `targets` and `session` only skip Central deployment SSM autodiscovery (URL and ARN required in advance). See [ZOA deployment discovery](guides/deployments-discovery.md).

### Manual override (API Lambda — Trusted Actions only)

```bash
export ZOA_API_URL="https://<id>.lambda-url.<region>.on.aws"
```

Do **not** use `ZOA_API_URL` for session discovery—that is the **API** plane inside the boundary.

**Identity (invoker vs Exec vs task role, identity bridge):** see [Operator workflow — Identity and SigV4](guides/operator-workflow.md#identity-and-sigv4).

### Environment variables and precedence

**Rule:** when a command exposes a **flag** for the same setting, an explicit flag **wins** over the environment variable. Env vars are defaults (especially in the boundary task); flags are overrides.

| Variable          | Purpose                                                                                    | CLI flag        | Commands                                                           |
| ----------------- | ------------------------------------------------------------------------------------------ | --------------- | ------------------------------------------------------------------ |
| `ZOA_API_URL`     | API Lambda Function URL (which **target VPC / EKS cluster** TAs run against)               | `--api-url`     | `zoa run`, `runs`, `get`, `audit`, … (not session/discovery)       |
| `AWS_REGION`      | Region for SigV4 / custom endpoints                                                        | `--region`      | All commands that call AWS or the API                              |
| `ZOA_REASON`      | Default Jira/PD reason for the session                                                     | `--reason`      | `zoa run`, `zoa session start`                                     |
| `ZOA_TARGET_TYPE` | `rc` or `mc` for **offline** TA catalog filter (same meaning as **TYPE** in `zoa targets`) | `--target-type` | `zoa actions` (alias: `catalog`) / `zoa describe` with `--offline` |

**Not env-backed (flags only):** TA **parameters** on `zoa run` (`-n`, `-A`, `--resource`, `--param`, …) — there is no `ZOA_NAMESPACE`-style env; use flags or `--param`.

**Boundary-only context (set by Access on the ECS task, not run flags):** `ZOA_TARGET`, `ZOA_DEPLOYMENT`, `ZOA_SESSION_ID`, `ZOA_OPERATOR`, session deadline/idle vars. These label the session in the shell and in `~/.claude/ZOA_SESSION.md`; they do **not** replace `zoa run` parameter flags. **Which cluster a TA hits** is always the API behind **`ZOA_API_URL`** (one Lambda per target VPC).

**Reason resolution** (same for `zoa run` and `zoa session start`): `--reason` → `ZOA_REASON` → error if still empty. On the boundary, `ZOA_REASON` is usually set from the session ticket at task start; use `zoa run … --reason TICKET` only for a one-off override.

**Offline catalog:** `--target-type` → `ZOA_TARGET_TYPE` → error if still unset (on a laptop with `--offline`, pass `--target-type` unless env is set). Same flags apply to **`zoa catalog`** (alias for `zoa actions`).

### Global flags

These apply to **every** subcommand (`session`, `run`, `runs`, `audit`, …). When a flag and an env var set the same value, the **flag wins** (see [Environment variables and precedence](#environment-variables-and-precedence) for `ZOA_API_URL`, `AWS_REGION`, `ZOA_REASON`, and `ZOA_TARGET_TYPE`).

| Flag                   | Short | Env var       | Description                                                                                                                   |
| ---------------------- | ----- | ------------- | ----------------------------------------------------------------------------------------------------------------------------- |
| `--api-url`            |       | `ZOA_API_URL` | **API** Lambda URL (Trusted Actions). Not used for session/discovery.                                                         |
| `--deployments-region` |       | _(none)_      | Central SSM region for `/zoa/deployments` (`deployments`, `targets`, `session`; default: failover `us-east-1` → `eu-west-1`). |
| `--access-url`         |       | _(none)_      | Direct Access URL (`targets`, `session` only); requires `--invoker-role-arn`; skips Central deployment SSM autodiscovery.     |
| `--invoker-role-arn`   |       | _(none)_      | Direct Access invoker role (`targets`, `session` only); requires `--access-url`.                                              |
| `--output`             | `-o`  |               | `table` (default), `wide`, or `json`                                                                                          |
| `--region`             |       | `AWS_REGION`  | AWS region for SigV4 and custom endpoints                                                                                     |
| `--help`               | `-h`  |               | Help for the current command                                                                                                  |

## Commands

### Discovery (laptop — Access Lambda via SSM autodiscovery)

| Command                | Description                                                                                                                                 |
| ---------------------- | ------------------------------------------------------------------------------------------------------------------------------------------- |
| `deployments`          | List deployments from Central SSM `us-east-1` (default: deployment, region, Access URL, invoker; `-o wide` adds account ID; `-o json`)      |
| `targets <deployment>` | List targets (RC + MC) for a deployment (default: target, region, type, status; `-o wide` / `-o json` for VPC, subnets, ECS, API URL, etc.) |

### Boundary sessions (laptop — Access Lambda)

| Command                                     | Description                                                                          |
| ------------------------------------------- | ------------------------------------------------------------------------------------ |
| `session start <deployment> <target>`       | Start a boundary session (default: ECS Exec shell; `--no-connect` for metadata only) |
| `session join <deployment/session-id>`      | Join an existing session via ECS Exec                                                |
| `session terminate <deployment/session-id>` | Terminate a session (stops ECS task if running)                                      |
| `session list <deployment>`                 | List your sessions (last 24h by default)                                             |
| `session history <deployment>`              | Session history across all operators (audit view)                                    |

**Session start:** Access runs the boundary task on start; default CLI flow then joins and opens ECS Exec. **`--no-connect`** provisions the task and prints session id / task info without Exec (same pattern as **`zoa run --no-wait`**). Use **`zoa session join`** to attach later.

**Access control (server-side):** the Access Lambda binds each session to the **operator** from your SigV4 identity (IAM role session name). **`session join`** and **`session terminate`** return **403** if you are not the owner. **`session list`** always uses **`scope=mine`** — you only see your own sessions, not another operator's. **`session history`** is the deliberate exception: it lists **all operators** for audit/situational awareness (and those queries are audited). You cannot join someone else's boundary shell even if you know their session id; Exec credentials are vended only after an ownership check. See [Operator workflow — ownership](guides/operator-workflow.md#4-start-session-join-exec-and-ownership) and [Access API — sessions](api-reference.md#get-apiv0sessions).

### Trusted Actions (boundary — API Lambda via `ZOA_API_URL`)

| Command             | Description                                                |
| ------------------- | ---------------------------------------------------------- |
| `run <action>`      | Execute a Trusted Action                                   |
| `runs`              | List recent executions                                     |
| `get <id>`          | Get execution details                                      |
| `output <id>`       | Show execution output                                      |
| `logs <id>`         | Show execution logs                                        |
| `download <id>`     | Download output to a local file                            |
| `actions`           | List all available Trusted Actions (`catalog` is an alias) |
| `describe <action>` | Show TA details (params, scope, timeout)                   |
| `audit`             | View audit trail                                           |

### Offline catalog

`actions` (alias: **`catalog`**) and `describe` support **offline mode** — reading the action registry embedded in the CLI binary, no API call needed. Inside the boundary, `ZOA_TARGET_TYPE` is pre-set by the task definition (`rc` or `mc`), so `zoa actions --offline` shows only the actions available on the current target.

```bash
# Offline catalog for RC (no API URL required)
zoa actions --offline --target-type rc

# Offline catalog for MC
zoa actions --offline --target-type mc

# Markdown catalog (used by Claude Code / boundary CLAUDE.md)
zoa actions --offline -o markdown

# Describe a specific action offline
zoa describe get_resource --offline --target-type rc
```

Inside the boundary, `--target-type` defaults to `ZOA_TARGET_TYPE` (set by the task). On the laptop, you must pass `--target-type` explicitly with `--offline`.

The **live API** (`zoa actions` without `--offline`) is the authoritative source when connected — the offline catalog is a convenience for disconnected use and agent context.

### General

| Command      | Description                       |
| ------------ | --------------------------------- |
| `version`    | Print client and server version   |
| `completion` | Generate shell completion scripts |

## Examples

### Session examples

```bash
# Discover deployments and targets
zoa deployments
zoa targets us-east-1

# Start a boundary session (opens ECS Exec shell)
zoa session start us-east-1 regional --reason ROSAENG-1234

# Start without connecting (metadata only, attach later)
zoa session start us-east-1 mc01 --reason ROSAENG-1234 --no-connect

# List your sessions
zoa session list us-east-1
zoa session list us-east-1 --status active
zoa session list us-east-1 -t regional

# Re-join an existing session
zoa session join us-east-1/1989937d-0889-485d-b64e-facee9e0150d

# Terminate a session
zoa session terminate us-east-1/1989937d-0889-485d-b64e-facee9e0150d

# View session history across all operators
zoa session history us-east-1
zoa session history us-east-1 --operator slopezma --since 7d
zoa session history us-east-1 --status terminated --since 7d
zoa session history us-east-1 -o json
```

DR flags (`--deployments-region`, `--access-url`, `--invoker-role-arn`): see [ZOA deployment discovery](guides/deployments-discovery.md).

### Trusted Action examples

```bash
# Discover
zoa actions
zoa describe get_resource

# Read resources (sync, default)
zoa run get_resource --reason ROSAENG-1234 --namespace kube-system --resource pods
zoa run get_resource --reason ROSAENG-1234 --resource nodes
zoa run get_resource --reason ROSAENG-1234 --namespace cert-manager --resource deployments

# Read with verbose output (full API objects)
zoa run get_resource --reason ROSAENG-1234 --resource pods -A --verbose

# AWS API reads
zoa run list_eks_clusters --reason ROSAENG-1234
zoa run describe_eks_cluster --reason ROSAENG-1234 --name my-cluster

# Write actions (have cooldown)
zoa run delete_pod --reason ROSAENG-1234 --namespace grafana --name grafana-abc123
zoa run rollout_restart --reason ROSAENG-1234 --namespace cert-manager --resource deployment --name cert-manager

# Dry-run a write action (executes the read preview)
zoa run delete_pod --reason ROSAENG-1234 --namespace grafana --name grafana-abc123 --dry-run

# Force (bypass cooldown)
zoa run delete_pod --reason ROSAENG-1234 --namespace grafana --name grafana-abc123 --force

# Async execution (for longer-running operations)
zoa run get_resource --reason ROSAENG-1234 --namespace kube-system --resource pods --async
zoa run get_resource --reason ROSAENG-1234 --namespace kube-system --resource pods --async --wait

# View results
zoa runs --limit 10
zoa runs -o wide              # Full details: dispatched/completed timestamps, log bytes
zoa runs --since 7d           # Last 7 days
zoa runs --since 2026-08-20 --until 2026-08-25  # Date range
zoa get <exec-id> --include-output
zoa output <exec-id>
zoa logs <exec-id>
zoa download <exec-id>
zoa download <exec-id> -f /tmp/output.json

# JSON output (for piping)
zoa runs -o json
zoa get <exec-id> -o json

# Audit (unified trail: TA runs, session_*, and API meta reads)
zoa audit --limit 20
zoa audit --since 7d --target my-cluster
zoa audit --action get_db_resource
zoa audit --action runs_list
zoa audit --action audit_query
zoa audit --since 2026-08-01 --until 2026-08-15
```

## Special actions

Focused guides for Trusted Actions that need extra context beyond `zoa describe`.

### Seaplane DB actions

`get_db_resource` and `list_db_gvks` read Seaplane control-plane resources directly from the seaplane-db (Aurora PostgreSQL), bypassing the Kubernetes API. They run on the **RC** ZOA (`ZOA_TARGET_TYPE=rc` on that Lambda) and authenticate to the database with an RDS IAM token. The table holds the Seaplane API custom resources (`hyperfleet.io/v1alpha1`): `Cluster`, `NodePool`, `Placement`, `DNSReservation`, `Index`, `OidcConfig` (all namespaced, one namespace per hosted cluster: `cluster-<uuid>`), and `ManagementCluster` (cluster-scoped).

Resources are keyed by GVK (`Group/Version/Kind`). The core group has an empty group, so if a core resource is ever present its GVK starts with a leading slash (e.g. `/v1/ConfigMap`).

By default the output is a kubectl-style summary table (NAMESPACE, NAME, VERSION, AGE, STATE). Add `-v` for the full objects (spec/status/metadata). To pipe into `jq`, use `-v` and either pipe **stdout** directly (default format) or add **`-o json`** and select the `output` field (execution envelope).

```bash
# Discover which resource types exist (with object counts). Run this first.
zoa run list_db_gvks --reason ROSAENG-1234

# List all objects of a type across every namespace
zoa run get_db_resource --reason ROSAENG-1234 --param gvk=hyperfleet.io/v1alpha1/Cluster -A

# List a type within a single hosted-cluster namespace
zoa run get_db_resource --reason ROSAENG-1234 --param gvk=hyperfleet.io/v1alpha1/NodePool -n cluster-38ae9f84-aa1f-48bd-89e8-2d4611404fa1

# Get a namespaced resource by name (returns the single resource)
zoa run get_db_resource --reason ROSAENG-1234 --param gvk=hyperfleet.io/v1alpha1/Cluster -n cluster-38ae9f84-aa1f-48bd-89e8-2d4611404fa1 --name jaime1

# Get the cluster-scoped ManagementCluster by name (no namespace)
zoa run get_db_resource --reason ROSAENG-1234 --param gvk=hyperfleet.io/v1alpha1/ManagementCluster --name mc-01

# Full objects (spec/status/metadata) instead of the summary table
zoa run get_db_resource --reason ROSAENG-1234 --param gvk=hyperfleet.io/v1alpha1/Cluster -A -v

# Pipe full objects into jq (needs -v for spec/status to exist; -o json wraps output, so index .output[].spec)
zoa run get_db_resource --reason ROSAENG-1234 --param gvk=hyperfleet.io/v1alpha1/Cluster -A -v -o json | jq '.output[].spec'

# Same, without -o json (needs -v for spec/status to exist; output is the bare array, so index .[].spec)
zoa run get_db_resource --reason ROSAENG-1234 --param gvk=hyperfleet.io/v1alpha1/Cluster -A -v | jq '.[].spec'
```

**Notes:**

- Run `list_db_gvks` first. A Kind that is not present in the DB returns an empty result rather than an error, so it doubles as a way to confirm the exact GVK string.
- With no `--name`, the action lists; with `--name`, it returns the single matching resource or a not-found error (mirrors `kubectl get <kind> <name>`).
- Omitting `-n`/`-A` lists across all namespaces. `ManagementCluster` is the only cluster-scoped kind (stored with an empty namespace); the rest live in a per-hosted-cluster `cluster-<uuid>` namespace.
- Fully-deleted objects are always excluded; objects still holding finalizers (mid-deletion) are shown as `Terminating`, matching the operator's own view.

## Command flags reference

Per-command flags and filters (global flags are under [Configuration](#global-flags)).

### `actions` / `describe`

| Flag            | Description                                                                                     |
| --------------- | ----------------------------------------------------------------------------------------------- |
| `--offline`     | Use the action registry embedded in this CLI build (no API call)                                |
| `--target-type` | Target type: `rc` or `mc` (env: `ZOA_TARGET_TYPE`; required with `--offline` unless env is set) |

**Output formats** for `actions`: `table` (default), `json`, `markdown` (offline only — full catalog for Claude Code / docs).

**Alias:** `zoa catalog` is an alias for `zoa actions`.

### `session start`

| Flag           | Short | Description                                                                                                                  |
| -------------- | ----- | ---------------------------------------------------------------------------------------------------------------------------- |
| `--reason`     |       | Reason for this session (required unless `ZOA_REASON` is set on laptop; normally required on laptop; **flag wins over env**) |
| `--deployment` | `-d`  | Deployment name (alternative to positional arg)                                                                              |
| `--target`     | `-t`  | Target ID (alternative to positional arg)                                                                                    |
| `--no-connect` |       | Provision boundary task and print session metadata; do not open ECS Exec                                                     |

### `session list`

| Flag       | Short | Description                                                                            |
| ---------- | ----- | -------------------------------------------------------------------------------------- |
| `--status` |       | Filter by status (`creating`, `active`, `terminated`, `failed`, `all`; default: `all`) |
| `--target` | `-t`  | Filter by target cluster                                                               |
| `--since`  |       | Start of time window (default: `24h`). See [Time Formats](#time-formats).              |
| `--until`  |       | End of time window (default: now). See [Time Formats](#time-formats).                  |
| `--limit`  |       | Max results (max 200, default 50)                                                      |

### `session history`

| Flag         | Short | Description                                                               |
| ------------ | ----- | ------------------------------------------------------------------------- |
| `--operator` |       | Filter by operator                                                        |
| `--target`   | `-t`  | Filter by target cluster                                                  |
| `--status`   |       | Filter by status                                                          |
| `--since`    |       | Start of time window (default: `24h`). See [Time Formats](#time-formats). |
| `--until`    |       | End of time window (default: now). See [Time Formats](#time-formats).     |
| `--limit`    |       | Max results (max 200, default 50)                                         |

### `zoa run` — TA parameters

Trusted Actions declare **parameters** in code (see `zoa describe <action>`). On `zoa run`, you pass them in two ways:

### Shared parameter flags (kubectl-style)

Common kube-api parameters have **dedicated flags** with short forms — same muscle memory as `kubectl`:

| API param        | CLI                                      |
| ---------------- | ---------------------------------------- |
| `namespace`      | `-n` / `--namespace`                     |
| `all_namespaces` | `-A` / `--all-namespaces` (sends `true`) |
| `label_selector` | `-l` / `--selector`                      |
| `verbose`        | `-v` / `--verbose` (sends `true`)        |
| `name`           | `--name`                                 |
| `resource`       | `--resource`                             |
| `gather`         | `--gather` (`must_gather`)               |
| `cluster_id`     | `--cluster-id` (`must_gather` + `hcp`)   |

Use these when the TA accepts the matching parameter — they are equivalent to `--param <api_key>=<value>`.

### Generic `--param` (repeatable)

Any TA parameter can be set with **`--param key=value`**. Repeat the flag for multiple keys:

```bash
zoa run get_db_resource --reason ROSAENG-1234 \
  --param gvk=hyperfleet.io/v1alpha1/Cluster -n cluster-38ae9f84-aa1f-48bd-89e8-2d4611404fa1
```

Parameters that have **no** dedicated flag (for example `gvk` on `get_db_resource`, or `extra_namespaces` on `must_gather`) must use `--param`.

**Precedence:** if the same API key is set by both a shared flag and `--param`, the **shared flag wins** (`-n` beats `--param namespace=…`).

**Discovery:** `zoa describe <action>` lists every parameter, which ones have a shared flag, and the API key name to use in `--param`.

### `run`

| Flag                   | Short | Description                                                                                                                                            |
| ---------------------- | ----- | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `--reason`             |       | Reason for this run (required unless `ZOA_REASON` is set; **flag wins over env** — see [Environment variables](#environment-variables-and-precedence)) |
| `--namespace`          | `-n`  | Target namespace                                                                                                                                       |
| `--all-namespaces`     | `-A`  | All namespaces                                                                                                                                         |
| `--name`               |       | Resource name                                                                                                                                          |
| `--resource`           |       | Resource type (for generic actions)                                                                                                                    |
| `--selector`           | `-l`  | Label selector                                                                                                                                         |
| `--verbose`            | `-v`  | Full JSON output (no compact summary)                                                                                                                  |
| `--param`              |       | Any TA parameter as `key=value` (repeatable); see [TA parameters](#zoa-run--ta-parameters)                                                             |
| `--force`              |       | Bypass write cooldown and concurrency limits                                                                                                           |
| `--dry-run`            |       | Execute the dry-run variant of the action                                                                                                              |
| `--execution-mode`     |       | Override execution class: `sync` or `async` (default: TA's declared class)                                                                             |
| `--timeout`            |       | Server-side TA execution timeout (e.g. 60s, 3m; bounded by server max 295s)                                                                            |
| `--no-wait`            |       | Return ID immediately, skip output display                                                                                                             |
| `--wait`               |       | Poll until async execution completes (no effect on sync — sync returns inline)                                                                         |
| `--wait-timeout`       |       | Max poll duration when `--wait` is active (default 5m)                                                                                                 |
| `--wait-poll-interval` |       | Poll frequency when `--wait` is active (default 30s)                                                                                                   |
| `--gather`             |       | `must_gather` scopes: `hcp`, `mc`, `rc` (comma-separated; must match the ZOA endpoint)                                                                 |
| `--cluster-id`         |       | Hosted cluster UUID (`must_gather` when `--gather` includes `hcp`)                                                                                     |

#### `must_gather`

**Must-gather** is the standard troubleshooting bundle: Kubernetes logs, events, and resource manifests packaged as `output.tar.gz` for offline analysis (compatible with `omc`/`omg`-style layouts).

| `--gather` | Collects                                                                                                                  |
| ---------- | ------------------------------------------------------------------------------------------------------------------------- |
| `hcp`      | One hosted ROSA cluster: hypershift dump + HCP/control-plane namespace diagnostics (MC ZOA only; requires `--cluster-id`) |
| `mc`       | Management cluster platform: platform namespaces (e.g. kube-applier, hypershift), nodes, storage, Karpenter CRs           |
| `rc`       | Regional cluster platform: platform namespaces (e.g. platform-api), Seaplane CRs, nodes, storage, Karpenter CRs         |

Read-only. Produces `output.tar.gz` (`zoa download`). Mode is async (see MODE column); use `--wait` to block.

```bash
# MC platform
zoa run must_gather --reason ROSAENG-1234 --gather mc --wait

# RC platform
zoa run must_gather --reason ROSAENG-1234 --gather rc --wait

# Hosted cluster (MC ZOA only)
zoa run must_gather --reason ROSAENG-1234 --gather hcp --cluster-id <uuid> --wait

zoa describe must_gather   # full parameter reference
zoa download <exec-id> -f /tmp/must-gather.tar.gz
```

Use `zoa describe must_gather` for parameters such as `extra_namespaces` and `skip_must_gather_image` (pass via `--param` unless a shared flag exists). For kube reads, prefer `-n`, `-A`, `--resource`, `--name`, `-l`, `-v` where applicable.

### `runs`

All filters are combinable:

| Flag               | Short | Description                                                                        |
| ------------------ | ----- | ---------------------------------------------------------------------------------- |
| `--status`         |       | Filter by status (dispatched, succeeded, failed, timed_out)                        |
| `--action`         |       | Filter by action name                                                              |
| `--operator`       |       | Filter by operator                                                                 |
| `--target`         | `-t`  | Filter by target cluster                                                           |
| `--scope`          |       | Filter by scope (kube-api, aws-api)                                                |
| `--type`           |       | Filter by type (read, write)                                                       |
| `--execution-mode` |       | Filter by execution class (sync, async)                                            |
| `--since`          |       | Show entries after this point (default: `24h`). See [Time Formats](#time-formats). |
| `--until`          |       | Show entries before this point. See [Time Formats](#time-formats).                 |
| `--dry-run`        |       | Show only dry-run executions                                                       |
| `--limit`          |       | Max results (max 100, default 20)                                                  |

### `get`

| Flag                   | Description                                                        |
| ---------------------- | ------------------------------------------------------------------ |
| `--include-output`     | Include execution output in display                                |
| `--include-logs`       | Include execution logs                                             |
| `--include-all`        | Include both output and logs                                       |
| `--wait`               | Poll until execution reaches terminal status (useful to reconnect) |
| `--wait-timeout`       | Max poll duration when `--wait` is active (default 5m)             |
| `--wait-poll-interval` | Poll frequency when `--wait` is active (default 30s)               |

### `audit`

| Flag         | Short | Description                                                                        |
| ------------ | ----- | ---------------------------------------------------------------------------------- |
| `--action`   |       | Filter by action name                                                              |
| `--operator` |       | Filter by operator                                                                 |
| `--target`   | `-t`  | Filter by target cluster                                                           |
| `--method`   |       | Filter by HTTP method (GET, POST)                                                  |
| `--approval` |       | Filter by approval state                                                           |
| `--since`    |       | Show entries after this point (default: `24h`). See [Time Formats](#time-formats). |
| `--until`    |       | Show entries before this point. See [Time Formats](#time-formats).                 |
| `--limit`    |       | Max results (max 200, default 50)                                                  |

**ACTION column:** Trusted Action runs use the **TA name** (`get_db_resource`, …). Session lifecycle uses **`session_*`** (`session_start`, `session_join`, `session_history`, …). Boundary **meta** API reads use reserved names: `runs_list` (`zoa runs`), `run_get` (`zoa get`), `run_output` / `run_logs` (artifacts), `audit_query` (`zoa audit`). Catalog reads (`zoa actions`, `zoa describe`, laptop `zoa targets`, `zoa session list` with `scope=mine`) are **not** audited (no row).

Default table **PATH** omits `/api/v0/` and shows the route family (`trusted-actions/...`, `sessions/...`, `audit`); `-o json` includes the full stored path. See [API reference — audit action values](api-reference.md#audit-action-values).

### `download`

| Flag         | Short | Description                                                 |
| ------------ | ----- | ----------------------------------------------------------- |
| `--file`     | `-f`  | Destination file path (default: `zoa-<id>-<artifact>.json`) |
| `--artifact` |       | Which artifact to download: `output` (default), `logs`      |

## Time formats

Both `--since` and `--until` accept the same flexible formats:

| Format              | Example                          | Meaning                                              |
| ------------------- | -------------------------------- | ---------------------------------------------------- |
| Duration            | `1h`, `24h`, `7d`, `30m`, `300s` | Relative to now (subtracted)                         |
| Short date          | `2026-08-25`                     | Start-of-day for `--since`, end-of-day for `--until` |
| RFC3339             | `2026-08-25T14:30:00Z`           | Exact timestamp                                      |
| RFC3339 with offset | `2026-08-25T16:30:00+02:00`      | Exact timestamp (converted to UTC)                   |

**Behavior notes:**

- `--since` defaults to `24h` if not specified (shows last 24 hours)
- `--until` is optional; when omitted, results go up to the present
- Short dates are interpreted inclusively: `--since 2026-08-25` means "from the start of Aug 25" and `--until 2026-08-25` means "through the end of Aug 25"
- Results are always sorted newest-first

**Examples:**

```bash
# Last 24 hours (default)
zoa runs

# Last 7 days
zoa runs --since 7d

# Specific day range (inclusive)
zoa runs --since 2026-08-20 --until 2026-08-25

# Since a specific timestamp
zoa runs --since 2026-08-25T09:00:00Z

# Time window: between 2 hours ago and 30 minutes ago
zoa runs --since 2h --until 30m

# Same filters work on audit
zoa audit --since 7d --target my-cluster
zoa audit --since 2026-08-01 --until 2026-08-15
```

## Shell completion

```bash
source <(zoa completion zsh)     # zsh
source <(zoa completion bash)    # bash
zoa completion fish | source     # fish
```
