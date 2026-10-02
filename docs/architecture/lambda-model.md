# ZOA Lambda Model

## Overview

ZOA deploys **multiple Lambda functions per deployment**:

| Lambda | Trigger | Purpose |
|---|---|---|
| **API** | Function URL (IAM auth) | CLI requests, sync TAs, streaming downloads (per VPC: RC + each MC) |
| **Worker** | EventBridge + self-invoke | Reconciler (1m), GC (5m), **boundary session reaper** (5m), async/approved TA execution |
| **Access** | Function URL (IAM auth) | ZOA boundary session lifecycle, target discovery (RC account, no VPC) |

API, Worker, and Access use the **same container image** (`zoa-lambda`), differentiated by the `HANDLER_MODE` environment variable (`api`, `worker`, `access`).

## Why Separate Functions

Each Lambda has different operational characteristics that are per-function AWS settings:

| Setting | API | Worker | Access |
|---------|-----|--------|--------|
| Invocation | Function URL (streaming) | EventBridge + self-invoke | Function URL (standard) |
| VPC | Yes (EKS access) | Yes (EKS access) | No (ECS RunTask, DynamoDB only) |
| Throttle behavior | HTTP 429 to caller | AWS queues + retries (up to 6h) | HTTP 429 to caller |

These cannot share a single function because timeout, concurrency, invocation mode (streaming vs standard handler), and VPC attachment are per-function settings.

Timeout and concurrency values are configured in Terraform (`terraform/modules/zoa-lambda/variables.tf`, `terraform/modules/zoa-access/`).

## Event Routing

The Worker Lambda uses the `route` field in the event payload:

```json
{"route": "reconciler"}
{"route": "gc"}
{"route": "reaper"}
{"route": "execute", "execution_id": "abc123"}
```

The **reaper** route scans DynamoDB for boundary sessions past their deadline and calls `ecs:StopTask` on the matching boundary ECS cluster (implemented in `pkg/scheduler/reaper.go`, scheduled from `terraform/modules/zoa-lambda/main.tf`).

## Self-Invocation Pattern

When the reconciler dispatches an approved TA:

1. Reconciler queries DynamoDB for `status=approved` executions
2. Atomically transitions each to `status=dispatched` (conditional write)
3. Invokes itself with `InvocationType=Event` (async, fire-and-forget):
   ```go
   lambdaClient.Invoke(ctx, &lambda.InvokeInput{
       FunctionName:   aws.String(os.Getenv("AWS_LAMBDA_FUNCTION_NAME")),
       InvocationType: lambdatypes.InvocationTypeEvent,
       Payload:        []byte(`{"route":"execute","execution_id":"abc123"}`),
   })
   ```
4. AWS Lambda queues the invocation and executes it in a separate concurrent slot

### Concurrency Model

Each Lambda has its own `reserved_concurrent_executions` pool (configured in Terraform via `lambda_api_concurrency` and `lambda_worker_concurrency`):

- **API Lambda** — each concurrent CLI request uses one slot. Under saturation, excess requests get throttled (429).
- **Worker Lambda** — scheduled ticks (reconciler, GC, reaper) and self-invoked TA executions share the worker pool. Excess invocations queue in Lambda's internal retry queue (up to 6 hours).

Reserved concurrency guarantees slots are always available (not stolen by other functions in the account) and caps cost by limiting fan-out.

See `terraform/modules/zoa-lambda/variables.tf` for current values.

## Code-Level Deadlines

Each route gets a `context.WithTimeout` enforced in code:

| Route | Deadline | Why |
|---|---|---|
| `reconciler` | 55s | Must finish quickly; runs every 60s |
| `gc` | 55s | Cleanup can be deferred; runs every 5min |
| `reaper` | 55s | Session enforcement; runs every 5min |
| `execute` | 295s | TA execution; bounded by Lambda timeout |

All values are **env-var tunable** without code redeployment.

## Batch Limits

Each scheduled phase processes at most `MAX_BATCH_PER_TICK` items (default 30). If:
- The batch limit is reached, or
- `ctx.Err() != nil` (deadline approaching)

...remaining items are deferred to the next tick. This prevents timeout under load.

## Data Flow

```mermaid
graph TD
    CLI["CLI (laptop)"] -->|"Access Function URL"| Access["Access Lambda"]
    CLI -->|"ECS Exec"| Boundary["ZOA boundary"]
    Boundary -->|"API Function URL, SigV4"| API["API Lambda"]
    API -->|"streaming response"| Boundary

    EB["EventBridge Scheduler"] -->|"rate(1m) / rate(5m)"| Worker["Worker Lambda<br/>(reconciler, GC, reaper)"]
    Worker -->|"self-invoke<br/>InvocationType=Event"| Slot["Worker Lambda<br/>(concurrent slot)"]

    Access --> Data["DynamoDB, ECS"]
    API --> Data2["DynamoDB, S3, K8s"]
    Worker --> Data2
    Slot -->|"execute single TA"| Data2
```

## Cross-Account Access (MC Lambdas)

MC Lambdas access RC data layer (DynamoDB, S3, KMS) via resource-based policies:
- DynamoDB: `aws_dynamodb_resource_policy` allowing the MC Lambda role
- S3: Bucket policy with `AllowCrossAccountLambdaAccess`
- KMS: Key policy with `AllowCrossAccountLambdaKMS`

No STS AssumeRole needed for data access — policies are on the resources themselves.

## Design Decisions

| Decision | Rationale |
|---|---|
| Self-invoke over SQS | Simpler (no queue config), automatic retries, Lambda handles backpressure |
| Reserved concurrency | Prevents noisy-neighbor issues; caps cost |
| 300s Lambda timeout | Gives 295s for code; handles slow TAs without hitting AWS 900s limit |
| Same binary | Zero divergence; route by env var + event payload |
| Access via Function URL | IAM auth without API Gateway; SSM publishes URL for CLI discovery |
| EMF metrics | CW Exporter already scrapes CloudWatch; EMF is zero-config |
| SQS DLQ | Catch failures from EventBridge and self-invocations |
