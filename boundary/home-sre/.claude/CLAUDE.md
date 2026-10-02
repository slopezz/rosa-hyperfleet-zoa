# ZOA Boundary session (Claude Code)

You are running inside a **ZOA Boundary** container: a time-boxed, audited ECS Fargate task in the target VPC (Regional Cluster or Management Cluster). This is **not** a standing cluster-admin or AWS-admin workflow — operational access goes through **ZOA Trusted Actions** (and future **break-glass**, when enabled).

Session-specific facts (deployment, target cluster, region, API URL) are in **`ZOA_SESSION.md`** in this directory — read that file first. It is **regenerated on every task start** (the image ships a stub; the task entrypoint fills in values).

## Authentication and audit

- The SRE authenticated through the **ZOA Access** path (Central Account → invoker role → session start). Identity is bound to the boundary session in DynamoDB.
- **Interactive work in this shell** (commands you type, `zoa run` stdout/stderr, Claude Code terminal I/O) is captured in **CloudWatch Logs** via **ECS Exec** on **`/ecs/<target>/zoa-boundary/ssm-sessions`** (KMS-encrypted, FedRAMP AU-09). Task startup lines only go to **`/ecs/<target>/zoa-boundary`** — see `docs/design/boundary-session-logging.md` in the zoa repo.
- **ZOA API activity** (TAs and other calls) is recorded in **ZOA audit** (DynamoDB). TA executions also appear in **`zoa runs`** / **`zoa get`**.
- **Local files under `/home/sre` are ephemeral.** When the session ends and the ECS task is stopped, this filesystem is destroyed — nothing in the container home directory is retained. Download TA artifacts to your laptop if you need them after the session (`zoa download` while still connected, or from a workstation using execution IDs).

Do **not** store long-lived credentials, kubeconfig with static tokens, or customer secrets in this home directory.

## ZOA CLI (inside the boundary)

`ZOA_API_URL` is already set for this VPC. You do **not** need `zoa deployments`, `zoa targets`, or `zoa session *` here — those are for starting/joining sessions from a laptop.

| Command | Purpose |
|---------|---------|
| **`zoa run <action> … --jira TICKET`** | **Primary path.** Run an approved TA. **Sync** mode (default for most actions) waits and prints output on success. Use **`--no-wait`** for async; then use **`zoa get`** / **`output`** / **`download`**. |
| **`zoa actions`** / **`zoa describe <action>`** | List TAs and parameters. |
| **`zoa runs`** | List TA **executions** (filters: `--since`, `--until`, `-o json\|wide`). |
| **`zoa get <exec-id>`** | Execution status/metadata; **`--include-output`** / **`--include-all`** for payload. |
| **`zoa output <exec-id>`** | Show TA stdout again (after **`zoa run`**, or for a past execution). Sync runs usually already printed output; use this to re-fetch. |
| **`zoa logs <exec-id>`** | Raw execution logs from S3 when the TA stored them. |
| **`zoa download <exec-id>`** | Save **`output`** or **`logs`** to a file (**`-f`**) — e.g. async without **`--wait`**, large artifacts, or anything you need on disk before the task ends. |
| **`zoa audit`** | Audit trail of **ZOA API calls** (broader than TAs alone). Use **`zoa runs`** for TA execution history. |
| **`zoa version`** | Client and API version. |

Use **`--jira`** on mutating or tracked work. Use **`-o json`** and **`jq`** for scripting.

## Other tools in the image

| Tool | Notes |
|------|--------|
| **`jq`** | Yes — parse `zoa` JSON output. |
| **`claude`** | Yes — Bedrock **Haiku 4.5** only; investigation assistant, not a bypass for TAs. |
| **`aws`** / **`kubectl`** | Binaries are **installed**, but there is **no usable kubeconfig or AWS profile** for direct cluster/account admin by default. Credentials appear only after **break-glass** is approved for this session (`ZOA_BREAKGLASS_ROLE_ARN`), scoped to that grant (kube read/write or AWS read/write). Until then, use **`zoa run`** for operations exposed as TAs. |

## What you must not do

- **`aws`** and **`kubectl`** are on **`PATH`** but will fail or be useless without break-glass credentials — do not treat them as the primary interface.
- Do not exfiltrate customer data or paste secrets into Claude prompts.
- Do not install packages — the task runs as UID **`sre` (1000)** with no root and no package manager for that user.

## Architecture reminder

```
Laptop  →  ZOA Access Lambda  →  ECS RunTask (this container)
                ↓
Inside container:  zoa run  →  SigV4  →  per-VPC ZOA API Lambda  →  EKS / AWS APIs
                   claude   →  Bedrock (regional, Haiku only)
Break-glass (future): scoped sts:AssumeRole after approval — kube/AWS per grant only
```

## Working style

- Use **`zoa run`** with a real **`--jira`** for operational work that must be audited. **Sync** TAs (most reads and many writes) print results when **`zoa run`** finishes — you often do not need anything else.
- Use **`zoa runs`** to find execution IDs; use **`zoa output`** / **`zoa download`** when you need output again (e.g. **async** TAs you started with **`--no-wait`**, or any past run — sync or async — by ID).
- Use Claude to interpret output and plan next steps — then execute through **`zoa run`** when action is required.
