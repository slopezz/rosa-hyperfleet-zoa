# ZOA Boundary session (Claude Code)

You are running inside a **ZOA Boundary** container: a time-boxed, audited ECS Fargate task in the target VPC (Regional Cluster or Management Cluster). This is **not** a standing cluster-admin or AWS-admin workflow — operational access goes through **ZOA Trusted Actions** (and future **break-glass**, when enabled).

**Read these files first (updated every task start):**

| File | Purpose |
|------|---------|
| `/home/sre/.claude/ZOA_SESSION.md` | Session ID, operator, deployment, target, API URL |
| `/home/sre/.claude/ZOA_ACTIONS.md` | **Baked TA catalog** for this deployment target (`rc` or `mc`); use `zoa describe` for live API details |
| This `CLAUDE.md` | Rules, Jira format, CLI cheat sheet |

The shell prompt is two lines: **`session:<id>`** (audit handle) and **`operator@zoa:deployment/target`** (where you are). Copy the session line when opening tickets or correlating CloudWatch Exec logs.

## Authentication and audit

- The SRE authenticated through the **ZOA Access** path (Central Account → invoker role → session start). Identity is bound to the boundary session in DynamoDB (`ZOA_OPERATOR` in the shell).
- **Interactive work in this shell** is captured in **CloudWatch Logs** via **ECS Exec** on **`/ecs/<target>/zoa-boundary/ssm-sessions`** (KMS-encrypted).
- **ZOA API activity** (TAs and other calls) is recorded in **ZOA audit** (DynamoDB). TA executions appear in **`zoa runs`** / **`zoa get`**.
- **Local files under `/home/sre` are ephemeral.** Download TA artifacts with **`zoa download`** while connected, or from a laptop using execution IDs (future: audited presigned URLs).

Do **not** store long-lived credentials, kubeconfig with static tokens, or customer secrets in this home directory.

## Jira ticket (required on every `zoa run`)

**`--jira` is mandatory** for `zoa run`. The CLI rejects runs without it.

- Format: project key + hyphen + number, e.g. **`ROSAENG-1234`**, **`HPSTRAT-62`**, **`OCM-12345`**.
- Use the **real ticket** for the incident or change request you are working — not placeholders like `TEST-1` unless your environment explicitly allows it.
- Example: `zoa run get_resource --resource pods -n openshift-ingress --jira ROSAENG-5678`

If you are unsure which action to run, read **`ZOA_ACTIONS.md`** or run **`zoa describe <action>`** before **`zoa run`**.

## ZOA CLI (inside the boundary)

`ZOA_API_URL` is already set for **this VPC**. The action catalog in **`ZOA_ACTIONS.md`** reflects **only what this API exposes** (RC vs MC may differ — e.g. some `aws-api` TAs only on MC).

You do **not** need `zoa deployments`, `zoa targets`, or `zoa session *` here — those are for starting/joining sessions from a laptop.

| Command | Purpose |
|---------|---------|
| **`zoa actions`** | List TAs for **this** environment (also in `ZOA_ACTIONS.md`). |
| **`zoa describe <action>`** | Param → CLI flag mapping, run modifiers (`--force`, `--dry-run`, …), examples; `-o json` for automation. |
| **`zoa run <action> … --jira TICKET`** | **Primary path.** Sync mode waits and prints output. **`--no-wait`** for async → then **`zoa get`** / **`output`** / **`download`**. |
| **`zoa runs`** | List TA executions (`--since`, `--until`, `-o json`). |
| **`zoa get <exec-id>`** | Status/metadata; **`--include-output`** for payload. |
| **`zoa output` / `zoa logs` / `zoa download`** | Re-fetch or save artifacts from S3. |
| **`zoa audit`** | ZOA API audit trail (broader than TAs alone). |
| **`zoa version`** | Client and API version. |

Use **`-o json`** and **`jq`** for scripting.

### Suggesting a TA during investigation

1. Read **`ZOA_ACTIONS.md`** (or `zoa actions`) for names and descriptions.
2. Match the problem to **scope** and **type**: e.g. need a Secret in a non-HCP namespace → **`get_secret`** (read); need pod list → **`get_resource`** with `--resource pods`.
3. Run **`zoa describe <action>`** to confirm parameters, then **`zoa run … --jira TICKET`**.
4. Do **not** use `kubectl`/`aws` for operations that have a TA unless break-glass is active.

## Other tools in the image

| Tool | Notes |
|------|--------|
| **`jq`** | Parse `zoa -o json` output. |
| **`claude`** | **Amazon Bedrock**; **`ANTHROPIC_MODEL`** set by Terraform (Sonnet 5). Assistant only — not a bypass for TAs. |
| **`aws`** / **`kubectl`** | Installed but **no default credentials**. Use after break-glass only. |

## What you must not do

- Do not treat **`kubectl`** / **`aws`** as the primary interface without break-glass.
- Do not exfiltrate customer data or paste secrets into Claude prompts.
- Do not install packages — UID **`sre` (1000)**, no root.

## Working style

1. Confirm context in **`ZOA_SESSION.md`** (session id, target).
2. Pick a TA from **`ZOA_ACTIONS.md`** → **`zoa describe`** → **`zoa run … --jira TICKET`**.
3. Use Claude to interpret output; execute changes only through **`zoa run`** (or approved break-glass later).
