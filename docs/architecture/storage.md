# ZOA storage

ZOA state lives in **DynamoDB** (metadata, sessions, audit) and **S3** (TA artifacts). All tables use **KMS encryption**, **TTL** for expiry, and **PITR** outside ephemeral environments.

Terraform source: `rosa-hyperfleet/terraform/modules/zoa/` (`dynamodb.tf`, `s3.tf`, `kms.tf`).

## DynamoDB tables

| Table | Primary key | Purpose |
| ----- | ----------- | ------- |
| **Executions** | PK `executionId` | TA run lifecycle, cooldown/concurrency queries |
| **Audit** | PK `accountId`, SK `timestamp` | API call audit trail |
| **Boundary sessions** | PK `sessionId` (UUID) | Session lifecycle, identity bridge |

TTL attribute: **`ttl`** (Unix epoch), set on write from **`DYNAMODB_TTL_DAYS`** (default **365** days) for executions, audit, and sessions.

### Executions — GSIs

| GSI | Keys | Used by |
| --- | ---- | ------- |
| `target-status-index` | PK `targetCluster`, SK `targetStatusKey` (`{status}#{timestamp}`) | Reconciler, GC, concurrency limiter (per target) |
| `date-bucket-index` | PK `dateBucket` (`YYYY-MM-DD`), SK `createdAt` | `zoa runs` list, write cooldown (today’s bucket) |

### Audit — GSIs

| GSI | Keys | Used by |
| --- | ---- | ------- |
| `date-bucket-index` | PK `dateBucket`, SK `timestamp` | `zoa audit` (CLI default `--since 24h`) |

### Boundary sessions — GSIs

| GSI | Keys | Used by |
| --- | ---- | ------- |
| `task-id-index` | PK `taskId` (ECS task UUID) | API **identity bridge** (`GetByTaskID`) |
| `status-deadline-index` | PK `status`, SK `deadline` | Worker **reaper** (hard + idle paths) |
| `date-bucket-index` | PK `dateBucket`, SK `createdAt` | `zoa session list` / `session history` |

`sessionId` ≠ `taskId`: one session row holds both after the task is active. See [identity and storage](../design/boundary-identity-and-storage.md).

### List query pattern

`zoa runs`, `zoa audit`, and `zoa session list/history` iterate **`date-bucket-index`** day-by-day from newest to oldest, with optional **FilterExpression** (target, operator, status, action, etc.). CLI defaults **`--since 24h`** so queries stay bounded.

## S3 artifact bucket

| Aspect | Behavior |
| ------ | -------- |
| **Encryption** | SSE-KMS with regional ZOA CMK; bucket key enabled |
| **Versioning** | Enabled |
| **Public access** | Blocked |
| **Object layout** | `executions/{executionId}/output.json`, `execution.log`, optional `output.tar.gz` (must-gather) |
| **Lifecycle** | Transition to Intelligent-Tiering at 30 days; expiration per `output_retention_days`; noncurrent version expiry 30 days |

Sync TA output is returned inline in the HTTP response; S3 is **authoritative** for async runs and long-term retention.

## Cross-account (MC)

MC API/Worker Lambdas assume a **data store role** in RC to read/write executions, audit, sessions, and S3. Resource policies on DynamoDB tables scope principals to org/ZOA Lambda roles.

## Related

- [Implementation details](implementation.md)
- [Boundary identity and storage](../design/boundary-identity-and-storage.md)
- [Observability](../observability.md)
