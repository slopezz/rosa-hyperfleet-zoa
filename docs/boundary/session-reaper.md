# ZOA Boundary — Session reaper

The **Worker Lambda** in each target VPC runs a **boundary session reaper** on the same EventBridge schedule as the TA reconciler/GC (`route: reaper`). It stops orphaned **ECS Fargate** boundary tasks and marks DynamoDB session rows **`terminated`** when policy says the session is done.

Implementation: `pkg/scheduler/reaper.go`, `pkg/boundaryexec/activity.go`, `pkg/store/session.go`, wired from `cmd/zoa-lambda/main.go` when `SESSIONS_TABLE` is set.

## Ways a session ends

| Path                        | Who stops the task?                                                           | `stop_reason` in DynamoDB |
| --------------------------- | ----------------------------------------------------------------------------- | ------------------------- |
| **Operator terminate**      | SRE on laptop: `zoa session terminate <deployment>/<session-id>` (Access API) | `operatorStop`            |
| **Max duration (deadline)** | Worker reaper (`creating` or `active` past `deadline`)                      | `deadlineReaperStop`      |
| **Stale `creating`**        | Worker reaper (row stuck in `creating` longer than timeout)                   | `staleCreatingReaperStop` |
| **Idle (no Exec activity)** | Worker reaper (`active` only, requires `taskId`)                              | `idleReaperStop`          |

Access may set **`failed`** / **`provisionFailed`** when `RunTask` fails on `session start` (`pkg/api/access.go`).

**Leaving the Exec shell** (`exit`, Ctrl+D) does **not** stop the task. Only **terminate**, **deadline reaper**, or **idle reaper** ends the ECS task.

## Two reaper policies (Worker only)

Both run in one `Reaper.Run()` pass: **deadline**, then **stale creating**, then **idle**. Each Worker scopes to **`TARGET_CLUSTER`** for its VPC.

| Policy            | Question                                             | Data source                                     | `stop_reason`               |
| ----------------- | ---------------------------------------------------- | ----------------------------------------------- | --------------------------- |
| **Deadline**      | Has `now` passed session **`deadline`**?             | GSI **`target-status-index`** (`ListExpired`: `creating` + `active`) | `deadlineReaperStop`        |
| **Stale creating**| Row still **`creating`** after provision timeout?    | `ListOpenByTargetStatus(creating)` + `createdAt` | `staleCreatingReaperStop` |
| **Idle**          | Has ECS Exec terminal activity been absent too long? | `ListActiveBeforeDeadline` + SSM + CloudWatch Logs | `idleReaperStop`            |

### Deadline

- Set on **session start** (Access Lambda): default from **`SESSION_MAX_DURATION_HOURS`** (default **4**). Optional `timeout_hours` on start is capped by the same value.
- Stored as RFC3339 **`deadline`** on the session row and in **`targetStatusKey`** as `active#<deadline>` while the session is active.
- Reaper queries the Worker’s **`TARGET_CLUSTER`** partition on **`target-status-index`** (not a fleet-wide status scan).
- Reaper reads the stored deadline via the GSI key (not env at stop time).

### Idle

- Configured on the **Worker** via **`SESSION_IDLE_TIMEOUT_SECONDS`** (default **3600**), rendered from rosa-seaplane `zoa_boundary_session_idle_timeout_seconds`.

### Stale `creating`

- **`SESSION_STALE_CREATING_SECONDS`** on the Worker (default **900**): if a session row is still **`creating`** and **`createdAt`** is older than this window, the reaper marks it **`terminated`** (`staleCreatingReaperStop`).
- If **`taskArn`** is set (partial provision), the reaper calls **`ecs:StopTask`** before updating DynamoDB.
- Rows with **no** task: DynamoDB cleanup only (no ECS stop).
- **Activity source of truth is AWS**, not DynamoDB:
  1. **SSM** `DescribeSessions` for `AmazonECS-ExecuteInteractiveCommand` targeting the ECS **task id** and `zoa-boundary`.
  2. **CloudWatch Logs** `DescribeLogStreams` on `/ecs/<target_cluster>/zoa-boundary/ssm-sessions` for streams `ecs-execute-command-<id>`.
  3. Latest `lastEventTimestamp` across those streams = last terminal activity.
- **If exec activity exists** and last activity is older than the idle window → terminate.
- **If no exec sessions exist** for the task → terminate when **`createdAt`** is older than the idle window (never joined, or join before logs exist).
- Sessions without **`taskId`** / **`targetCluster`** are skipped.

**`execSessionIds` in DynamoDB** is a **forensics hint** only. Idle enforcement does not require the CLI `exec-attached` call.

## Exec session registration (`POST /api/v0/sessions/exec-attached/{id}`)

After `ecs:ExecuteCommand` succeeds, the laptop CLI appends the SSM session id to **`execSessionIds`**. If the call fails, the CLI warns and still opens the shell (`internal/cli/session_exec.go`). See [session logging](session-logging.md).

## RunTask environment (catalog + Claude context)

Access injects at `RunTask` (in addition to task-definition defaults):

| Env                                              | Purpose                                        |
| ------------------------------------------------ | ---------------------------------------------- |
| `ZOA_TARGET_TYPE`                                | `rc` or `mc` — selects baked TA catalog file   |
| `ZOA_SESSION_ID` / `ZOA_OPERATOR` / `ZOA_REASON` | Shell and Claude context (not API audit truth) |
| `ZOA_API_URL`, `ZOA_TARGET`, `ZOA_DEPLOYMENT`    | API plane wiring                               |

Entrypoint copies `/usr/share/zoa/catalog/ZOA_ACTIONS.{rc|mc}.md` → `/home/sre/.claude/ZOA_ACTIONS.md`.

## Terraform / IAM (rosa-seaplane)

- Worker env: `SESSION_IDLE_TIMEOUT_SECONDS` when `sessions_table_name` is set (`terraform/modules/zoa-lambda`).
- Worker IAM: `ssm:DescribeSessions`, `logs:DescribeLogStreams` on `.../zoa-boundary/ssm-sessions`.
- Access env: `SESSION_MAX_DURATION_HOURS` (`terraform/modules/zoa-access`).

## Operational notes

- **Idle baseline `createdAt`** includes task provisioning time; slow `RunTask` reduces time before idle reap if the operator never joins.
- **SSM history pagination** is capped (20 pages); revisit at very high join churn.

See also [session logging](session-logging.md), [operator workflow](../guides/operator-workflow.md), [boundary architecture](architecture.md).
