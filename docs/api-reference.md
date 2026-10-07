# API Reference

The ZOA API is exposed via an AWS Lambda Function URL with IAM authentication (SigV4). Each target cluster (RC or MC) has its own API endpoint.

## Authentication

All requests must be signed with AWS SigV4. The Lambda Function URL uses `authorization_type = "AWS_IAM"`. The **`zoa` CLI** sets **`X-Operator`** to `sts:GetCallerIdentity().Arn` for the credentials that signed the request.

On the **API** Lambda, the server resolves **operator** and **session_id** via the **identity bridge** (`task-id-index` GSI), then falls back to the invoker session name for laptop `zoa run` until IAM restricts API invoke. On **Access**, **operator**, **signer_arn**, and **account_id** are set at **session start** from the invoker ARN and `X-Account-ID`. See [boundary identity and storage](design/boundary-identity-and-storage.md) and [SRE access guide — Identity](boundary/sre-access-guide.md#identity-and-sigv4).

## Base URL

```
https://<function-url-id>.lambda-url.<region>.on.aws
```

Each cluster has a unique Function URL. Set `ZOA_API_URL` to the target cluster's URL.

## Endpoints

### Health & Version

| Method | Path       | Description                                        |
| ------ | ---------- | -------------------------------------------------- |
| `GET`  | `/health`  | Health check (200 OK)                              |
| `GET`  | `/version` | Server version, commit, build time, target cluster |

### Trusted Actions

| Method | Path                                   | Description                                  |
| ------ | -------------------------------------- | -------------------------------------------- |
| `GET`  | `/api/v0/trusted-actions`              | List all registered TAs                      |
| `GET`  | `/api/v0/trusted-actions/{action}`     | Describe a specific TA (params, scope, RBAC) |
| `POST` | `/api/v0/trusted-actions/{action}/run` | Execute a TA                                 |

### Executions

| Method | Path                                       | Description                           |
| ------ | ------------------------------------------ | ------------------------------------- |
| `GET`  | `/api/v0/trusted-actions/runs`             | List executions (with filters)        |
| `GET`  | `/api/v0/trusted-actions/runs/{id}`        | Get execution details                 |
| `GET`  | `/api/v0/trusted-actions/runs/{id}/output` | Download execution output (streaming) |
| `GET`  | `/api/v0/trusted-actions/runs/{id}/logs`   | Download execution logs (streaming)   |

### Audit

| Method | Path                            | Description       |
| ------ | ------------------------------- | ----------------- |
| `GET`  | `/api/v0/trusted-actions/audit` | Query audit trail |

### Access (boundary sessions)

All Access routes use the same `/api/v0` prefix and error envelope as Trusted Actions. Served by the regional Access Lambda (`HANDLER_MODE=access`).

| Method | Path                         | Description                               |
| ------ | ---------------------------- | ----------------------------------------- |
| `GET`  | `/api/v0/targets`            | List targets (RC + MC) for the deployment |
| `POST` | `/api/v0/sessions/start`     | Start a boundary session                  |
| `GET`  | `/api/v0/sessions`           | List sessions                             |
| `POST` | `/api/v0/sessions/stop/{id}` | Stop a session                            |
| `POST` | `/api/v0/sessions/join/{id}` | Join a session (ECS Exec credentials)     |
| `POST` | `/api/v0/approve/{id}`       | Approve request (stub)                    |
| `POST` | `/api/v0/reject/{id}`        | Reject request (stub)                     |

Session and target routes require `X-Operator` (assumed-role ARN) and `X-Account-ID`, same as TA execution.

---

## POST /api/v0/trusted-actions/{action}/run

Execute a Trusted Action.

### Headers

| Header         | Required  | Description                                                                          |
| -------------- | --------- | ------------------------------------------------------------------------------------ |
| `X-Account-ID` | Yes       | AWS account ID of the caller (set by CLI from STS)                                   |
| `X-Operator`   | Yes (CLI) | Caller ARN from `GetCallerIdentity` (API: identity bridge maps task role → operator) |

### Request Body

```json
{
  "jira": "OSD-12345",
  "params": {
    "namespace": "kube-system",
    "resource": "pods"
  },
  "force": false,
  "dry_run": false,
  "execution_mode": "sync",
  "timeout_seconds": 60
}
```

| Field             | Type   | Required                | Description                                                                                                                                      |
| ----------------- | ------ | ----------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------ |
| `jira`            | string | Yes (if TA requires it) | Jira ticket for audit (format: `PROJECT-123`)                                                                                                    |
| `params`          | object | Depends on TA           | Key-value parameters for the TA                                                                                                                  |
| `force`           | bool   | No                      | Bypass write cooldown (write TAs), max concurrent limit (all TAs), and TA-level safety checks (e.g., owner reference protection in `delete_pod`) |
| `dry_run`         | bool   | No                      | Execute the TA's DryRunAction instead                                                                                                            |
| `execution_mode`  | string | No                      | Override TA's default mode (`sync` or `async`)                                                                                                   |
| `timeout_seconds` | int    | No                      | Override TA's default timeout (bounded by server max)                                                                                            |

### Response (sync, success)

```json
{
  "id": "601a65eb-48bb-4afd-b290-1577be5b1812",
  "status": "succeeded",
  "target": "eph-994026fc-regional",
  "executed_action": "get_resource",
  "execution_mode": "sync"
}
```

### Response (async, dispatched)

```json
{
  "id": "adb1e392-fb6e-4b0e-a05e-dae54c1ac835",
  "status": "dispatched",
  "target": "eph-994026fc-regional",
  "executed_action": "get_resource",
  "execution_mode": "async"
}
```

### Error Responses

| Code | Reason             | Description                                           |
| ---- | ------------------ | ----------------------------------------------------- |
| 400  | `invalid_body`     | Request body parse failure                            |
| 400  | `invalid_params`   | Missing required param or unknown param               |
| 400  | `invalid-jira`     | Jira ticket format invalid                            |
| 400  | `missing_account`  | `X-Account-ID` header missing                         |
| 400  | `timeout_exceeded` | Requested timeout exceeds server maximum              |
| 404  | `action_not_found` | TA not registered                                     |
| 429  | `write_cooldown`   | Write action executed too recently (use `force=true`) |
| 429  | `max_concurrent`   | Too many active executions on target                  |

---

## GET /api/v0/trusted-actions/runs/{id}

Get execution details.

### Response

```json
{
  "id": "601a65eb-48bb-4afd-b290-1577be5b1812",
  "action": "get_resource",
  "target_cluster": "eph-994026fc-regional",
  "status": "succeeded",
  "execution_mode": "sync",
  "scope": "kube-api",
  "type": "read",
  "dry_run": false,
  "jira": "OSD-123",
  "operator": "slopezma",
  "params": { "namespace": "kube-system", "resource": "pods" },
  "revision": "df90d53",
  "created_at": "2026-08-17T10:08:29.123Z",
  "dispatched_at": "2026-08-17T10:08:29.123Z",
  "completed_at": "2026-08-17T10:08:29.456Z",
  "duration_ms": 333,
  "output_bytes": 883,
  "log_bytes": 1024
}
```

### Statuses

| Status       | Meaning                       |
| ------------ | ----------------------------- |
| `dispatched` | Created, awaiting execution   |
| `succeeded`  | Completed successfully        |
| `failed`     | Execution failed (check logs) |
| `timed_out`  | Exceeded timeout deadline     |

---

## GET /api/v0/trusted-actions/runs

List executions with optional filters.

### Query Parameters

| Param    | Type   | Description              |
| -------- | ------ | ------------------------ |
| `status` | string | Filter by status         |
| `action` | string | Filter by action name    |
| `limit`  | int    | Max results (default 50) |

---

## GET /api/v0/trusted-actions/runs/{id}/output

Stream the execution's output artifact from S3. Returns raw JSON.

Used by `zoa output <id>` and `zoa download <id>`.

---

## GET /api/v0/trusted-actions/runs/{id}/logs

Stream the execution's log artifact from S3. Returns plain text.

Used by `zoa logs <id>`.

---

## GET /api/v0/trusted-actions/audit

Query the audit trail.

### Query Parameters

| Param      | Type   | Description              |
| ---------- | ------ | ------------------------ |
| `action`   | string | Filter by action         |
| `operator` | string | Filter by operator       |
| `limit`    | int    | Max results (default 50) |

### Response

```json
{
  "items": [
    {
      "timestamp": "2026-08-17T10:08:38.000Z",
      "method": "POST",
      "path": "list_eks_clusters/run",
      "status_code": 202,
      "operator": "slopezma",
      "action": "list_eks_clusters",
      "target_cluster": "eph-994026fc-regional",
      "jira": "TEST-1",
      "execution_id": "d5b57bf2-1bdb-42bf-9607-8abf47d5331c"
    }
  ]
}
```

---

## GET /api/v0/trusted-actions

List all registered Trusted Actions.

### Response

```json
{
  "items": [
    {
      "name": "get_resource",
      "scope": "kube-api",
      "type": "read",
      "execution_mode": "sync",
      "description": "Get or list Kubernetes resources..."
    }
  ]
}
```

---

## GET /api/v0/trusted-actions/{action}

Describe a specific TA including parameters, RBAC, and metadata.

### Response

```json
{
  "name": "get_resource",
  "scope": "kube-api",
  "type": "read",
  "execution_mode": "sync",
  "description": "Get or list Kubernetes resources...",
  "timeout_seconds": 60,
  "parameters": [
    { "name": "resource", "required": true, "description": "Resource type" },
    {
      "name": "namespace",
      "required": false,
      "description": "Target namespace"
    }
  ],
  "authorization": {
    "approval": "none"
  }
}
```
