# ZOA Boundary — Container image

Image: **`zoa-boundary`** (`Containerfile.boundary`). Konflux component **`rosa-seaplane-zoa-boundary`** publishes to `quay.io/redhat-user-workloads/rosa-tenant/rosa-seaplane-zoa-boundary` (pipelines planned; see [Konflux](../konflux.md)). Local dev and ephemeral CI use `quay.io/rrp-dev-ci/zoa-boundary`. Tag pinned in rosa-seaplane `config/defaults.yaml` (`zoa_boundary_image_tag` + `zoa_boundary_source_image`).

## Design

- **Multi-stage build** — compilers and download tools stay in builder stages; **final image has no curl/wget**.
- **Checksum-verified downloads** — `kubectl` (`.sha256` sidecar), Claude Code (`SHASUMS256.txt`), AWS CLI v2 (version pin + post-install version match — AWS does not publish per-version SHA256 sidecars).
- **Architecture-aware binaries** — CI builds `linux/amd64` and `linux/arm64`; the AWS CLI download maps these to its `x86_64` and `aarch64` archive names, respectively.
- **Non-root runtime** — user `sre` (uid 1000); ECS task definition sets `user: 1000`.
- **ECS Exec** — `script` + `cat` from `util-linux` / coreutils; entrypoint fails closed if missing.

## Shipped tools

| Tool         | Purpose                                                                                  |
| ------------ | ---------------------------------------------------------------------------------------- |
| `zoa`        | TAs, actions, audit (API Lambda)                                                         |
| `kubectl`    | Break-glass after `zoa breakglass activate`; no kubeconfig by default                    |
| `aws` CLI v2 | SigV4 to Function URLs; break-glass uses scoped cred file after activate (not task role)   |
| `jq`         | JSON from `zoa -o json`                                                                  |
| `claude`     | Bedrock (`CLAUDE_CODE_USE_BEDROCK=1`); operational assist only (not customer data store) |

### Claude Code in a boundary task

- **Each plain `claude` starts a new conversation.** To continue a prior one in the **same ECS task**, use **`claude --resume`** (no arguments) — Claude Code opens an **interactive session list** (search, title/summary, last activity). That is the path most people want; you do **not** need the UUID from the exit banner.
- **Shortcuts:** **`claude --continue`** reopens the latest conversation for the current directory (`/home/sre`). Optional: **`claude --resume <session-id>`** if you copied the ID Claude printed on exit.
- **History is local to the ECS task** under `/home/sre/.claude/` (project transcripts, settings). It **persists while the boundary task runs** (same Exec reconnect or a new `claude` in the same shell). It is **not** synced to S3 or your laptop; when the task is reaped or terminated, that history is gone.
- **Leaving Claude without `zoa session terminate`** only ends the Claude process — the **ZOA boundary session / ECS task keeps running** (by design). Run **`claude --resume`** again to pick the prior **Claude Code session** (not the same thing as the ZOA session id on your shell prompt).
- **Bedrock mode** (`CLAUDE_CODE_USE_BEDROCK=1`): no Anthropic cloud account memory. Claude may still use **on-disk project history** for `--resume`; it will **not** automatically recall a previous chat if you run plain `claude` again.
- **Durable context for agents**: bake guidance in `/home/sre/.claude/CLAUDE.md`, `ZOA_SESSION.md`, and `ZOA_ACTIONS.md` (refreshed at task start). For incident notes you need after teardown, copy to a ticket or use **`zoa run`** output / audit — not Claude chat alone.

## Shell

- `/etc/bash_completion.d/` — `zoa`, `kubectl`, `aws`
- `~/.bashrc.d/09-zoa-session-id.bashrc` — exports `ZOA_SESSION_ID` from task env
- `~/.bashrc.d/10-zoa-prompt.bashrc` — two-line session prompt (`sessionId:` + `operator@zoa:`)
- `~/.bashrc.d/15-zoa-motd.bashrc` — session info banner on login
- `~/.bashrc.d/16-zoa-session-login.bashrc` — session context setup
- `~/.bashrc.d/99-session-exit-reminder.bashrc` — on `exit`/Ctrl+D, prints `zoa session terminate` hints for the laptop (task keeps running)
- Entrypoint: `zoa-boundary-entrypoint.sh` — writes `ZOA_SESSION.md`, `ZOA_ACTIONS.md`, banner

## Build

```bash
make image-boundary
# or with tag
IMAGE_TAG=$(git rev-parse --short HEAD) make image-push-boundary
```

Pass `VCS_REF` / `VERSION` via build-args when integrating with Konflux (see `docs/konflux.md`).

## Tests

```bash
make test-shell   # bats tests/shell/ (requires bats-core; builds bin/zoa first)
```

Wire into CI via openshift/release (planned).

## Related

- [Session logging](session-logging.md)
- [zoa-lambda module README](https://github.com/openshift-online/rosa-seaplane/blob/main/terraform/modules/zoa-lambda/README.md) (boundary ECS: `boundary-ecs.tf`, `boundary-task.tf`)
