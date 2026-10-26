# API Reference

ZOA exposes its APIs through **two Lambda Function URLs**, each serving a different operational plane:

| Lambda     | `HANDLER_MODE` | Caller                                | Purpose                                       |
| ---------- | -------------- | ------------------------------------- | --------------------------------------------- |
| **Access** | `access`       | SRE laptop (via SSM autodiscovery)    | Session lifecycle, targets, approval (future) |
| **API**    | `api`          | Boundary task (`ZOA_API_URL` env var) | Trusted Action execution, runs, audit         |

Both share the same `/api/v0` prefix, error envelope, and authentication model. Health and version endpoints are available on both.

## Authentication

All requests must be signed with AWS SigV4. Both Lambda Function URLs use `authorization_type = "AWS_IAM"`.

The **`zoa` CLI** sets **`X-Operator`** to `sts:GetCallerIdentity().Arn` for the credentials that signed the request:

- **Access Lambda** — the operator ARN comes from the **invoker** (SRE laptop, assumed via SSM-discovered invoker role). Used for session ownership.
- **API Lambda** — boundary callers use the **ECS task role**; the server resolves the human **operator** and **session id** via the **identity bridge** (task id in the ARN → DynamoDB session lookup).

See [Operator workflow — Identity and SigV4](guides/operator-workflow.md#identity-and-sigv4).

## Base URLs

**Access Lambda** (session plane — SRE laptop):

```
https://<access-function-url-id>.lambda-url.<region>.on.aws
```

The CLI autodiscovers this from SSM (`/zoa/deployments/<deployment>`) when you run `zoa session` or `zoa targets` with a deployment argument. Operators never set this URL manually.

**API Lambda** (TA execution plane — boundary task):

```
https://<api-function-url-id>.lambda-url.<region>.on.aws
```

Each target VPC (RC or MC) has its own API Lambda. Inside the boundary, `ZOA_API_URL` is pre-set by the task definition. Operators never set this URL manually either.

### Common headers

| Header         | Required | Description                                                                          |
| -------------- | -------- | ------------------------------------------------------------------------------------ |
| `X-Account-ID` | Yes      | AWS account ID of the caller (set by CLI from STS)                                   |
| `X-Operator`   | Yes      | Caller ARN from `GetCallerIdentity` (API: identity bridge maps task role → operator) |

---

# Access Lambda — Session Plane

Served by the Access Lambda (`HANDLER_MODE=access`) in the RC account (no VPC attachment).

## Endpoints

### Health & Version

| Method | Path       | Description                                     |
| ------ | ---------- | ----------------------------------------------- |
| `GET`  | `/health`  | Health check (200 OK)                           |
| `GET`  | `/version` | Server version, commit, build time, mode=access |

### Targets

| Method | Path              | Description                               |
| ------ | ----------------- | ----------------------------------------- |
| `GET`  | `/api/v0/targets` | List targets (RC + MC) for the deployment |

`GET /targets` is not written to the audit trail (read-only discovery, same class as `zoa actions` / `zoa describe` on the API Lambda).

### Sessions

| Method | Path                                  | Description                                                                                 |
| ------ | ------------------------------------- | ------------------------------------------------------------------------------------------- |
| `POST` | `/api/v0/sessions/start`              | Start a boundary session                                                                    |
| `GET`  | `/api/v0/sessions`                    | List sessions (filters: `scope`, `status`, `operator`, `target`, `since`, `until`, `limit`) |
| `POST` | `/api/v0/sessions/terminate/{id}`     | Terminate a session (stops ECS task if running)                                             |
| `POST` | `/api/v0/sessions/join/{id}`          | Join a session (provisions boundary task if needed, returns ECS Exec credentials)           |
| `POST` | `/api/v0/sessions/exec-attached/{id}` | Record ECS Exec session attachment (called by CLI after `session-manager-plugin` connects)  |

### Approval (future)

Approval stubs are registered on the Access Lambda today. The approval workflow will expand to cover TAs and break-glass when those features ship.

| Method | Path                   | Description                          |
| ------ | ---------------------- | ------------------------------------ |
| `POST` | `/api/v0/approve/{id}` | Approve request (stub — returns 501) |
| `POST` | `/api/v0/reject/{id}`  | Reject request (stub — returns 501)  |

---

## POST /api/v0/sessions/start

Start a new boundary session.

### Request Body

```json
{
  "deployment_name": "us-east-1",
  "target": "regional",
  "reason": "ROSAENG-1234",
  "timeout_hours": 4
}
```

| Field             | Type   | Required | Description                                                                |
| ----------------- | ------ | -------- | -------------------------------------------------------------------------- |
| `deployment_name` | string | Yes      | Deployment name (from `zoa deployments`)                                   |
| `target`          | string | Yes      | Target cluster ID (from `zoa targets`)                                     |
| `reason`          | string | Yes      | Jira ticket or PagerDuty incident (validated format)                       |
| `timeout_hours`   | int    | No       | Session duration in hours (default: max configured, bounded by server max) |

### Response

```json
{
  "session_id": "1989937d-0889-485d-b64e-facee9e0150d",
  "status": "active",
  "target": "regional",
  "task_arn": "arn:aws:ecs:us-east-1:123456789012:task/cluster/task-abc",
  "deadline": "2026-10-13T19:27:40.603274166Z",
  "region": "us-east-1"
}
```

On success, Access has already called `ecs:RunTask` for the boundary container in the target VPC. The session row is **`active`** with `task_arn` set before the response is returned.

### Error Responses

| Code | Reason                 | Description                                    |
| ---- | ---------------------- | ---------------------------------------------- |
| 400  | `invalid_identity`     | Cannot extract SRE identity from `X-Operator`  |
| 400  | `invalid_request`      | Invalid JSON body                              |
| 400  | `missing_target`       | Target field is required                       |
| 400  | `missing_reason`       | Reason is required                             |
| 400  | `invalid_reason`       | Reason format invalid (must be Jira/PD format) |
| 400  | `invalid_timeout`      | Timeout exceeds configured maximum             |
| 404  | `target_not_found`     | Target not registered                          |
| 502  | `ecs_error`            | Boundary `RunTask` failed                      |
| 500  | `session_update_error` | Task started but session state update failed   |

---

## GET /api/v0/sessions

List sessions. Used by both `zoa session list` (own sessions) and `zoa session history` (all operators).

### Query Parameters

| Param      | Type   | Description                                                              |
| ---------- | ------ | ------------------------------------------------------------------------ |
| `scope`    | string | `mine` for own sessions only (default: all operators)                    |
| `status`   | string | Filter by session status (`active`, `terminated`, `failed`, `creating`)  |
| `operator` | string | Filter by operator (not allowed with `scope=mine`)                       |
| `target`   | string | Filter by target cluster                                                 |
| `since`    | string | Start of time window (duration: `1h`, `7d`; date: `2026-08-25`; RFC3339) |
| `until`    | string | End of time window (same formats as `since`; default: now)               |
| `limit`    | int    | Max results (default 50, max 200)                                        |

### Response

```json
{
  "items": [
    {
      "sessionId": "1989937d-0889-485d-b64e-facee9e0150d",
      "operator": "slopezma",
      "targetCluster": "regional",
      "reason": "ROSA-123567",
      "status": "terminated",
      "stopReason": "idleReaperStop",
      "createdAt": "2026-10-13T15:27:40.603274166Z",
      "deadline": "2026-10-13T19:27:40.603274166Z"
    }
  ],
  "count": 1
}
```

**Audit:** cross-operator queries (no `scope=mine`) are recorded as `session_history` in the audit trail. Own-session list (`scope=mine`) is not audited.

---

## POST /api/v0/sessions/terminate/{id}

Terminate a session. Stops the ECS task if running and marks the session as terminated.

### Error Responses

| Code | Reason                  | Description                             |
| ---- | ----------------------- | --------------------------------------- |
| 403  | `forbidden`             | Only the session owner can terminate it |
| 404  | `not_found`             | Session not found                       |
| 409  | `invalid_status`        | Session is not active or creating       |
| 502  | `task_terminate_failed` | ECS StopTask failed                     |

---

## POST /api/v0/sessions/join/{id}

Join an **active** session that already has a boundary task (`task_arn` set on `session start`). Returns connection info and ECS Exec credentials scoped to that task only.

### Response

```json
{
  "session_id": "db4baeeb-c818-4c85-8cb7-fd07d15b9aac",
  "cluster": "arn:aws:ecs:us-east-1:123456789012:cluster/us-east-1-zoa-boundary",
  "task_arn": "arn:aws:ecs:us-east-1:123456789012:task/us-east-1-zoa-boundary/abc123",
  "region": "us-east-1",
  "container_name": "zoa-boundary",
  "exec_command": "runuser -u sre -- /bin/bash -l",
  "exec_credentials": {
    "access_key_id": "ASIA...",
    "secret_access_key": "...",
    "session_token": "...",
    "expiration": "2026-10-13T17:27:40Z"
  },
  "deadline": "2026-10-13T19:27:40.603274166Z"
}
```

### Error Responses

| Code | Reason                 | Description                                  |
| ---- | ---------------------- | -------------------------------------------- |
| 403  | `forbidden`            | Only the session owner can join              |
| 404  | `not_found`            | Session not found                            |
| 409  | `invalid_status`       | Session is not `active`                      |
| 409  | `session_not_ready`    | Boundary task not provisioned (retry start)  |

---

## POST /api/v0/sessions/exec-attached/{id}

Record that an ECS Exec session has attached. Called by the CLI after `session-manager-plugin` connects successfully.

### Request Body

```json
{
  "exec_session_id": "k7zkjuilu2vhsrp76e48ie3ciy"
}
```

### Error Responses

| Code | Reason                    | Description                                   |
| ---- | ------------------------- | --------------------------------------------- |
| 400  | `invalid_exec_session_id` | Exec session ID format invalid                |
| 403  | `forbidden`               | Only the session owner can record attachments |

---

# API Lambda — Trusted Action Plane

Served by the API Lambda (`HANDLER_MODE=api`) in each target VPC.

## Endpoints

### Health & Version

| Method | Path       | Description                                        |
| ------ | ---------- | -------------------------------------------------- |
| `GET`  | `/health`  | Health check (200 OK)                              |
| `GET`  | `/version` | Server version, commit, build time, target cluster |

### Audit

| Method | Path            | Description       |
| ------ | --------------- | ----------------- |
| `GET`  | `/api/v0/audit` | Query audit trail |

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

---

## GET /api/v0/audit

Query the audit trail. The store holds TA executions (API Lambda), session operations (Access Lambda), and **meta** API reads (listing runs, fetching artifacts, querying this endpoint). Each successful meta read appends one row (for example `audit_query` when you call this endpoint).

### Query Parameters

| Param      | Type   | Description                                                                                          |
| ---------- | ------ | ---------------------------------------------------------------------------------------------------- |
| `action`   | string | Filter by `action` field — TA name, `session_*`, or API meta actions (`audit_query`, `runs_list`, …) |
| `operator` | string | Filter by operator                                                                                   |
| `limit`    | int    | Max results (default 50)                                                                             |

### Audit `action` values

The `action` field is a **stable event type** for filtering (`zoa audit --action …`). It is not the HTTP path. High-volume **read-only catalog** calls are intentionally **not** audited (no row, no `action`).

| Audited? | Plane  | HTTP / CLI                                                    | `action` value                                |
| -------- | ------ | ------------------------------------------------------------- | --------------------------------------------- |
| Yes      | Access | `POST …/sessions/start` · `zoa session start`                 | `session_start`                               |
| Yes      | Access | `POST …/sessions/join/{id}`                                   | `session_join`                                |
| Yes      | Access | `POST …/sessions/terminate/{id}`                              | `session_terminate`                           |
| Yes      | Access | `POST …/sessions/exec-attached/{id}`                          | `session_exec_attached`                       |
| Yes      | Access | `GET …/sessions` without `scope=mine` · `zoa session history` | `session_history`                             |
| No       | Access | `GET …/sessions?scope=mine` · `zoa session list`              | — (self-service; avoid noise)                 |
| No       | Access | `GET …/targets` · `zoa targets`                               | — (read-only catalog)                         |
| No       | Access | `GET /health`, `GET /version`                                 | —                                             |
| Yes      | API    | `POST …/trusted-actions/{ta}/run` · `zoa run`                 | **TA registry name** (e.g. `get_db_resource`) |
| Yes      | API    | `GET …/trusted-actions/runs` · `zoa runs`                     | `runs_list`                                   |
| Yes      | API    | `GET …/trusted-actions/runs/{id}` · `zoa get`                 | `run_get`                                     |
| Yes      | API    | `GET …/runs/{id}/output` · `zoa output` / `download`          | `run_output`                                  |
| Yes      | API    | `GET …/runs/{id}/logs` · `zoa logs`                           | `run_logs`                                    |
| Yes      | API    | `GET /api/v0/audit` · `zoa audit`                             | `audit_query`                                 |
| No       | API    | `GET …/trusted-actions` · `zoa actions`                       | — (read-only catalog)                         |
| No       | API    | `GET …/trusted-actions/{action}` · `zoa describe`             | — (read-only catalog)                         |
| No       | API    | `GET /health`, `GET /version`                                 | —                                             |

**Families:** `session_*` = Access session plane; TA name = Trusted Action dispatch; `runs_*` / `run_*` = execution reads; `audit_query` = querying the audit index itself.

### Response

```json
{
  "items": [
    {
      "timestamp": "2026-08-17T10:08:38.000Z",
      "method": "POST",
      "path": "/api/v0/trusted-actions/list_eks_clusters/run",
      "status_code": 202,
      "operator": "slopezma",
      "action": "list_eks_clusters",
      "target_cluster": "regional",
      "reason": "ROSAENG-1234",
      "execution_id": "d5b57bf2-1bdb-42bf-9607-8abf47d5331c"
    }
  ]
}
```

---

## POST /api/v0/trusted-actions/{action}/run

Execute a Trusted Action. Called from inside the boundary via `zoa run`.

### Request Body

```json
{
  "reason": "ROSAENG-1234",
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

| Field             | Type   | Required      | Description                                                                                                                                      |
| ----------------- | ------ | ------------- | ------------------------------------------------------------------------------------------------------------------------------------------------ |
| `reason`          | string | Yes           | Jira issue or PagerDuty incident (e.g. `ROSAENG-1234`, `#123456`)                                                                                |
| `params`          | object | Depends on TA | Key-value parameters for the TA                                                                                                                  |
| `force`           | bool   | No            | Bypass write cooldown (write TAs), max concurrent limit (all TAs), and TA-level safety checks (e.g., owner reference protection in `delete_pod`) |
| `dry_run`         | bool   | No            | Execute the TA's DryRunAction instead                                                                                                            |
| `execution_mode`  | string | No            | Override TA's default mode (`sync` or `async`)                                                                                                   |
| `timeout_seconds` | int    | No            | Override TA's default timeout (bounded by server max)                                                                                            |

### Response (sync, success)

```json
{
  "id": "601a65eb-48bb-4afd-b290-1577be5b1812",
  "status": "succeeded",
  "target": "regional",
  "executed_action": "get_resource",
  "execution_mode": "sync"
}
```

### Response (async, dispatched)

```json
{
  "id": "adb1e392-fb6e-4b0e-a05e-dae54c1ac835",
  "status": "dispatched",
  "target": "regional",
  "executed_action": "get_resource",
  "execution_mode": "async"
}
```

### Error Responses

| Code | Reason                              | Description                                           |
| ---- | ----------------------------------- | ----------------------------------------------------- |
| 400  | `invalid_body`                      | Request body parse failure                            |
| 400  | `invalid_params`                    | Missing required param or unknown param               |
| 400  | `missing_reason` / `invalid_reason` | Reason missing or format invalid                      |
| 400  | `missing_account`                   | `X-Account-ID` header missing                         |
| 400  | `timeout_exceeded`                  | Requested timeout exceeds server maximum              |
| 404  | `action_not_found`                  | TA not registered                                     |
| 429  | `write_cooldown`                    | Write action executed too recently (use `force=true`) |
| 429  | `max_concurrent`                    | Too many active executions on target                  |

---

## GET /api/v0/trusted-actions/runs/{id}

Get execution details.

### Response

```json
{
  "id": "601a65eb-48bb-4afd-b290-1577be5b1812",
  "action": "get_resource",
  "target_cluster": "regional",
  "status": "succeeded",
  "execution_mode": "sync",
  "scope": "kube-api",
  "type": "read",
  "dry_run": false,
  "reason": "ROSAENG-1234",
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
