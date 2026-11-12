# ZOA storage

ZOA state lives in **DynamoDB** (metadata, sessions, audit) and **S3** (TA artifacts). All tables use **KMS encryption**, **TTL** for expiry, and **PITR** outside ephemeral environments.

Terraform source: `rosa-seaplane/terraform/modules/zoa/` (`dynamodb.tf`, `s3.tf`, `kms.tf`).

## DynamoDB tables

| Table                 | Primary key                    | Purpose                                                              |
| --------------------- | ------------------------------ | -------------------------------------------------------------------- |
| **Executions**        | PK `executionId`               | TA run lifecycle, cooldown/concurrency queries                       |
| **Audit**             | PK `accountId`, SK `timestamp` | API call audit trail                                                 |
| **Boundary sessions** | PK `sessionId` (UUID)          | Session lifecycle, [identity bridge](../boundary/identity-bridge.md) |
| **Break-glass**       | PK `requestId` (UUID)          | Break-glass request/grant lifecycle ([break-glass](../break-glass/README.md)) |

TTL attribute: **`ttl`** (Unix epoch), set on write from **`DYNAMODB_TTL_DAYS`** (default **365** days) for executions, audit, and sessions.

### Executions — GSIs

| GSI                   | Keys                                                              | Used by                                          |
| --------------------- | ----------------------------------------------------------------- | ------------------------------------------------ |
| `target-status-index` | PK `targetCluster`, SK `targetStatusKey` (`{status}#{timestamp}`) | Reconciler, GC, concurrency limiter (per target) |
| `date-bucket-index`   | PK `dateBucket` (`YYYY-MM-DD`), SK `createdAt`                    | `zoa runs` list, write cooldown (today’s bucket) |

### Audit — GSIs

| GSI                 | Keys                            | Used by                                 |
| ------------------- | ------------------------------- | --------------------------------------- |
| `date-bucket-index` | PK `dateBucket`, SK `timestamp` | `zoa audit` (CLI default `--since 24h`) |

### Boundary sessions

**Access** writes rows at `session_start`; **`task_id`** is added when the ECS task is active. **`sessionId`** (PK) is the stable audit handle; **`taskId`** is the identity-bridge lookup key (same value as the STS session name on the task role ARN).

| Attribute (Dynamo) | JSON               | Purpose                                                                              |
| ------------------ | ------------------ | ------------------------------------------------------------------------------------ |
| `sessionId`        | `session_id`       | UUID at start                                                                        |
| `operator`         | `operator`         | Human SRE at start                                                                   |
| `signerARN`        | `signer_arn`       | Invoker ARN at start                                                                 |
| `accountId`        | `account_id`       | Caller account at start                                                              |
| `taskId`           | `task_id`          | ECS task UUID — GSI **`task-id-index`**                                              |
| `taskArn`          | `task_arn`         | Full ARN for ECS stop/exec                                                           |
| `deadline`         | `deadline`         | Max session end (RFC3339)                                                            |
| `targetCluster`    | `target_cluster`   | MC/RC target name — GSI **`target-status-index`** PK                                 |
| `targetStatusKey`  | —                  | GSI sort key; while **active**, `active#<deadline>` (RFC3339Nano) — see note below   |
| `stopReason`       | `stop_reason`      | `operatorStop`, `deadlineReaperStop`, `staleCreatingReaperStop`, `idleReaperStop`, … |
| `execSessionIds`   | `exec_session_ids` | ECS Exec SSM ids → CloudWatch `ecs-execute-command-<id>` (not ZOA `session_id`)      |
| `dateBucket`       | —                  | `YYYY-MM-DD` for list/history                                                        |

#### Boundary sessions — GSIs

| GSI                   | Keys                                     | Used by                                                                                      |
| --------------------- | ---------------------------------------- | -------------------------------------------------------------------------------------------- |
| `task-id-index`       | PK `taskId`                              | API **`GetByTaskID`** (identity bridge)                                                      |
| `target-status-index` | PK `targetCluster`, SK `targetStatusKey` | Worker **reaper** per `TARGET_CLUSTER` (expired deadline, stale `creating`, idle candidates) |
| `date-bucket-index`   | PK `dateBucket`, SK `createdAt`          | `zoa session list` / `session history`                                                       |

**Same GSI name, different suffix rules:** on **executions**, `targetStatusKey` is `{status}#{createdAt}`; on **sessions**, active rows use `{status}#{deadline}` so the reaper can query by deadline within a `targetCluster` partition. Reaper queries: [session reaper](../boundary/session-reaper.md).

**Targets** (RC/MC wiring for `session start`) are **not** in DynamoDB — they are SSM parameters under `/zoa/targets/...`. See [deployment discovery](../guides/deployments-discovery.md).

### List query pattern

`zoa runs`, `zoa audit`, and `zoa session list/history` iterate **`date-bucket-index`** day-by-day from newest to oldest, with optional **FilterExpression** (target, operator, status, action, etc.). CLI defaults **`--since 24h`** so queries stay bounded.

## S3 artifact bucket

| Aspect            | Behavior                                                                                                                |
| ----------------- | ----------------------------------------------------------------------------------------------------------------------- |
| **Encryption**    | SSE-KMS with regional ZOA CMK; bucket key enabled                                                                       |
| **Versioning**    | Enabled                                                                                                                 |
| **Public access** | Blocked                                                                                                                 |
| **Object layout** | `executions/{executionId}/output.json`, `execution.log`, optional `output.tar.gz` (must-gather)                         |
| **Lifecycle**     | Transition to Intelligent-Tiering at 30 days; expiration per `output_retention_days`; noncurrent version expiry 30 days |

Sync TA output is returned inline in the HTTP response; S3 is **authoritative** for async runs and long-term retention.

## Cross-account (MC)

MC API/Worker Lambdas use **resource-based policies** on RC DynamoDB tables and S3 (and KMS) so MC Lambdas in the MC account can read/write executions, audit, sessions, and artifacts without a separate data-store AssumeRole for DynamoDB. Access Lambda and session table remain in the RC account.

## Related

- [Identity bridge](../boundary/identity-bridge.md)
- [Session reaper](../boundary/session-reaper.md)
- [Implementation details](implementation.md)
- [Observability](../observability.md)
