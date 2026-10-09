# ZOA Boundary — Session reaper and exec session tracking

The **Worker Lambda** in each target VPC runs a **boundary session reaper** on the same EventBridge schedule as the TA reconciler/GC. It stops orphaned **ECS Fargate** boundary tasks and marks DynamoDB session rows **terminated** when policy says the session is done.

Implementation: `pkg/scheduler/reaper.go`, `pkg/boundaryexec/activity.go`, `pkg/store/session.go`, wired from `cmd/zoa-lambda/main.go` when `SESSIONS_TABLE` is set.

## Two independent policies

| Policy                      | Question                                             | Enforced by                                  | Stop reason          |
| --------------------------- | ---------------------------------------------------- | -------------------------------------------- | -------------------- |
| **Deadline (max duration)** | Has the session lived longer than its `deadline`?    | `ListExpired` on GSI `status-deadline-index` | `deadlineReaperStop` |
| **Idle (no usage)**         | Has ECS Exec terminal activity been absent too long? | SSM + CloudWatch (+ optional Dynamo hints)   | `idleReaperStop`     |

Both run in one `Reaper.Run()` pass: **deadline first**, then **idle**. Each Worker instance scopes to **`TARGET_CLUSTER`** for its VPC so RC and MC workers do not cross-stop tasks.

### Deadline

- Set on **session start** (Access Lambda): default length from **`SESSION_MAX_DURATION_HOURS`** (default **4**, Access env). Optional JSON `timeout_hours` on start is capped by the same value.
- Stored on the session row as RFC3339 `deadline`.
- Reaper does not interpret env at stop time — only the stored deadline.

### Idle

- Configured on the **Worker** via **`SESSION_IDLE_TIMEOUT_SECONDS`** (default **3600**).
- **Activity source of truth is AWS**, not DynamoDB:
  1. **SSM** `DescribeSessions` (history) for `AmazonECS-ExecuteInteractiveCommand` sessions whose target contains the ECS **task id** and `zoa-boundary`.
  2. **CloudWatch Logs** `DescribeLogStreams` on `/ecs/<target_cluster>/zoa-boundary/ssm-sessions` for streams `ecs-execute-command-<id>` (merged from SSM ids and optional Dynamo hints).
  3. Latest `lastEventTimestamp` across those streams = last terminal activity.
- **If exec activity exists** and last activity is older than the idle window → terminate.
- **If no exec sessions exist at AWS** for the task (never joined, or CLI failed before logs exist) → terminate when **`createdAt`** is older than the idle window. This catches tasks started but never used without requiring `execSessionIds` in DynamoDB.
- Sessions without `taskId` / `targetCluster` are skipped (still provisioning or failed).

**Design choice:** `execSessionIds` in DynamoDB is a **forensics hint** (correlate streams quickly). Idle enforcement does **not** require the CLI to call `exec-attached`.

## Exec session registration (`POST /api/v0/sessions/exec-attached/{id}`)

After `ecs:ExecuteCommand` succeeds, the laptop CLI calls Access to append the SSM session id to **`execSessionIds`** (stream name form). Flow:

1. `POST /sessions/join/{id}` → vended exec credentials + `exec_command`
2. CLI `ExecuteCommand` → SSM session id
3. **`POST /sessions/exec-attached/{id}`** → Dynamo append (owner + active session only)
4. `session-manager-plugin` → interactive shell

If step 3 fails, the CLI **warns on stderr** and still opens the shell (`internal/cli/session_exec.go`, `internal/ecsexec/join.go`). Reaper idle logic still works via SSM/CW.

## RunTask environment (catalog + Claude context)

Access injects at `RunTask` (in addition to task-definition defaults):

| Env                                           | Purpose                                                                        |
| --------------------------------------------- | ------------------------------------------------------------------------------ |
| `ZOA_DEPLOYMENT_TARGET`                       | `rc` or `mc` (lowercased from target metadata) — selects baked TA catalog file |
| `ZOA_SESSION_ID` / `ZOA_OPERATOR`             | Session facts in `ZOA_SESSION.md`                                              |
| `ZOA_API_URL`, `ZOA_TARGET`, `ZOA_DEPLOYMENT` | Existing boundary wiring                                                       |

Entrypoint copies `/usr/share/zoa/catalog/ZOA_ACTIONS.{rc|mc}.md` → `/home/sre/.claude/ZOA_ACTIONS.md`. Regenerate catalogs with `go run ./hack/generate-boundary-catalog/` after TA metadata changes.

## Terraform / IAM (rosa-hyperfleet)

- Worker env: `SESSION_IDLE_TIMEOUT_SECONDS` when `sessions_table_name` is set (`terraform/modules/zoa-lambda`).
- Worker IAM: `ssm:DescribeSessions`, `logs:DescribeLogStreams` on `.../zoa-boundary/ssm-sessions` (`lambda_boundary_reaper_idle`).
- Access env: `SESSION_MAX_DURATION_HOURS` (`terraform/modules/zoa-access`).

## Operational notes

- **Leaving the Exec shell** does not stop the task; only **session stop**, **deadline reaper**, or **idle reaper** does.
- **Idle baseline `createdAt`** includes task provisioning time; very slow `RunTask` reduces time-to-idle-reap before first join.
- **SSM history pagination** is capped (20 pages); sufficient for dev/ephemeral; revisit at high join churn.
- Roll back feature bundle: git parent before `feat(boundary): idle reaper, exec session tracking, and CLI/catalog UX` on `feat/zoa-boundary`.

See also [boundary session logging](boundary-session-logging.md), [SRE access guide](../boundary/sre-access-guide.md), [boundary architecture](../boundary/architecture.md).
