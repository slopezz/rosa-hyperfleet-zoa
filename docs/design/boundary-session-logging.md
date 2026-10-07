# ZOA Boundary — ECS Exec and CloudWatch logging

## Two log groups (one KMS key)

ZOA boundary uses **two CloudWatch log groups** encrypted with the **same CMK** (shared ZOA key on RC; per-cluster key on MC when no `kms_key_arn`).

| Purpose               | Log group                                     | Stream naming                          | What it captures                                                                    |
| --------------------- | --------------------------------------------- | -------------------------------------- | ----------------------------------------------------------------------------------- |
| **Container stdout**  | `/ecs/<cluster_id>/zoa-boundary`              | `container/zoa-boundary/<task-id>`     | Task startup script only (banner, tool check, “ready for connections”).             |
| **ECS Exec sessions** | `/ecs/<cluster_id>/zoa-boundary/ssm-sessions` | `ecs-execute-command-<ssm-session-id>` | Interactive shell I/O after `zoa session join` — bash, `zoa run`, Claude Code, etc. |

`<cluster_id>` is the HyperFleet cluster id (`regional_id` on RC, `management_id` on MC).

**Do not** search session content in the container group. **Do not** use the exec group for “did the task start?” — use container logs.

Terraform: `terraform/modules/zoa-lambda/` in [rosa-hyperfleet](https://github.com/openshift-online/rosa-hyperfleet) (boundary ECS cluster, task definition, log groups).

FedRAMP AU-09: exec group + KMS + `cloudWatchEncryptionEnabled = true` on cluster `executeCommandConfiguration`.

## AWS requirements

From [Monitor Amazon ECS Exec commands using CloudWatch Logs](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/ecs-exec-logging.html):

1. **Cluster** — `logging = OVERRIDE`, exec log group = `.../ssm-sessions`, same CMK as both log groups.
2. **Task role** — `logs:DescribeLogGroups`; `CreateLogStream` / `DescribeLogStreams` / `PutLogEvents` on **exec log group only**; KMS `GenerateDataKey` / `Decrypt` on the shared CMK.
3. **Caller (ECS Exec)** — KMS for encrypted exec channel (`*-zoa-access-exec-scoped` role vended on session join).
4. **RunTask** — `EnableExecuteCommand: true` (`pkg/awsecs/client.go`).
5. **Image** — **`script` and `cat`** (`util-linux` in `Containerfile.boundary`); startup fails if missing.

**Interactive shell user:** ECS Exec always attaches as **root**; Terraform sets `ecs_exec_interactive_command` (Access Lambda env `ZOA_ECS_EXEC_COMMAND`, default `runuser -u sre -- /bin/bash -l`). ZOA Access returns it as `exec_command` on `POST /sessions/join/{id}`; the CLI passes it through to `ExecuteCommand` with no separate business logic.

**Exec credentials:** On join, Access assumes `${regional_id}-zoa-access-exec-scoped` with a session policy limiting `ecs:ExecuteCommand` to that session's task ARN and returns `exec_credentials` to the CLI. The laptop does not assume `OrganizationAccountAccessRole` for exec.

**Skel files:** `/home/sre/.claude/CLAUDE.md`, baked `.claude/settings.json`, and runtime `.claude/ZOA_SESSION.md` — use `ls -la /home/sre` (plain `ls` hides dotfiles).

**Historical root cause (eph, Oct 2026):** Missing `script` in the image; exec transcripts never shipped. Secondary confusion: only reading `container/...` streams in the container log group.

## What ECS Exec PTY logging is

ZOA boundary does **not** implement a separate “command-only” audit daemon inside the container. Session forensics use **AWS ECS Exec logging**: the SSM/ECS Exec channel records what flows through the **interactive terminal (pseudo-TTY)** while you are connected with `zoa session join` (or the default connect after `zoa session start`).

The boundary image runs **`script`** (required at startup) so Exec can ship that terminal session to CloudWatch. Think of it as a **session recording** of the shell, not a filtered list of argv.

### What is included in the transcript

| Included               | Examples                                                                                              |
| ---------------------- | ----------------------------------------------------------------------------------------------------- |
| **Keystrokes / input** | Commands you type, Claude prompts, answers at approval prompts                                        |
| **Program output**     | `zoa run` stdout/stderr in the shell, `kubectl` output (break-glass, when enabled), errors, MOTD text |
| **TUI redraws**        | Claude Code screen updates (ANSI sequences — looks busy in Grafana but content is present)            |
| **Session bookends**   | `Script started on …`, `Script done on …`, ZOA exit hints after `exit`                                |

So for forensics: **everything the SRE (or Claude driving the same shell) sees and types in that Exec session** is intended to be captured in `@message` for the stream `ecs-execute-command-<ssm-session-id>`.

### What is not the exec transcript

| Log group                                           | Captures                                                                                      |
| --------------------------------------------------- | --------------------------------------------------------------------------------------------- |
| `/ecs/<cluster_id>/zoa-boundary` (container stdout) | **Task startup only** — entrypoint banner, “ready for connections”, not the interactive shell |
| **DynamoDB** (`zoa runs`, `zoa audit`)              | **Trusted Actions** and API audit — structured params/output, not the full terminal           |
| Processes with **no TTY**                           | Background jobs not attached to the interactive shell are outside the PTY recording           |

Use **exec PTY** for “what happened in the session”; use **`zoa runs` / `zoa get`** for authoritative TA payloads; use both together for investigations.

### Claude Code and readability

Claude Code uses a full-screen TUI; the PTY capture can look noisy in Grafana (escape sequences, redraws). The recording is still **valid for forensics** — user questions, `Bash(zoa run …)` lines, ✓ execution summaries, and logout hints remain in the text. Search inside `@message` or export the stream via the AWS CLI.

Optional: [Bedrock model invocation logging](https://docs.aws.amazon.com/bedrock/latest/userguide/model-invocation-logging.html) for model-call metadata — separate from ECS exec logs.

**Ingest timing:** Events often appear **after Exec disconnect**, with **~1–2 minutes** lag. Set Grafana time range accordingly.

## Log group naming

Exec transcripts for a **target cluster** (RC or MC):

```
/ecs/<cluster_id>/zoa-boundary/ssm-sessions
```

Example (ephemeral regional target `eph-046f5f15-regional`):

```
/ecs/eph-046f5f15-regional/zoa-boundary/ssm-sessions
```

Each **SSM / ECS Exec attach** gets a log stream:

```
ecs-execute-command-<ssm-session-id>
```

The laptop CLI records this on the session row after join (`exec_session_ids` in `zoa session history`).

## Viewing session logs in Grafana Explorer

Same pattern as [ZOA Lambda logs](../observability.md#lambda-logs): use **CloudWatch Logs** in Explore, not CloudWatch Metrics.

1. Open **Grafana → Explore**.
2. Select the **CloudWatch Logs** datasource for the **account that hosts the boundary task**:
   - **CloudWatch Logs (Regional)** — RC boundary sessions (target type `rc`, cluster id = regional id).
   - **CloudWatch Logs (`<mc-id>`)** — MC boundary sessions (target type `mc`, e.g. `eph-046f5f15-mc01`).
3. Switch the query editor to **CloudWatch Logs** (toggle at the top; default is often **CloudWatch Metrics**).
4. **Select log group(s)** — search for `zoa-boundary/ssm-sessions`, e.g.  
   `/ecs/eph-046f5f15-regional/zoa-boundary/ssm-sessions`.
5. Resolve the **log stream** for the session you care about (see below).
6. Run a **Logs Insights** query on that group (paste as a **single** query block; avoid blank lines between pipe stages):

```text
fields @timestamp, @message
| filter @logStream = "ecs-execute-command-v2hqi98n2456t7b99oubhyfn9a"
| sort @timestamp asc
| limit 10000
```

Replace `@logStream` with the stream from step 5.

### Resolve `@logStream` from `zoa session history`

From your laptop (Central / invoker credentials):

```bash
zoa session history us-east-1-eph-046f5f15 -o json
```

Pick the session row (operator, `target_cluster`, `created_at`). **`exec_session_ids`** holds the CloudWatch stream name(s) (prefix `ecs-execute-command-`):

```bash
zoa session history us-east-1-eph-046f5f15 -o json \
  | jq -r '.items[] | select(.operator == "slopezma") | .exec_session_ids[]?'
```

Use the newest stream if multiple joins occurred on the same boundary session.

Correlate without JSON via the transcript itself: lines `Script started`, `Operator:`, `Session:`, and `sessionId:` in `@message`.

### AWS CLI (same log group and stream)

```bash
aws logs get-log-events \
  --profile <deployment-account-profile> \
  --region us-east-1 \
  --log-group-name "/ecs/eph-046f5f15-regional/zoa-boundary/ssm-sessions" \
  --log-stream-name "ecs-execute-command-v2hqi98n2456t7b99oubhyfn9a" \
  --start-from-head
```

## Task stop behavior (`stopTimeout`)

Fargate `stopTimeout` is **30 seconds** (same scale as the default Kubernetes `terminationGracePeriodSeconds` and the Fargate platform default). ZOA boundary has no persistent home volume or shutdown sync — only a grace period for SIGTERM and CloudWatch agent flush after the session ends.

## Verification after deploy

1. Rebuild boundary image; bump image tag; `terraform apply` (exec log group `.../ssm-sessions`, task definition, IAM).
2. New session → join → `echo ZOA_EXEC_LOG_TEST-$(date -u +%s)` → exit session → wait ~1–2 min.
3. **Exec (expect hit here):**

```bash
aws logs tail "/ecs/<cluster_id>/zoa-boundary/ssm-sessions" --follow

aws logs describe-log-streams \
  --log-group-name "/ecs/<cluster_id>/zoa-boundary/ssm-sessions" \
  --order-by LastEventTime --descending --max-items 10
```

4. **Container (startup only):**

```bash
aws logs tail "/ecs/<cluster_id>/zoa-boundary" --since 1h
# streams: container/zoa-boundary/<task-id>
```

5. Optional: [amazon-ecs-exec-checker](https://github.com/aws-containers/amazon-ecs-exec-checker).

## ZOA TA audit (complement)

`zoa run` is recorded in **ZOA DynamoDB audit** regardless of CloudWatch exec logging. For a full picture: **exec PTY transcript** (what happened in the shell) + **`zoa runs` / `zoa get`** (structured TA params and output).

## DynamoDB `execSessionIds` (forensics)

After each successful `ExecuteCommand`, the laptop CLI calls Access **`POST /api/v0/sessions/exec-attached/{session-id}`** to append the SSM session id (CloudWatch stream name). This is **optional for reaper idle logic** — the Worker correlates activity via SSM and CloudWatch directly. See [session reaper](boundary-session-reaper.md).
