# ZOA Boundary container image

Image: **`zoa-boundary`** (`Containerfile.boundary`). Published to Quay (`quay.io/rrp-dev-ci/zoa-boundary`); tag pinned in hyperfleet `config/defaults.yaml` (`zoa_boundary_image_tag`).

## Design

- **Multi-stage build** — compilers and download tools stay in builder stages; **final image has no curl/wget**.
- **Checksum-verified downloads** — `kubectl` (`.sha256`), Claude Code (`SHASUMS256.txt`).
- **Architecture-aware binaries** — `kubectl` and AWS CLI v2 use `TARGETARCH` (today CI builds `linux/amd64` only; arm64-ready for a future multi-arch PR).
- **Non-root runtime** — user `sre` (uid 1000); ECS task definition sets `user: 1000`.
- **ECS Exec** — `script` + `cat` from `util-linux` / coreutils; entrypoint fails closed if missing.

## Shipped tools

| Tool                                     | Purpose                                                                                  |
| ---------------------------------------- | ---------------------------------------------------------------------------------------- |
| `zoa`                                    | TAs, actions, audit (API Lambda)                                                         |
| `kubectl`                                | Future break-glass; no kubeconfig by default                                             |
| `aws` CLI v2                             | SigV4 to Function URLs; future AWS break-glass                                           |
| `jq`                                     | JSON from `zoa -o json`                                                                  |
| `claude`                                 | Bedrock (`CLAUDE_CODE_USE_BEDROCK=1`); operational assist only (not customer data store) |
| `vim-minimal`, `bind-utils`, `procps-ng` | Light ops                                                                                |

**Not included:** `oc`, `ocm`, backplane, tmux, EFS home sync — intentional; boundary is a short-lived jump box, not a persistent SRE workstation.

## Shell

- `/etc/bash_completion.d/` — `zoa`, `kubectl`, `aws`
- `~/.bashrc.d/15-zoa-motd.bashrc` — UTF-8 **ZOA** banner + Hello + session context on Exec login (once per shell)
- `~/.bashrc.d/10-zoa-prompt.bashrc` — two-line session prompt
- `~/.bashrc.d/99-session-exit-reminder.bashrc` — on `exit`/Ctrl+D, `==>` hints to stop from laptop (task keeps running)
- `/etc/zoa-boundary/banner.sh` — shared MOTD, exit hint, container startup log (no ANSI colors)
- Entrypoint: `zoa-boundary-entrypoint.sh` — `ZOA_SESSION.md`, `ZOA_ACTIONS.md`, startup banner

## Build

```bash
make image-boundary
# or with tag
IMAGE_TAG=$(git rev-parse --short HEAD) make image-push-boundary
```

Pass `VCS_REF` / `VERSION` via build-args when integrating with Konflux (see `docs/konflux.md`).

## Tests

```bash
make test-shell   # bats — entrypoint.sh (requires bats-core)
```

Wire into CI via openshift/release (planned).

## Related

- [Session logging](../design/boundary-session-logging.md)
- [Terraform module README](https://github.com/openshift-online/rosa-hyperfleet/blob/main/terraform/modules/zoa-boundary/README.md)
