# ZOA Boundary + Access Lambda Epic Plan (ROSAENG-60291)

## Context

Today, SREs call per-VPC Lambda Function URLs **directly from their laptop** (the `TEMPORARY` path in the architecture diagram). This epic delivers the **target state**: SREs interact with ZOA exclusively from audited, time-boxed ECS Fargate containers ("ZOA Boundary") placed inside each target VPC.

The epic follows the format of [ROSAENG-65229](https://redhat.atlassian.net/browse/ROSAENG-65229) (ZOA Lambda Rearchitecture).

**Last plan revision:** 2026-10-06 (aligned with `feat/zoa-boundary` in `rosa-hyperfleet` + `rosa-hyperfleet-zoa`).

### Jira index

Child stories were defined in this document before Jira subtasks existed. Create or link issues under [ROSAENG-60291](https://redhat.atlassian.net/browse/ROSAENG-60291) using the titles below. Team: **[ROSA] HyperFleet** (`customfield_10001`: `0c538cd9-152b-49f6-ad7c-e2fa2f865809`). Do **not** set Component (HyperFleet convention).

| #   | Title (Jira summary)                                                                | Primary repo                             | Jira key                                                           |
| --- | ----------------------------------------------------------------------------------- | ---------------------------------------- | ------------------------------------------------------------------ |
| 1   | ZOA Boundary Core — Access Lambda, boundary container, CLI, identity bridge, reaper | `rosa-hyperfleet-zoa`                    | [ROSAENG-68805](https://redhat.atlassian.net/browse/ROSAENG-68805) |
| 2   | ZOA Boundary Infrastructure — Terraform modules, SSM autodiscovery, DynamoDB, IAM   | `rosa-hyperfleet`                        | [ROSAENG-68806](https://redhat.atlassian.net/browse/ROSAENG-68806) |
| 3   | Konflux Pipeline — ZOA Boundary Image                                               | `rosa-hyperfleet-zoa`                    | [ROSAENG-68807](https://redhat.atlassian.net/browse/ROSAENG-68807) |
| 4   | Observability — ZOA Access Lambda + Boundary Sessions                               | `configuration` + hyperfleet             | [ROSAENG-68808](https://redhat.atlassian.net/browse/ROSAENG-68808) |
| 5   | E2E Testing — boundary session lifecycle                                            | `rosa-hyperfleet`                        | [ROSAENG-68809](https://redhat.atlassian.net/browse/ROSAENG-68809) |
| 6   | ZOA Boundary Documentation — architecture, CLI reference, SRE runbook               | both                                     | [ROSAENG-68810](https://redhat.atlassian.net/browse/ROSAENG-68810) |
| 7   | App-interface: Central Account SAML role for ZOA Access invoker (dev, int, stage)   | app-interface + `rosa-hyperfleet` config | [ROSAENG-68811](https://redhat.atlassian.net/browse/ROSAENG-68811) |

### Implementation status (2026-10-06)

Work is on branch **`feat/zoa-boundary`**. Config pins container/Lambda tags to **`a1fa929`** (`rosa-hyperfleet/config/defaults.yaml`); images must exist in Quay before regions pick up UX fixes.

| Area                                                            | Status              | Notes                                                                                                                                                                   |
| --------------------------------------------------------------- | ------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Access / API / Worker Lambda (`HANDLER_MODE`)                   | **Shipped in code** | Same `zoa-lambda` image; Access deployed via `terraform/modules/zoa-access`                                                                                             |
| ZOA CLI: deployments, targets, session, audit                   | **Shipped**         | `session list` = **your** sessions (`scope=mine`, default `--status all`, 24h window); `session history` = all operators                                                |
| Session start / join                                            | **Shipped**         | Default **ECS Exec connect** after RUNNING; use `--no-connect` for metadata only                                                                                        |
| Offline TA catalog                                              | **Shipped**         | `zoa actions --offline`, `zoa catalog`, `-o markdown`; `ZOA_TARGET_TYPE` (legacy `ZOA_DEPLOYMENT_TARGET`); entrypoint writes `~/.claude/ZOA_ACTIONS.md` at task start   |
| Boundary image UX                                               | **Shipped in repo** | Compound PS1 `sessionId:<deployment>/<uuid>`, exit hints, expanded `CLAUDE.md` — requires **boundary image redeploy**                                                   |
| Identity bridge + sessions DynamoDB                             | **Shipped**         | `task-id-index` GSI; operator from invoker SigV4 session name                                                                                                           |
| Exec session ids (`exec-attached`)                              | **Partial**         | CLI registers SSM exec id on join; **HTTP 500** seen in ephemeral — redeploy Access Lambda with surfaced error reason; re-test                                          |
| Session recording Layer 2 (auditd / PROMPT_COMMAND)             | **Not shipped**     | **SSM ECS Exec → CloudWatch** + **DynamoDB audit** only today; structured command layer tracked in [boundary-session-logging.md](../design/boundary-session-logging.md) |
| Konflux boundary pipeline                                       | **Open**            | Story 3                                                                                                                                                                 |
| Observability / E2E / full doc pass                             | **Partial**         | `docs/boundary/*` exists; epic AC 13–15 not fully closed                                                                                                                |
| SAML hub role for invoker (not `OrganizationAccountAccessRole`) | **Open**            | Story 7 — app-interface + `aws.zoa_access_trusted_assumer_role_names` per env                                                                                           |

### Why this matters

The direct laptop path was a bootstrapping shortcut. It has fundamental gaps:

- **No session auditing**: No record of terminal I/O or command history — SRE actions between TA executions are invisible
- **No network isolation**: SRE laptop on corporate VPN can reach any Function URL — lateral movement is possible if credentials are compromised
- **No identity bridge**: The SRE's personal IAM role is the only identity — no per-session attribution, no time-boxing, no single-SRE enforcement
- **FedRAMP non-compliant**: FedRAMP requires complete audit trails for all privileged operations, including interactive sessions — not just TA executions
- **No break-glass path**: Without a container in the target VPC, there is no private network path for future break-glass EKS access (EKS is fully private, no public endpoint)

### Architecture diagrams

#### 1. End-to-end SRE workflow

Shows the complete flow from SRE authentication through TA execution inside a boundary container. Two authentication domains are visible: Central Account (laptop → invoker role → Access Lambda Function URL) and ECS task role (container to per-VPC Function URL).

```mermaid
sequenceDiagram
    participant SRE as SRE Laptop
    participant JA as AWS Central Account
    participant PS as SSM Parameter Store
    participant IR as RC Invoker Role
    participant AL as ZOA Access Lambda (Function URL)
    participant DDB as DynamoDB
    participant ECS as ECS Fargate Task
    participant FU as Per-VPC Lambda Function URL
    participant EKS as Target EKS

    Note over SRE,JA: Authentication (requires RH VPN for kinit only)
    SRE->>JA: kinit + rh-aws-saml-login → Central Account IAM role

    Note over SRE,PS: Deployment autodiscovery (direct SSM read, no Lambda)
    SRE->>PS: zoa deployments → read /zoa/deployments
    PS-->>SRE: {us-east-1: {access_url, invoker_role_arn}, ...}

    Note over SRE,AL: Target discovery (via ZOA Access)
    SRE->>IR: sts:AssumeRole (central hub role → invoker)
    SRE->>AL: zoa targets us-east-1 (SigV4 with invoker role creds)
    AL->>DDB: read targets from SSM /zoa/targets/<deployment>/ (RC-local)
    AL-->>SRE: [rc, mc01, mc02]

    Note over SRE,ECS: Session creation (Access Lambda creates ECS task)
    SRE->>AL: zoa session start us-east-1 mc01 (SigV4 with invoker role)
    AL->>DDB: write boundary-sessions {taskId, operator, target, deadline}
    AL->>ECS: ecs:RunTask in mc01 VPC (inject ZOA_ENDPOINT, ZOA_TARGET)
    AL-->>SRE: {taskId, status: creating}
    Note over AL,ECS: Wait for RUNNING...
    AL-->>SRE: {taskId, status: active}

    Note over SRE,ECS: Interactive session (SSM WebSocket)
    SRE->>ECS: zoa session join ID → aws ecs execute-command (SSM)
    Note over ECS: SRE inside audited ZOA Boundary container

    Note over ECS,EKS: TA execution (from inside container)
    ECS->>FU: zoa run get_resource ... (SigV4 with ECS task role)
    FU->>DDB: identity bridge: task ARN → SRE identity
    FU->>EKS: execute TA (ephemeral SA + RBAC)
    EKS-->>FU: result
    FU-->>ECS: streamed output to SRE terminal
```

#### 2. Two-layer discovery architecture

Shows why deployment pointers live in the Central Account (CLI needs them before contacting any ZOA service) while target details live in the RC account (ZOA Access Lambda needs them to create ECS tasks, and they contain sensitive infrastructure data like VPC IDs and subnet IDs that should not leak to the Central Account).

```mermaid
graph TD
    subgraph centralAccount [Central Account — thin pointer layer, 1 per env]
        paramEnvs["/zoa/deployments SSM Parameter<br/>{deployment_name: access_url, invoker_role_arn}"]
    end

    subgraph rcAccount [RC Account — full target registry]
        boundaryTargets["SSM /zoa/targets/<deployment>/<cluster><br/>{target_type, vpc_id, subnet_ids,<br/>security_group_id, function_url,<br/>account_id}"]
        accessLambda["ZOA Access Lambda"]
    end

    subgraph sreLaptop [SRE Laptop]
        zoaCLI["zoa CLI"]
    end

    subgraph terraform [Terraform Pipelines]
        rcPipeline["RC Pipeline"]
        mcPipeline["MC Pipeline"]
    end

    zoaCLI -->|"zoa deployments<br/>(direct SSM read)"| paramEnvs
    zoaCLI -->|"zoa targets &lt;deployment&gt;<br/>(assume invoker role → Function URL)"| accessLambda
    accessLambda -->|"local read<br/>(same account, no cross-account)"| boundaryTargets

    rcPipeline -->|"cross-account ssm:PutParameter"| paramEnvs
    rcPipeline -->|"local DynamoDB write"| boundaryTargets
    mcPipeline -->|"cross-account via zoa-data-access role"| boundaryTargets
```

#### 3. Network topology per target VPC

Shows the network paths from the ZOA Boundary container. The container lives in a private subnet alongside the per-VPC Lambda. Function URL traffic goes through NAT Gateway (Function URLs are public HTTPS endpoints even though the Lambda is VPC-attached — VPC attachment only affects the Lambda's outbound network, not its invocation endpoint). EKS API access is direct (private endpoint, same VPC) and reserved for future break-glass use only.

```mermaid
graph TD
    subgraph targetVPC [Target VPC]
        subgraph privateSubnets [Private Subnets]
            boundary["ZOA Boundary<br/>ECS Fargate Task"]
            lambdaENI["Per-VPC Lambda<br/>(VPC-attached ENI)"]
            eksPrivate["EKS API Server<br/>(private endpoint)"]
        end
        subgraph publicSubnets [Public Subnets]
            nat["NAT Gateway"]
        end
    end

    funcURL["Lambda Function URL<br/>(public HTTPS endpoint<br/>*.lambda-url.region.on.aws)"]
    awsServices["AWS Services<br/>(ECR, CloudWatch, SSM, S3)"]

    boundary -->|"zoa CLI: SigV4 HTTP<br/>via NAT (public endpoint)"| nat
    nat --> funcURL
    funcURL -->|"invocation plane"| lambdaENI

    lambdaENI -->|"execution plane<br/>(private, same VPC)"| eksPrivate

    boundary -.->|"break-glass only (future)<br/>direct private DNS"| eksPrivate

    boundary -->|"SSM, CW Logs, ECR<br/>via NAT"| nat
    nat --> awsServices
```

#### 4. Identity bridge flow

Shows how SRE identity is preserved across the authentication domain boundary. The SRE authenticates to the Central Account with their personal identity (kinit → Kerberos → SAML → IAM role with session name). The ZOA Access Lambda records this identity when creating the ECS task. Inside the container, all requests use the shared ECS task role — the per-VPC Lambda resolves the task ARN back to the originating SRE via DynamoDB lookup. This ensures every TA execution is attributed to the correct SRE, even though the container uses a shared role.

```mermaid
sequenceDiagram
    participant SRE as SRE (slopezma)
    participant JA as Central Account IAM
    participant AL as ZOA Access Lambda
    participant DDB as DynamoDB boundary-sessions
    participant ECS as ECS Task (shared role)
    participant VPCLambda as Per-VPC Lambda
    participant ExecDDB as DynamoDB executions

    SRE->>JA: kinit slopezma@REDHAT.COM
    JA-->>SRE: IAM role: assumed-role/sre-role/slopezma

    SRE->>AL: POST /api/v0/sessions/start (SigV4)
    Note over AL: Extract from SigV4:<br/>ARN: ...assumed-role/sre-role/slopezma<br/>Session name: slopezma
    AL->>DDB: PUT {sessionId: task-abc, operator: slopezma, operatorARN: ...sre-role/slopezma}
    AL->>ECS: ecs:RunTask → task-abc starts

    Note over ECS: Container runs with shared role:<br/>arn:...assumed-role/zoa-boundary-task/task-abc

    ECS->>VPCLambda: zoa run get_resource (SigV4 with task role)
    Note over VPCLambda: Caller ARN: ...assumed-role/zoa-boundary-task/task-abc<br/>Extract task ID: task-abc
    VPCLambda->>DDB: GET sessionId=task-abc
    DDB-->>VPCLambda: {operator: slopezma}
    VPCLambda->>ExecDDB: PUT execution {operator: slopezma, ...}
    Note over VPCLambda: TA execution attributed to slopezma,<br/>not to the ECS task role
```

#### 5. Session lifecycle and reaper

Shows the full lifecycle of a boundary session from creation through termination, including the reaper safety net. The reaper runs on the per-VPC Worker Lambda (same EventBridge infrastructure as the existing reconciler/GC) and enforces the 4h hard deadline. No workspace sync on exit — all audit data is captured in real-time via SSM session logging and structured command audit (no EFS, no S3 workspace escrow).

```mermaid
stateDiagram-v2
    [*] --> creating: zoa session start
    creating --> active: ECS task RUNNING
    creating --> failed: ECS task failed to start

    active --> terminated: zoa session terminate
    active --> terminated: reaper (4h deadline)

    failed --> [*]
    terminated --> [*]

    note right of active
        SRE can join/disconnect/rejoin
        Session state persists in container
        SSM records all terminal I/O
        (Structured command layer — auditd/PROMPT_COMMAND — not in initial image)
    end note
    note right of terminated
        Reasons: sre_exit / deadline_exceeded / reaper / error
        DynamoDB updated and metric emitted
    end note
```

## Jira Content

### Epic: ROSAENG-60291

#### Jira Fields

**Title**: ZOA Boundary: Audited SRE Access Containers + ZOA Access Lambda

**TL;DR**: Today SREs call per-VPC Lambda Function URLs directly from their laptop — a temporary bootstrapping path that bypasses session auditing, network isolation, and the identity bridge needed for FedRAMP compliance. The tool meant to enforce zero operator access has no record of what the SRE does between TA executions, no time-boxing, and no way to attribute actions to a specific individual when multiple SREs share the same IAM role.

This epic delivers the target ZOA access model: SREs authenticate via their AWS Central Account, autodiscover available deployments/targets via SSM Parameter Store, assume a central-trusted invoker role in the RC account, and create time-boxed ECS Fargate containers ("ZOA Boundary") placed inside target VPCs. All TA execution happens exclusively from within these containers. The ZOA Access Lambda (Function URL with IAM auth, no VPC attachment) handles session lifecycle, vends per-task scoped credentials for ECS Exec isolation, and routes approval requests. Session recording ships **SSM terminal I/O (CloudWatch)** plus **DynamoDB application audit**; structured command capture (auditd or PROMPT_COMMAND) is a follow-on layer documented separately. The tamper-proof identity bridge resolves every TA execution back to the originating SRE via SigV4 task UUID → DynamoDB session lookup — no ABAC required. Both the resolved operator and the raw SigV4 signer ARN are stored for forensic completeness, and every operation is linked to its originating boundary session via session ID. Sessions are time-boxed (4h default) with automatic reaper enforcement.

**What will be delivered:**

- ZOA Access Lambda (Go, no VPC) with Function URL (IAM auth) + central-trusted invoker role per region
- ZOA Boundary container image (`Containerfile.boundary`) with zoa CLI, aws CLI v2, kubectl, jq, Claude Code (Bedrock)
- ZOA CLI commands for discovery (`zoa deployments`, `zoa targets <deployment>`) and session management (`zoa session start/stop/join/list/history`) with compound session IDs
- SSM Parameter Store autodiscovery in Central Account (deployments, Function URLs, invoker role ARNs)
- Tamper-proof identity bridge: SigV4 task UUID → `task-id-index` GSI → SRE username + session ID (no ABAC, scoped credentials model, no client-supplied headers)
- DynamoDB `boundary-sessions` table for session state tracking (in `zoa/` module, consolidated storage, GSIs: `operator-index`, `status-deadline-index`, `date-bucket-index`, `task-id-index`)
- SSM Parameter Store for target registration (Terraform-managed lifecycle, no DynamoDB)
- ECS task tags (tamper-proof): `sre`, `sessionId`, `deployment`, `target` — second independent SRE attribution path
- Bedrock integration: regional-only IAM (no cross-region inference), model invocation logging (metadata only, no payloads)
- Boundary session reaper (EventBridge-triggered on Worker Lambda, 4h timeout)
- Terraform modules: `zoa-access` (Lambda + Function URL + invoker role), `zoa-boundary` (ECS task definition, IAM, SG)
- Konflux pipeline for ZOA Boundary container image (Enterprise Contract)
- Per-region pipeline step to publish metadata to Central Account SSM Parameter Store
- Full observability stack: EMF metrics, YACE scrape, alerting rules, recording rules, Grafana dashboard
- E2E testing for boundary session lifecycle, identity bridge, reaper
- Documentation: architecture, CLI reference, SRE runbook
- Approval stub routes (`/approve/{id}`, `/reject/{id}`) on both Access and API Lambda — logic deferred to approval workflow epic
- Architectural readiness for future break-glass (kubectl/aws CLI installed, EKS SG egress open, `breakglass_role_arns` variable reserved)

**Acceptance Criteria**:

| #   | Criterion                                                                                                                                                                                                                                                |
| --- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | SRE can run `zoa deployments` from laptop and see all available deployments (from SSM `/zoa/deployments`)                                                                                                                                                |
| 2   | SRE can run `zoa targets <deployment>` and see all targets (rc, mc01, mc02) within that deployment                                                                                                                                                       |
| 3   | SRE can run `zoa session start <deployment> <target>` and get an interactive shell inside a boundary container in the target VPC. Session ID returned as compound `deployment/session-id`.                                                               |
| 4   | SRE can execute TAs (`zoa run`) from inside the boundary container against the target cluster                                                                                                                                                            |
| 5   | Every TA execution from a boundary container is attributed to the originating SRE — tamper-proof identity bridge (SigV4 task UUID → DynamoDB → SRE username). Both resolved `operator` and raw `signerARN` stored in execution and audit records.        |
| 6   | Session ID resolved server-side from the identity bridge (task ARN → sessions table → session ID) and stored in all TA execution and audit entries — complete audit chain from session to every operation, no client-supplied headers trusted            |
| 7   | Sessions are time-boxed (4h default) and auto-terminated by the reaper                                                                                                                                                                                   |
| 8   | SSM session logging captures full terminal I/O to CloudWatch Logs (KMS-encrypted)                                                                                                                                                                        |
| 9   | `zoa session list <deployment>` shows **the caller's** sessions (last 24h, default `--status all`). `zoa session history <deployment>` shows **all operators** for situational awareness / audit. `stop` and `join` enforce ownership (server-side 403). |
| 10  | `zoa audit` shows unified audit trail across TA executions and session lifecycle events                                                                                                                                                                  |
| 11  | All infrastructure is Terraform-managed (`zoa-access`, `zoa-boundary` modules) and GitOps-deployed                                                                                                                                                       |
| 12  | Boundary container image is Konflux-built with Enterprise Contract                                                                                                                                                                                       |
| 13  | Observability stack covers Access Lambda and boundary sessions (metrics, alerts, dashboards)                                                                                                                                                             |
| 14  | E2E tests validate session lifecycle, identity bridge, reaper, and negative cases                                                                                                                                                                        |
| 15  | Architecture documentation, CLI reference, and SRE runbook are published                                                                                                                                                                                 |
| 16  | Approve/reject routes exist on both Access and API Lambda (stub `501` — approval workflow is a separate epic)                                                                                                                                            |
| 17  | Container and infra are prepared for future break-glass (kubectl, aws CLI installed; EKS SG egress open; `breakglass_role_arns` variable reserved; break-glass architecture documented)                                                                  |
| 18  | ECS task role IAM is correctly scoped: NO DynamoDB, NO Access Lambda invoke, NO ECS tag modification, NO IAM modification — only SSM messages, CloudWatch Logs, and per-VPC Lambda Function URL invoke                                                   |

---

## Child Issues (7 stories)

### 1. ZOA Boundary Core (rosa-hyperfleet-zoa)

#### Jira Fields

**Title**: ZOA Boundary Core — Access Lambda, boundary container, CLI, identity bridge, reaper

**Overview**: Complete ZOA Boundary Go implementation in `rosa-hyperfleet-zoa`. Adds `HANDLER_MODE=access` as a third Lambda handler mode for session lifecycle and target discovery. Builds the boundary container image with ZOA CLI and SRE tooling. Implements `zoa targets` (autodiscovery), `zoa session` (lifecycle), and `zoa audit` (unified trail). Implements the tamper-proof identity bridge — resolving ECS task ARN back to originating SRE via SigV4 task UUID → DynamoDB session lookup using `task-id-index` GSI (no ABAC, no client-supplied session headers). Session ID is derived server-side from the same lookup, eliminating any env-var-based injection that the SRE could tamper with. Adds a reaper for session timeout enforcement. Single integrated deliverable — all components must be developed and tested together.

**Scope**:

- Access Lambda handler mode (`HANDLER_MODE=access`) with session and target routes
- Boundary container image (`Containerfile.boundary`) — UBI9, zoa CLI, aws CLI v2, kubectl, jq, Claude Code (Bedrock). All binaries SHA256-verified. No curl/wget in final image.
- CLI commands: `zoa deployments`, `zoa targets <deployment>`, `zoa session start/stop/join/list/history` (compound session IDs), `zoa audit`, offline catalog (`zoa actions --offline`, `zoa catalog`, `ZOA_TARGET_TYPE`)
- Tamper-proof identity bridge: SigV4 task UUID → `task-id-index` GSI on sessions table → SRE operator + session ID. Dual-field storage: resolved `operator` + raw `signerARN` in executions and audit tables.
- Session ID resolved server-side: identity bridge lookup returns both operator and session ID from DynamoDB via `task-id-index` GSI (no env vars, no client-supplied headers — nothing the SRE can tamper with)
- `zoa session list` returns **only the signed-in operator's** sessions (`scope=mine`, default `--status all`, 24h window). `zoa session history` lists **all operators** (audit view) with `--operator`, `--since`, etc.
- Session reaper on Worker Lambda (EventBridge, 5m interval, `status-deadline-index` GSI query)
- SSM-backed target store (`GetParametersByPath`, Terraform-managed lifecycle)
- Approval stub routes (`/approve/{id}`, `/reject/{id}`) on both Access and API Lambda

**Acceptance Criteria**:

| #   | Criterion                                                                                                                                                                                                                                                                                                                             |
| --- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | `HANDLER_MODE=access` is a third Lambda handler mode (alongside `api` and `worker`) with routes for session management, target listing, and approval stubs                                                                                                                                                                            |
| 2   | `Containerfile.boundary` builds a UBI9 image with zoa CLI, aws CLI v2, kubectl, jq, Claude Code — minimal attack surface, all binaries SHA256-verified, no curl/wget in final image                                                                                                                                                   |
| 3   | `zoa deployments` lists deployments from SSM; `zoa targets <deployment>` lists targets from ZOA Access Lambda (SSM-backed store)                                                                                                                                                                                                      |
| 4   | `zoa session start/stop/join/list/history` manages boundary container lifecycle with SigV4 auth. Compound session IDs (`deployment/session-id`) for self-routing. **`list`** = your sessions; **`history`** = fleet-wide. `stop` and `join` enforce ownership. **`start`** connects via ECS Exec by default (`--no-connect` to skip). |
| 5   | `zoa audit` shows unified audit trail (TA executions + session lifecycle) with `--type` filter                                                                                                                                                                                                                                        |
| 6   | Identity bridge resolves ECS task ARN → SRE username via tamper-proof SigV4 task UUID → DynamoDB session lookup. Both `operator` and `signerARN` stored in execution and audit records.                                                                                                                                               |
| 7   | Session ID derived server-side from identity bridge (task ARN → `task-id-index` GSI → session record) — no client-supplied env vars or headers trusted for session linkage                                                                                                                                                            |
| 8   | Reaper (Worker Lambda scheduled task) terminates sessions past 4h deadline using `status-deadline-index` GSI                                                                                                                                                                                                                          |
| 9   | `zoa approve` / `zoa reject` routes return `501 Not Implemented` on both Access and API Lambda                                                                                                                                                                                                                                        |
| 10  | `zoa session terminate` and `zoa session join` enforce ownership (server-side 403 if caller != session.operator)                                                                                                                                                                                                                           |
| 11  | All new code has unit tests; conformance test updated for new handler mode                                                                                                                                                                                                                                                            |
| 12  | `make all` passes (verify → test → build)                                                                                                                                                                                                                                                                                             |

**Repos**: `rosa-hyperfleet-zoa`

#### Plan Details

#### Access Lambda — `HANDLER_MODE=access` (third mode)

The `zoa-lambda` container image serves all three Lambda roles. The `HANDLER_MODE` environment variable selects which routes are active:

| Mode     | Routes                                                                                                                                       | Caller                                              | Deployment                       |
| -------- | -------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------- | -------------------------------- |
| `access` | `/api/v0/sessions/start`, `/api/v0/sessions`, `/api/v0/sessions/terminate/{id}`, `/api/v0/targets`, `/api/v0/approve/{id}`, `/api/v0/reject/{id}` | Laptop (invoker role via Function URL)              | RC account, no VPC, 1 per region |
| `api`    | `/run`, `/runs`, `/actions`, `/audit`, `/version`, `/approve/{id}`, `/reject/{id}`                                                           | Boundary container (ECS task role via Function URL) | Per-VPC (RC + each MC)           |
| `worker` | EventBridge reconciler/GC/reaper events, self-invoke `execute` events                                                                        | EventBridge + Lambda self-invoke                    | Per-VPC (RC + each MC)           |

`/approve/{id}` and `/reject/{id}` on both `access` and `api` modes — approver can do it from laptop (ZOA Access) or from inside a boundary (per-VPC API). Routes return `501 Not Implemented` until the approval workflow epic ships.

Access Lambda handles:

- **Session lifecycle**: `POST /api/v0/sessions/start`, `GET /api/v0/sessions`, `POST /api/v0/sessions/terminate/{id}`
- **Target listing**: `GET /api/v0/targets` (reads SSM `/zoa/targets/<deployment>/` parameters, RC-local)
- **Placement routing**: resolve target cluster → VPC → Function URL from SSM target parameters
- **Cross-account session creation**: `sts:AssumeRole` into MC account to `ecs:RunTask` there
- **Identity recording**: map SigV4 caller (Central Account role) to SRE identity, write to `boundary-sessions` DynamoDB table
- **Future: Approval/rejection**: write `approved`/`rejected` status to DynamoDB (per-VPC reconciler handles activation)

Key design: Access Lambda does NOT create EKS access entries or execute TAs. Keeps IAM minimal.

**No API Gateway** — Access Lambda uses a Function URL with `AWS_IAM` auth type (one fewer failure domain, native streaming, SSM discoverability).

**Cross-account access uses a central-trusted invoker role**: only IAM role **names** listed in `aws.zoa_access_trusted_assumer_role_names` in the environment Central Account may assume the invoker role in RC. Today the default is `OrganizationAccountAccessRole` (dev jump). **Story 7** adds app-interface SAML hub roles (pattern: [osd-staging-2 rrp-admin](https://gitlab.cee.redhat.com/service/app-interface/-/blob/master/data/aws/osd-staging-2/roles/rrp-admin.yml)) for dev/int/stage. `mc_ou_path` is not used for Access or Boundary IAM.

Resource-based policy on Function URL: allows the invoker role to call `lambda:InvokeFunctionUrl`.

#### Boundary Container Image — `Containerfile.boundary`

Purpose-built container for HyperFleet ZOA. Built from within the `rosa-hyperfleet-zoa` repo alongside the Lambda and runner images — same build pipeline, same base image (UBI9), same release cycle. This ensures the boundary container always ships a zoa CLI binary that matches the Lambda it talks to, avoiding version skew between CLI and API. The tooling set is tailored to HyperFleet's serverless architecture (Lambda Function URLs, SigV4 auth, EKS-only targets) rather than the OCM/Backplane ecosystem.

**Pre-installed tooling (minimum attack surface — every binary is auditable, all SHA256-verified):**

- `zoa` CLI (built from same repo, version-matched to Lambda)
- AWS CLI v2
- `kubectl` (EKS native — no `oc` needed for EKS clusters)
- `jq` (JSON processing)
- `tar`, `gzip` (archive extraction)
- `vim-minimal` (basic editing)
- `procps-ng` (ps — process debugging)
- `bind-utils` (dig/nslookup — VPC DNS troubleshooting)
- `openssl` (certificate debugging)
- Claude Code (Amazon Bedrock integration, SHA256-verified from GitHub Releases)
- `bash-completion` (tab completion for kubectl, aws, zoa)
- Runtime TA catalog for agents: entrypoint runs `zoa actions --offline -o markdown` → `~/.claude/ZOA_ACTIONS.md` (not baked static `ZOA_ACTIONS.rc.md` files)
- Structured command audit (**planned** — see [boundary-session-logging.md](../design/boundary-session-logging.md); not in the initial boundary image)

**Deliberately excluded** (minimal attack surface — additional tools can be added via Containerfile PR if needed):

- `curl`, `wget` — prevents downloading arbitrary binaries into the container. All binaries are COPYed from builder stages.
- `oc` — this is an EKS container, kubectl is the native client. Must-gather runs as a TA (K8s Job with `hypershift dump cluster`), not via `oc adm must-gather`.
- `yq` — `jq` covers JSON needs; ZOA CLI has `-o json` output. Break-glass is the exception, not the rule; if needed, it's a one-line Containerfile change.
- `helm`, `k9s`, `stern` — `kubectl` covers the same ground; add if SREs request
- `git` — nothing to clone inside a boundary session
- `terraform`, `skopeo`, `python3` — pipeline/build tools, not SRE operations
- `tmux` — single-session container, no multiplexing needed

**Session recording (SSM + application audit shipped; structured commands planned):**

Based on the [original architecture design](https://gist.github.com/slopezz/ffdadd0d26167710b4f92b3b65d04488#session-recording-three-layers). **As of 2026-10-06**, Layers 1 and 3 are implemented; Layer 2 is design-only ([boundary-session-logging.md](../design/boundary-session-logging.md)).

| Layer                                                  | Mechanism                                                                                     | What it captures                                                         | Where it goes                                                       | Queryable?                                           | Status      |
| ------------------------------------------------------ | --------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------ | ------------------------------------------------------------------- | ---------------------------------------------------- | ----------- |
| **1. SSM session logging**                             | Built-in ECS Exec → SSM Agent                                                                 | Full terminal I/O (every character typed AND displayed — input + output) | CloudWatch Logs (`/ecs/zoa-boundary/ssm-sessions`), KMS-encrypted   | CW Logs Insights (raw text search). Forensic replay. | **Shipped** |
| **2. auditd** (preferred) or PROMPT_COMMAND (fallback) | `auditd` kernel-level `execve` interception, targeted rules for `kubectl`, `zoa`, `aws`, `oc` | Per-binary execution: which command was invoked, arguments, exit code    | CloudWatch Logs (`/ecs/zoa-boundary/commands`) via CloudWatch agent | Yes — structured fields, SQL-like queries            | **Planned** |
| **3. ZOA DynamoDB audit**                              | Application-level (already exists)                                                            | TA executions, session start/stop/join, approvals                        | DynamoDB `audit` table (existing)                                   | Yes — `zoa audit` CLI                                | **Shipped** |

**Layer 1 (SSM)** is zero-effort — configure `execute_command_configuration` on the ECS cluster with a CW Logs log group and KMS key (standard ECS Exec pattern). Every `ecs execute-command` session streams terminal I/O in real-time. This is the **immutable FedRAMP baseline** — complete evidence of everything that happened, including all command output.

**Layer 2 (auditd — preferred)** intercepts `execve` syscalls at the kernel level. Targeted rules capture only security-relevant binaries:

```bash
-a always,exit -F arch=b64 -S execve -F path=/usr/local/bin/kubectl -k rosa-cmd
-a always,exit -F arch=b64 -S execve -F path=/usr/local/bin/zoa -k rosa-cmd
-a always,exit -F arch=b64 -S execve -F path=/usr/local/bin/aws -k rosa-cmd
-a always,exit -F arch=b64 -S execve -F path=/usr/local/bin/oc -k rosa-cmd
```

CloudWatch agent streams these to a separate log group for structured queries. Key advantage over PROMPT_COMMAND: auditd catches commands from scripts, pipes, and sub-processes — not just what the SRE types at the prompt. Much harder to evade (kernel-level vs bash-level).

**⚠️ Fargate validation required**: `auditd` needs `CAP_AUDIT_WRITE` (usually available on Fargate) and `CAP_AUDIT_CONTROL` (for rule configuration — **may be restricted**). Must validate early in implementation. If not available, fall back to PROMPT_COMMAND (Layer 2 fallback).

**Fallback (PROMPT_COMMAND)**: If auditd is blocked on Fargate, a bash profile script (`/etc/profile.d/zoa-audit.sh`) sets `PROMPT_COMMAND` to emit a JSON record after every command. Covers top-level shell commands only (not sub-processes). SRE can `unset PROMPT_COMMAND` — but SSM (Layer 1) records the `unset` itself, and the gap is detectable.

**Why both SSM and auditd?** SSM captures what the SRE **saw** (input + output = forensic replay). auditd captures what the SRE **did** (structured, searchable, no output noise). One answers "show me the exact terminal at 14:32", the other answers "list all kubectl commands slopezma ran today."

**Layer 3 (ZOA DynamoDB)** already exists — every `zoa run`, `zoa session start/stop/join`, and future `zoa approve/reject` writes to the `audit` DynamoDB table. This is the application-level trail.

**Future RH compliance integration**: All three layers' data can be exported to S3 (CW Logs via subscription filter, DynamoDB via export or stream). From S3, data can feed into Red Hat compliance tooling (RHACS, Splunk, or other SIEM) as requirements crystallize.

**Excluded from bastion image (not needed for SRE operations):**

- `terraform` (infra provisioning belongs in pipelines, not SRE shells)
- `skopeo` (image mirroring)
- `python3 + boto3` (aws CLI covers AWS operations)
- `postgresql`/`psql` (RDS access should go through TAs, not direct psql)

**Runtime properties:**

- Non-root `sre` user (uid=1000)
- `ZOA_ENDPOINT` env var injected at task creation (per-VPC Lambda Function URL)
- `ZOA_TARGET` env var (target identifier: rc, mc01, etc.)
- SSM Session Manager ready (ECS Exec support)
- Time-boxed: 4h hard deadline (container exits, not extendable)
- SSM session logging to CloudWatch Logs (KMS-encrypted, real-time terminal I/O — forensic replay)
- Structured command audit: **deferred** (auditd or PROMPT_COMMAND — see session recording section and [boundary-session-logging.md](../design/boundary-session-logging.md))
- Network-isolated: only reaches Lambda Function URLs (via NAT) and EKS API (same VPC, for break-glass only)

Base image: UBI9 (consistent with zoa-lambda and zoa-runner).

**Break-glass readiness** (no EKS access today, but prepared for future — see [Break-Glass Architecture](#break-glass-architecture--detailed-design-for-future-epic) for full details):

- `~/.kube/` and `~/.aws/` directories are writable (created by `useradd`). Start empty — populated by `zoa breakglass connect`.
- Reserved env var `ZOA_BREAKGLASS_ROLE_ARN` (empty by default — break-glass epic injects per-scope role ARN via RunTask overrides)
- EKS API reachable from container (same VPC, SG allows 443 to EKS) — but no EKS Access Entry exists for the task role
- `kubectl` and `aws eks get-token` installed — zero-credential kubeconfig pattern (exec plugin generates 15-min tokens from ECS task role, auto-refreshed)
- PS1 prompt shows `[sre@zoa:us-east-1/mc01]` — extensible to show break-glass scope

#### CLI Commands

**Full CLI hierarchy** (grouped in `--help` output, TA commands stay top-level for shortest typing):

```
$ zoa --help

ZOA — Zero Operator Access CLI

Trusted Actions (inside session — ZOA_API_URL auto-set):
  run          Execute a Trusted Action
  runs         List recent executions
  get          Get execution details
  output       Show execution output
  logs         Show execution logs
  download     Download output file
  actions      List available Trusted Actions
  describe     Show TA details

Discovery (from laptop — reads SSM / ZOA Access):
  deployments  List available ZOA deployments
  targets      List targets within a deployment

Sessions (from laptop — manages boundary containers):
  session      Manage sessions (start, stop, join, list, history)

Break-Glass (future):
  breakglass   Emergency direct access (request, connect, revoke, list)

Approval:
  approve      Approve a request
  reject       Reject a request

Audit:
  audit        View audit trail (all event types)

Meta:
  version      Print version info
  completion   Generate shell completions
```

**Discovery — `zoa deployments` + `zoa targets`** (not audit-logged):

Discovery is split into two commands matching the two-layer architecture: deployments (SSM direct, Central Account) and targets (Access Lambda, invoker role):

| Command                    | Purpose                                                      | Endpoint                                      |
| -------------------------- | ------------------------------------------------------------ | --------------------------------------------- |
| `zoa deployments`          | List ZOA installations (deployment_name, region, Access URL) | SSM (direct read, Central Account)            |
| `zoa targets <deployment>` | List targets in deployment (rc, mc01, mc02)                  | Access Lambda Function URL (via invoker role) |

Example output:

```
$ zoa deployments
DEPLOYMENT                REGION      ACCESS URL                                           INVOKER ROLE
us-east-1                 us-east-1   https://abc123.lambda-url.us-east-1.on.aws/          arn:aws:iam::599476212575:role/us-east-1-zoa-access-invoker
us-east-1-eph-f8d5483c    us-east-1   https://def456.lambda-url.us-east-1.on.aws/          arn:aws:iam::599476212575:role/us-east-1-eph-f8d5483c-zoa-access-invoker

$ zoa targets us-east-1
TARGET    TYPE    REGION      VPC              STATUS
rc        RC      us-east-1   vpc-0abc123...   ready
mc01      MC      us-east-1   vpc-0def456...   ready
mc02      MC      us-east-1   vpc-0ghi789...   ready
```

**Why two commands**: "deployment" and "target" are different concepts. A deployment is a regional ZOA installation (SSM entry, invoker role, Access Lambda). A target is an EKS cluster within that deployment. Overloading a single command with two meanings creates confusion — especially when multiple deployments share the same region (ephemeral, future canary).

`deployment_name` (the positional arg) maps directly to the internal config variable `deployment_name` — equals `aws_region` for normal deployments (e.g., `us-east-1`), `aws_region-eph_prefix` for ephemeral (e.g., `us-east-1-eph-f8d5483c`). The REGION column shows the actual AWS region, which matters when deployment_name ≠ region.

**Session lifecycle — `zoa session`** (audit-logged where noted):

| Command                                    | Purpose                                              | Endpoint                     | Audit logged |
| ------------------------------------------ | ---------------------------------------------------- | ---------------------------- | ------------ |
| `zoa session start <deployment> <target>`  | Create ECS task, wait RUNNING                        | Access Lambda (invoker role) | **Yes**      |
| `zoa session terminate <deployment/session-id>` | Terminate session (immediate `ecs:StopTask`)              | Access Lambda (invoker role) | **Yes**      |
| `zoa session join <deployment/session-id>` | Reconnect via SSM                                    | Access Lambda (invoker role) | **Yes**      |
| `zoa session list <deployment>`            | **Your sessions** (last 24h; default `--status all`) | Access Lambda (invoker role) | No           |
| `zoa session history <deployment>`         | **All operators** (audit / situational awareness)    | Access Lambda (invoker role) | No           |

**Compound session IDs**: Session IDs include the deployment prefix (`deployment/session-id`, e.g. `us-east-1/sess-abc123`). This embeds the routing key directly in the ID so `stop` and `join` auto-resolve which Access Lambda to talk to — no separate `--deployment` flag needed. The compound ID is displayed by `session start`, `session list`, and `session history`.

Positional args for `start`: `<deployment>` = `deployment_name`, `<target>` = target ID (rc, mc01). Also available as flags for scripts: `zoa session start -d us-east-1 -t mc01`.

Example workflow:

```bash
# Discover what's available
$ zoa deployments
DEPLOYMENT                REGION
us-east-1                 us-east-1
us-east-1-eph-f8d5483c    us-east-1

# See targets in a deployment
$ zoa targets us-east-1
TARGET    TYPE    REGION      STATUS
rc        RC      us-east-1   ready
mc01      MC      us-east-1   ready

# Start a session (compound ID returned)
$ zoa session start us-east-1 mc01
Session started: us-east-1/sess-abc123
Status: active

# List your sessions (compound IDs in output)
$ zoa session list us-east-1
SESSION ID                     TARGET  STATUS   CREATED   DEADLINE
us-east-1/sess-abc123          mc01    active   2m ago    3h58m

# Fleet-wide audit view
$ zoa session history us-east-1 --status active

# Join (compound ID has the routing — no deployment flag needed)
$ zoa session join us-east-1/sess-abc123

# Stop (same — compound ID is self-routing)
$ zoa session terminate us-east-1/sess-abc123
```

**Approval commands** (top-level — approver should NOT need to create a session just to approve):

| Command                          | Purpose                                        | Endpoint                     | Audit logged |
| -------------------------------- | ---------------------------------------------- | ---------------------------- | ------------ |
| `zoa approve <id>`               | Approve (stub for now — `501 Not Implemented`) | Access Lambda (invoker role) | **Yes**      |
| `zoa reject <id> --reason "..."` | Reject (stub for now)                          | Access Lambda (invoker role) | **Yes**      |

**Unified audit** — `zoa audit` covers ALL event types (TA executions + session lifecycle + future break-glass). Filter by `--type ta|session|breakglass` to narrow scope. Same filter flags as `zoa runs` (`--since`, `--until`, `--operator`, `--status`, `--limit`, `-o json`).

**Audit logging policy**: The Access Lambda writes audit entries to the same `audit` DynamoDB table used for TA executions for `start`, `stop`, `join`, `approve`, `reject`. Discovery (`targets`) and read-only queries (`list`, `history`) are NOT audit-logged — they have no side effects and no sensitive data.

**Context auto-detection** — the CLI auto-detects where the SRE is:

- `ZOA_API_URL` set → inside a session (ECS container) → TA commands work directly, discovery/session commands not needed
- `ZOA_API_URL` not set → on laptop → discovery and session commands resolve Access Lambda Function URL + invoker role from SSM using positional args

The deployment context is always provided per-command (positional arg or compound ID), not as global state. This avoids hidden configuration that could route commands to the wrong deployment.

`zoa approve` and `zoa reject` are top-level commands (not under `zoa session`) because approval should be frictionless — approver just needs `kinit` → `rh-aws-saml-login` → `zoa approve ID`. Routes exist on both Access and API Lambda but return `501 Not Implemented` until the approval workflow epic ships.

**CLI naming rationale:**

- **`deployments`** (not `environments`): "environment" already means dev/int/stage/prod in the project vocabulary. The SRE selects their environment by authenticating (AWS profile / Central Account). `deployments` matches the SSM path (`/zoa/deployments`), the Terraform variable (`deployment_name`), and the concept — a single ZOA installation. Multiple deployments can coexist in the same region (ephemeral, future canary).
- **`targets`** (separate from `deployments`): Targets are EKS clusters within a deployment. Overloading one command for both concepts creates confusion. `zoa deployments` answers "where can I go?", `zoa targets <dep>` answers "what's in this deployment?".
- **`session`** (not `boundary`): "Boundary" is internal project jargon. SREs understand "session" universally (SSH, SSM, tmux). Also avoids tab-completion collision with `breakglass` (both start with `b`).
- **`session history`** (not `session sessions`): Avoids the awkward noun repetition that `boundary sessions` would have.
- **Compound session IDs** (`deployment/session-id`): Follows the Google resource-name pattern (`projects/X/instances/Y`). Embeds routing info so `stop` and `join` are self-contained — no `--deployment` flag needed on every command. The CLI parses the deployment prefix and auto-resolves the correct Access Lambda.
- **Positional args** for `session start`: `zoa session start us-east-1 mc01` reads like English and saves 16 characters vs `--deployment us-east-1 --target mc01`. Flags (`-d`, `-t`) available for scripts.
- **TA commands stay top-level**: `zoa run` is 80%+ of CLI usage (inside sessions). No breaking change. Grouped visually in `--help` but flat in command path.

**Design decisions:**

- `zoa session start` **connects via ECS Exec by default** after the task is `active` (same path as `zoa session join`). Use `--no-connect` to print metadata only (JSON output never auto-connects).
- `zoa session start` flags: `--no-connect`, `--no-wait`, `--timeout` (default 4h), `-d`/`-t` (flag alternatives to positional args for scripts)
- **Compound session IDs**: `deployment/session-id` format (e.g. `us-east-1/sess-abc123`). The CLI constructs the compound ID from the deployment name and the raw session ID returned by the Access Lambda. On `stop`/`join`, the CLI parses the compound ID to extract the deployment (for routing) and the raw ID (for the API call). This follows the Google resource-name pattern and eliminates the need for `--deployment` flags on every command.

**Prerequisite**: `session-manager-plugin` must be installed on the SRE's laptop for `session start` (default connect) and `session join`. Install: `brew install --cask session-manager-plugin` (macOS) or RPM (Linux). The CLI detects absence and prints install instructions.

**Ownership and listing rules:**

| Command                            | Visibility                                 | Ownership enforcement                                     |
| ---------------------------------- | ------------------------------------------ | --------------------------------------------------------- |
| `zoa session list <deployment>`    | **Caller only** (`scope=mine`, 24h window) | None                                                      |
| `zoa session history <deployment>` | **All operators**                          | None — situational awareness / audit                      |
| `zoa session terminate <deployment/id>` | Own sessions only                          | Server-side: Access Lambda validates `operator == caller` |
| `zoa session join <deployment/id>` | Own sessions only                          | Server-side: Access Lambda validates `operator == caller` |

`zoa session list` supports `--status`, `--target`. Default status filter: **`all`** (within 24h). `zoa session history` adds `--since`, `--until`, `--operator`, `--target`, `--status` for audit-style queries.

**SRE identity across re-authentication:**

`rh-aws-saml-login` produces temporary STS credentials with a session name derived from the SRE's Kerberos principal (e.g., `slopezma`). Each re-authentication produces **different credentials** (new access key, secret key, session token) but the **session name is stable** because it comes from the Kerberos identity.

The SigV4 ARN looks like: `arn:aws:sts::123:assumed-role/sre-role/slopezma`

- `sre-role` — the shared IAM role name (stable, same for all SREs)
- `slopezma` — the session name from SAML (stable per SRE, derived from Kerberos principal)

**Critical design rule**: the `operator` field in `boundary-sessions` DynamoDB must store the **username extracted from the SigV4 session name** (e.g., `slopezma`), NOT the full temporary credential ARN. Ownership checks compare `operator == caller_session_name`. This way, an SRE who re-authenticates (gets new temporary credentials) can still join/stop their own sessions.

**Validated (2026-09):** `rh-aws-saml-login` uses the Kerberos-backed STS session name as the IAM role session suffix (e.g. `811685182089-rrp-admin/slopezma` → session name `slopezma`). ZOA Access stores **operator** from that session name for ownership checks across re-authentication. Hub roles are environment-specific app-interface roles (see Story 7), not only `OrganizationAccountAccessRole`.

#### Identity Bridge — ECS Task ARN to SRE Identity

Inside a ZOA Boundary container, SigV4 requests are signed with the ECS task role (not the SRE's personal role). The per-VPC Lambda must resolve the ECS task identity back to the SRE who created the session.

**Flow:**

1. ZOA Access Lambda creates ECS task, records `{taskId, taskArn, operator, region, target, createdAt, deadline}` in `boundary-sessions` DynamoDB table
2. Inside container, `zoa` CLI calls per-VPC Function URL with SigV4 (task role)
3. Per-VPC Lambda extracts task ID from caller ARN: `arn:aws:sts::ACCOUNT:assumed-role/zoa-boundary-task-role/TASK_ID`
4. Lambda queries `boundary-sessions` DynamoDB: task ID → SRE identity
5. All executions attributed to that SRE in the existing `executions` and `audit` tables (same `operator` field already in use)

**Current state**: Today the `Operator` field stores the full IAM ARN from SigV4 (e.g., `arn:aws:sts::123:assumed-role/sre-role/slopezma`). The session name portion already carries the SRE identity. With the boundary model, the ARN changes to the ECS task role, so the DynamoDB lookup becomes necessary.

**Identity stability across re-authentication**: The `operator` field must store the **username** (extracted from the SigV4 session name, e.g., `slopezma`), not the full temporary ARN. This ensures that an SRE who re-authenticates to the Central Account (gets new temporary credentials) can still be matched to their existing sessions and TA executions. The full ARN is stored separately as `operatorARN` for audit/forensic purposes.

**Ownership enforcement**: The per-VPC Lambda and ZOA Access Lambda both compare `operator == caller_session_name` for ownership checks (stop, join). Fleet-wide visibility uses **`zoa session history`**, not `list`.

#### Boundary Session Reaper

Extend the existing per-VPC Worker Lambda with a `reaper` scheduled task (EventBridge, every 5m) that terminates expired boundary containers:

1. Query `boundary-sessions` DynamoDB: `targetCluster = MY_TARGET AND status = active AND deadline < now`
2. For each expired session: `ecs:StopTask` (local, same account)
3. Update DynamoDB: `status=terminated, reason=deadline_exceeded`
4. Emit `ZOA/ReaperTerminations` CloudWatch metric

**Design:**

- 4h hard deadline, not extendable (new container = fresh audit trail)
- Reaper runs on the same per-VPC Worker Lambda (already has EventBridge schedules for reconciler/GC)
- IAM: Worker Lambda role needs `ecs:StopTask` + `ecs:DescribeTasks` for local ECS cluster
- Future enhancement: inactivity-based early termination (query auditd log for last command timestamp, or CloudWatch Logs for last SSM event)

#### DynamoDB Types

New Go types in `pkg/store/` for `boundary-sessions` table:

**Schema:**

- PK: `sessionId` (ECS task ID)
- Attributes: `operator`, `operatorARN`, `targetCluster`, `region`, `taskArn`, `ecsCluster`, `status`, `createdAt`, `deadline`, `terminatedAt`, `terminationReason`, `vpcId`
- GSI: `operator-index` (PK=operator, SK=createdAt) — for `zoa session list` filtering by SRE
- TTL: 30 days (session metadata, not long-term audit — the audit table already covers FedRAMP)
- Optional break-glass fields: `breakglassScope`, `breakglassStatus`, `breakglassExpiresAt` (NULL until break-glass epic)

**Session statuses:**

- `creating` — ECS RunTask called, waiting for RUNNING
- `active` — container is RUNNING, SRE can join
- `terminated` — container stopped (reason: sre_exit / deadline_exceeded / reaper / error)
- `failed` — ECS task failed to start

Go interfaces: `Session` struct, `SessionStore` interface with `Put`, `Get`, `List`, `UpdateStatus`.

**Repo**: `rosa-hyperfleet-zoa`

---

### 2. ZOA Boundary Infrastructure (rosa-hyperfleet)

#### Jira Fields

**Title**: ZOA Boundary Infrastructure — Terraform modules, SSM autodiscovery, DynamoDB, IAM

**Overview**: Implement the AWS infrastructure layer for the ZOA Boundary using Terraform modules, deploying the Access Lambda with Function URL and central-trusted invoker role, ECS Fargate task definitions for boundary containers, DynamoDB sessions table, SSM autodiscovery in the Central Account, and cross-account IAM wiring for MC sessions. Includes Bedrock integration (regional-only IAM, model invocation logging) and tamper-proof ECS task tags for SRE attribution. Worked in parallel with Story 1 — both needed to test anything end-to-end.

**Scope**:

- Terraform module `zoa-access`: Access Lambda + Function URL (IAM auth) + central-trusted invoker role + KMS-encrypted CloudWatch Logs
- Terraform module `zoa-boundary`: ECS task definition (Fargate) + IAM roles + security group + CloudWatch Logs (KMS) + KMS key
- DynamoDB table: `boundary-sessions` in `zoa/` module (GSIs: `operator-index`, `status-deadline-index`, `date-bucket-index`, `task-id-index`; TTL: 30d). Target registration via SSM Parameter Store (Terraform-managed lifecycle).
- ECS task tags: Access Lambda sets tamper-proof tags (`sre`, `sessionId`, `deployment`, `target`) on every ECS task at creation — no `ecs:TagResource` on task role
- SSM Parameter Store `/zoa/deployments` in Central Account (or RC account for dev/ephemeral)
- Cross-account IAM: Access Lambda `sts:AssumeRole` into MC for `ecs:RunTask`; MC boundary task role on MC Lambda resource policy
- Bedrock: classic **Invoke** in-region only — application inference profile sourced from the regional Haiku foundation model (no `us.anthropic.*` geo profiles). Task env `CLAUDE_CODE_USE_BEDROCK=1`. IAM `bedrock:InvokeModel` + scoped Marketplace subscribe.
- Bedrock model invocation logging: `aws_bedrock_model_invocation_logging_configuration` to CloudWatch Logs (metadata only — token counts, model ID, identity ARN. No payload capture, no S3).
- Worker Lambda IAM: `ecs:StopTask` + `ecs:DescribeTasks` + reaper EventBridge schedule
- Modified `zoa-lambda` module: SSM target self-registration, `SESSIONS_TABLE` env var, boundary module output wiring
- Modified `zoa/` module: `boundary-sessions` table always created, `data_access_ssm` policy on `data-access` role for MC cross-account SSM writes
- Boundary infrastructure always deployed (no feature flag) — data plane unconditional, compute gated by image tag only
- All timeouts and tunables exposed as Terraform variables

**Acceptance Criteria**:

| #   | Criterion                                                                                                                                                                                                                                        |
| --- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| 1   | `terraform/modules/zoa-access/` deploys: Lambda (`HANDLER_MODE=access`) + Function URL (IAM auth) + central-trusted invoker role + KMS-encrypted CloudWatch Logs                                                                                 |
| 2   | `terraform/modules/zoa-boundary/` deploys: ECS task definition, IAM task role (Function URL + Bedrock + SSM + CW Logs), security group, CW Logs log group (KMS), KMS key                                                                         |
| 3   | `boundary-sessions` DynamoDB table created in `zoa/` module with GSIs (`operator-index`, `status-deadline-index`, `date-bucket-index`, `task-id-index`) and TTL; targets use SSM Parameter Store                                                 |
| 4   | SSM `/zoa/deployments` parameter written to Central Account (or RC account for dev/ephemeral) with Function URL + invoker role ARN                                                                                                               |
| 5   | Cross-account IAM: Access Lambda can `ecs:RunTask` in MC accounts; MC boundary task role is permitted caller on MC Lambda Function URL; invoker role trusts Central Account hub roles only (`central_account_id` + `trusted_assumer_role_names`) |
| 6   | Bedrock Invoke scoped to deployment region; Haiku via application inference profile from in-region foundation model (`claude_bedrock_foundation_model_id`).                                                                                      |
| 7   | Bedrock model invocation logging enabled via `aws_bedrock_model_invocation_logging_configuration` — CloudWatch Logs only, no payload capture, no S3. Log group: `/aws/bedrock/model-invocations` (KMS-encrypted).                                |
| 8   | ECS task tags set by Access Lambda at `RunTask`: `Component`, `function` (`zoa`), `sre`, `sessionId`, `deployment`, `target`. Task role has NO `ecs:TagResource` permission (tamper-proof).                                                      |
| 9   | Worker Lambda has `ecs:StopTask` + `ecs:DescribeTasks` IAM and reaper EventBridge schedule                                                                                                                                                       |
| 10  | Invoker role trusts only the environment Central Account and configured assumer role names (not `mc_ou_path`). Central account ID comes from `data.aws_caller_identity.central` (same `aws.central` provider as SSM deployment writes).          |
| 11  | `terraform validate` and `terraform plan` pass; `make pre-push` passes                                                                                                                                                                           |
| 12  | Ephemeral environment deploys end-to-end (RC + MC)                                                                                                                                                                                               |

**Repos**: `rosa-hyperfleet`

#### Plan Details

**New module: `terraform/modules/zoa-access/`**

- Lambda function (no VPC, same `zoa-lambda` image, `HANDLER_MODE=access`)
- Function URL with `AWS_IAM` auth type (replaces API Gateway — one fewer service in path, supports response streaming, SSM handles discoverability)
- Central-trusted invoker role (`zoa-access-invoker`): trust policy allows `sts:AssumeRole` only from IAM roles listed in `trusted_assumer_role_names` in the environment Central Account (`central_account_id` from the same source as other cross-account central writes). Permissions: `lambda:InvokeFunctionUrl` on the Access Lambda only. Add future Red Hat SAML hub roles to config (`aws.zoa_access_trusted_assumer_role_names`); central roles still need `sts:AssumeRole` on `*-zoa-access-invoker` (narrow in app-interface when SAML lands).
- IAM execution role: `ecs:RunTask` (RC + cross-account MC), DynamoDB read/write (`boundary-sessions`), SSM `GetParametersByPath` (`/zoa/targets/`), `sts:AssumeRole`, CloudWatch Logs
- Lambda resource-based policy: allows invoker role to call Function URL

**New module: `terraform/modules/zoa-boundary/`**

- ECS task definition (Fargate, ZOA Boundary image from ECR)
- ECS cluster (or reuse existing)
- IAM task role: `lambda:InvokeFunctionUrl`, **Bedrock Invoke** (in-region Haiku app profile), `ssmmessages:*`, CloudWatch Logs, `kms:GenerateDataKey`/`kms:Decrypt` (for SSM session encryption)
- IAM task execution role: ECR pull, CloudWatch Logs
- Security group: egress to Function URL (443), EKS API (443, future break-glass), AWS services, Bedrock. No inbound.
- CloudWatch Logs log group for SSM session recording (`/ecs/zoa-boundary/ssm-sessions`), KMS-encrypted
- KMS key for ECS Exec session encryption and CloudWatch Logs
- Bedrock: `claude_bedrock_foundation_model_id` (default Haiku 4.5 FM), application inference profile per cluster, in-region only

**Bedrock access control (Claude Code in boundary container):**

Bedrock is **regional** — each region has its own endpoint and model catalog. This aligns with the per-region HyperFleet model: the ECS task role in `us-east-1` only permits Bedrock calls to `us-east-1`.

| Concern              | Design                                                                                                                            |
| -------------------- | --------------------------------------------------------------------------------------------------------------------------------- |
| Which models allowed | Terraform `claude_bedrock_foundation_model_id` (default `anthropic.claude-haiku-4-5-20251001-v1:0`). Haiku-only for cost control. |
| Regional scope       | Application inference profile with `copy_from` = in-region foundation model ARN only. No `us.`/`eu.`/`global.` system profiles.   |
| Cost control         | Haiku-only default keeps costs low. Override foundation model ID per region only if a different model is approved.                |
| Model availability   | Verify Haiku 4.5 + app profile creation in each HyperFleet region before rollout.                                                 |

Task env (Terraform): `CLAUDE_CODE_USE_BEDROCK=1`, `ANTHROPIC_MODEL` / `ANTHROPIC_DEFAULT_HAIKU_MODEL` = application inference profile ID.

**DynamoDB table in `terraform/modules/zoa/`:**

- `boundary-sessions` — PK: `sessionId`, GSIs: `operator-index` (PK: operator, SK: createdAt), `status-deadline-index` (PK: status, SK: deadline), `date-bucket-index` (PK: dateBucket, SK: createdAt), `task-id-index` (PK: taskId). TTL: 30 days.
- No DynamoDB for targets — target registration uses SSM Parameter Store (Terraform-managed lifecycle: write on apply, remove on destroy)

**Bedrock model invocation logging (account-level):**

- `aws_bedrock_model_invocation_logging_configuration` — CloudWatch Logs destination only
- Captures metadata per invocation: `identity.arn`, `modelId`, token counts, `requestId`
- Payload capture disabled: `text_data_delivery_enabled = false`, `image_data_delivery_enabled = false`
- No S3 destination — SSM session recording already captures the terminal conversation
- Log group: `/aws/bedrock/model-invocations` (KMS-encrypted)
- IAM role for Bedrock to write to CloudWatch Logs

**Boundary infrastructure is always deployed** — no `enable_zoa_boundary` or `enable_boundary` feature flag. The data plane (DynamoDB `boundary-sessions` table, `data_access_ssm` IAM policy, reaper schedule) is always created as part of the `zoa/` module. Compute (ECS tasks, Access Lambda) is gated only by whether the image tag is set (same pattern as `zoa_lambda` gating — empty image tag skips deployment, but data layer is always ready).

**Modified: `terraform/modules/zoa-lambda/`**

- `SESSIONS_TABLE` env var conditionally merged into Lambda environment (enables session identity bridge resolution when set)
- `aws_ssm_parameter.zoa_target`: each cluster self-registers with full target metadata (target_id, deployment_name, target_type, vpc_id, subnet_ids, security_group_id, ecs_cluster_arn, task_definition_arn, function_url, account_id, region, status) — auto-removed on `terraform destroy`
- Lambda resource-based policy: add ZOA Boundary task role as permitted caller
- Worker Lambda IAM: `ecs:StopTask` + `ecs:DescribeTasks` (reaper)
- Reaper EventBridge schedule on each Worker (RC + MC when `sessions_table_name` is set; same module as reconciler/GC)
- Boundary module outputs (security_group_id, ecs_cluster_arn, task_definition_arn) wired through to SSM target parameters

**Modified: `terraform/modules/zoa/`**

- `boundary-sessions` DynamoDB table always created (no count/conditional)
- `data_access_ssm` IAM policy on `data-access` role: `ssm:PutParameter`, `ssm:DeleteParameter`, `ssm:GetParameter`, `ssm:AddTagsToResource` on `/zoa/targets/*` — enables MC pipelines to write target metadata to RC account's SSM via cross-account role assumption

**Cross-account wiring:**

- ZOA Access Lambda `sts:AssumeRole` into MC account for `ecs:RunTask`
- MC boundary task role added to MC per-VPC Lambda resource policy
- MC pipelines write target metadata to RC SSM via `data-access` role (extended with SSM permissions)

#### SSM Parameter Store Autodiscovery (Central Account)

SREs already access a **Central Account** (one per environment: dev, int, stage) via app-interface. This story adds SSM Parameter Store for ZOA CLI autodiscovery. The Central Account is the right place because the CLI needs deployment pointers **before** contacting any ZOA service — SREs already have credentials here.

**Two-layer discovery architecture:**

| Data                                                                | Location                                               | Writer                                             | Reader                                  |
| ------------------------------------------------------------------- | ------------------------------------------------------ | -------------------------------------------------- | --------------------------------------- |
| Deployment list + Function URLs + invoker role ARNs                 | Central Account SSM (`/zoa/deployments`)               | RC Terraform (cross-account)                       | CLI directly                            |
| Target registry (rc, mc01, VPCs, Function URLs, subnets, task defs) | RC account SSM (`/zoa/targets/<deployment>/<cluster>`) | Each cluster's Terraform (auto-removed on destroy) | ZOA Access Lambda (local, same account) |

Central Account stays thin (just deployment pointers). All operational detail (VPCs, subnets, SGs, task role ARNs) stays in RC — the ZOA Access Lambda reads it locally without cross-account calls.

**Parameter layout (Central Account):**

- `/zoa/deployments` — JSON map keyed by `deployment_name` (unique per deployment within an environment):

```json
{
  "us-east-1": {
    "access_url": "https://abc123.lambda-url.us-east-1.on.aws/",
    "invoker_role_arn": "arn:aws:iam::RC_ACCOUNT:role/us-east-1-zoa-access-invoker",
    "deployment_name": "us-east-1",
    "region": "us-east-1",
    "enabled": true
  }
}
```

For ephemeral (dev Central Account), multiple entries coexist:

```json
{
  "us-east-1-eph-f8d5483c": {
    "access_url": "https://def456.lambda-url.us-east-1.on.aws/",
    "invoker_role_arn": "arn:aws:iam::RC_ACCOUNT:role/us-east-1-eph-f8d5483c-zoa-access-invoker",
    "deployment_name": "us-east-1-eph-f8d5483c",
    "region": "us-east-1",
    "enabled": true
  },
  "us-east-1-eph-ab12cd34": {
    "access_url": "https://ghi789.lambda-url.us-east-1.on.aws/",
    "invoker_role_arn": "arn:aws:iam::RC_ACCOUNT:role/us-east-1-eph-ab12cd34-zoa-access-invoker",
    "deployment_name": "us-east-1-eph-ab12cd34",
    "region": "us-east-1",
    "enabled": true
  }
}
```

- `deployment_name` is the unique identifier used throughout the platform (equals `aws_region` for normal deployments, `aws_region-eph_prefix` for ephemeral — see `config/defaults.yaml` and `config/ephemeral/defaults.yaml`)
- Written by each RC Terraform pipeline via cross-account `sts:AssumeRole`
- On environment teardown, Terraform removes the entry (critical for ephemeral lifecycle)

**Target registry (RC account, SSM Parameter Store):**

- Path: `/zoa/targets/<deployment_name>/<cluster_name>` (e.g., `/zoa/targets/us-east-1/mc01`)
- Value: JSON — `{"target_type": "mc", "vpc_id": "vpc-abc", "subnet_ids": "subnet-a,subnet-b", "function_url": "https://...", "account_id": "123456", "region": "us-east-1"}`
- Written by each cluster's Terraform (`zoa-lambda` module) — auto-removed on `terraform destroy` (no orphans, no GC)
- Read by ZOA Access Lambda via `GetParametersByPath` (local, same account, ambient creds)

**Why SSM instead of DynamoDB for targets**: Targets are static, Terraform-managed data that changes only when clusters are added/removed. SSM is simpler — Terraform manages the full lifecycle (write on apply, remove on destroy). DynamoDB is reserved for operational data with query patterns (executions, audit, sessions).

**Pipeline integration:**

- RC Terraform: writes `/zoa/deployments` entry to Central Account SSM (cross-account via `provider = aws.central`)
- Each cluster's Terraform: writes its own target entry to RC-account SSM (local `ssm:PutParameter`)
- Ephemeral teardown: `terraform destroy` auto-removes both deployment and target entries

**Cross-account IAM wiring (Central Account):**

- Central Account needs a "pipeline writer" IAM role that RC pipeline roles can assume
- Permission: `ssm:PutParameter` and `ssm:DeleteParameter` on `/zoa/deployments` only
- Trust policy: allow `sts:AssumeRole` from RC pipeline roles across all RC accounts in that environment

**Bootstrapping (dev/ephemeral):** For dev/ephemeral, the `/zoa/deployments` SSM parameter can live in the **RC account** instead of the Central Account. SREs already have RC credentials (`rrp-rc` profile) for dev/ephemeral work. The CLI reads SSM from whatever credentials are active — it doesn't care which account owns the parameter. The RC Terraform pipeline writes the parameter locally (no cross-account wiring needed). This means **Central Account cross-account wiring does NOT block stories 1+2 for dev/ephemeral development and testing**.

**Repo**: `rosa-hyperfleet`

---

### 3. Konflux Pipeline — ZOA Boundary Image

#### Jira Fields

**Title**: Konflux Pipeline — ZOA Boundary container image build and supply chain

**Overview**: Onboard the ZOA Boundary container image into the Konflux supply chain. Register `zoa-boundary` as a new Konflux Component alongside existing `zoa-lambda` and `zoa-runner` — same pattern, separate image. Includes Tekton pipelines, Enterprise Contract validation, Quay push, ECR skopeo mirror, and MintMaker config.

**Scope**:

- `zoa-boundary` Konflux Component with `ImagesRepository` and `IntegrationTestScenario`
- PR pipeline: `.tekton/zoa-boundary-pull-request.yaml`
- Push pipeline: `.tekton/zoa-boundary-push.yaml` → Quay → ECR mirror
- MintMaker/renovate config for grouped dependency updates

**Acceptance Criteria**:

| #   | Criterion                                                                                         |
| --- | ------------------------------------------------------------------------------------------------- |
| 1   | `zoa-boundary` Konflux Component registered with `ImagesRepository` and `IntegrationTestScenario` |
| 2   | PR pipeline (`.tekton/zoa-boundary-pull-request.yaml`) builds and validates on every PR           |
| 3   | Push pipeline (`.tekton/zoa-boundary-push.yaml`) builds, pushes to Quay, and mirrors to ECR       |
| 4   | Enterprise Contract passes (UBI9 base, no critical CVEs)                                          |
| 5   | MintMaker/renovate config updated for `zoa-boundary` dependencies                                 |

**Repos**: `rosa-hyperfleet-zoa` (Containerfile + Tekton), `rosa-hyperfleet` (ECR + skopeo mirror)

#### Plan Details

- `Containerfile.boundary` in `rosa-hyperfleet-zoa`
- UBI9 base image (consistent with other ZOA images)
- Push to Quay: `quay.io/redhat-user-workloads/rosa-tenant/zoa-boundary:<commit-sha>`
- Skopeo mirror to ECR (same pipeline step as zoa-lambda/zoa-runner)

---

### 4. Observability — ZOA Access Lambda + Boundary Sessions

#### Jira Fields

**Title**: ZOA Boundary Observability — metrics, alerts, dashboards

**Overview**: Extend the ZOA observability stack (EMF → CloudWatch → YACE → Prometheus → Thanos → Grafana) to cover the ZOA Access Lambda and boundary sessions. Mirrors the pattern from [ROSAENG-65234](https://redhat.atlassian.net/browse/ROSAENG-65234).

**Scope**:

- EMF metrics from Access Lambda: session lifecycle (created, terminated, duration, active), API traffic (count, latency, errors), identity bridge (lookup latency, failures)
- YACE scrape jobs for `ZOA-Access` custom namespace (RC only) and ECS task metrics
- 5 alerting rules: `ZOABoundaryReaperStalled` (critical), `ZOABoundarySessionCreationFailures`, `ZOAAccessLambdaErrors`, `ZOAIdentityBridgeFailures`, `ZOABoundaryOrphanedSessions`
- Recording rules for session success rate and active session count
- Grafana dashboard panels for active sessions, creation rate, duration histogram, reaper activity, Access Lambda health
- Monitoring e2e specs on live ephemeral

**Acceptance Criteria**:

| #   | Criterion                                                                                                                                         |
| --- | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | Access Lambda emits EMF metrics for session lifecycle (created, terminated, duration, active) and API traffic (request count, latency, errors)    |
| 2   | Identity bridge metrics emitted (lookup latency, failures)                                                                                        |
| 3   | YACE scrape jobs configured for `ZOA-Access` namespace and ECS boundary task metrics                                                              |
| 4   | At least 5 alerting rules deployed (reaper stalled, session creation failures, Access Lambda errors, identity bridge failures, orphaned sessions) |
| 5   | Recording rules for session success rate and active session count                                                                                 |
| 6   | Grafana dashboard with panels for active sessions, creation rate, duration histogram, reaper activity, Access Lambda health                       |
| 7   | Monitoring e2e specs validate metric presence, recording rules, and alert rule groups on live ephemeral                                           |

**Repos**: `rosa-hyperfleet-zoa` (EMF emission code), `rosa-hyperfleet` (YACE config, alerting rules, Grafana dashboard)

#### Plan Details

**EMF metrics (emitted from ZOA Access Lambda):**

| Metric                           | Dimensions                                                    | Purpose                              |
| -------------------------------- | ------------------------------------------------------------- | ------------------------------------ |
| `BoundarySessionCreated`         | region, target, operator                                      | Session start rate                   |
| `BoundarySessionTerminated`      | region, target, reason (sre_exit / deadline / reaper / error) | Termination tracking                 |
| `BoundarySessionDurationSeconds` | region, target                                                | Session length distribution          |
| `BoundarySessionActive`          | region, target                                                | Current active sessions (gauge)      |
| `BoundaryReaperTerminations`     | region, target                                                | Reaper-forced terminations           |
| `AccessLambdaRequestCount`       | method, path, statusCode                                      | API traffic                          |
| `AccessLambdaLatencyMs`          | method, path                                                  | Response time                        |
| `AccessLambdaErrors`             | method, path, errorType                                       | Error breakdown                      |
| `IdentityBridgeLookupMs`         | target                                                        | DynamoDB identity resolution latency |
| `IdentityBridgeFailures`         | target, reason                                                | Failed identity resolutions          |

**YACE CloudWatch Exporter:**

- Add `ZOA-Access` custom namespace scrape job (RC only — Access Lambda is not per-VPC)
- Add ECS metrics scrape for boundary task containers (task count, CPU, memory)

**Alerting rules (PrometheusRule CR in `alerting-rules/templates/zoa-boundary.yaml`):**

- `ZOABoundaryReaperStalled` (critical) — no reaper terminations when expired sessions exist
- `ZOABoundarySessionCreationFailures` (warning) — elevated session creation error rate
- `ZOAAccessLambdaErrors` (warning) — elevated 5xx rate on Access Lambda
- `ZOAIdentityBridgeFailures` (warning) — identity resolution failures (SRE attribution broken)
- `ZOABoundaryOrphanedSessions` (warning) — active sessions past deadline without reaper action

**Recording rules:**

- `zoa_boundary:session_creation_success_rate:5m`
- `zoa_boundary:active_sessions:current`
- `zoa_access:request_success_rate:5m`

**Grafana dashboard:**

- Extend existing ZOA dashboard (or new "ZOA Boundary" dashboard) with panels: active sessions, session creation rate, session duration histogram, reaper activity, Access Lambda latency/errors, identity bridge health

**Monitoring e2e:**

- Thanos-based specs validating metric presence, recording rules, and alert rule groups (same pattern as existing `test/e2e/monitoring_test.go`)

**Repos**: `rosa-hyperfleet-zoa` (EMF emission code), `rosa-hyperfleet` (YACE config, alerting rules, Grafana dashboard)

---

### 5. E2E Testing

#### Jira Fields

**Title**: ZOA Boundary E2E — session lifecycle, identity bridge, reaper, autodiscovery

**Overview**: End-to-end validation for ZOA Boundary. Extends the existing Ginkgo e2e suite with boundary-specific specs covering session lifecycle, identity bridge correctness, reaper enforcement, autodiscovery, cross-account MC sessions, and negative paths. Wires into Prow CI.

**Scope**:

- Session lifecycle specs: start → join → execute TA → stop → verify terminated
- Identity bridge specs: TA execution attributed to originating SRE, not shared task role
- Reaper specs: session past 4h deadline auto-terminated
- Autodiscovery specs: CLI reads SSM → Access Lambda → targets
- Cross-account specs: MC session via AssumeRole
- Negative specs: unauthorized caller rejected, non-creator cannot join/stop
- `openshift/release` Prow job configuration

**Acceptance Criteria**:

| #   | Criterion                                                                                                                     |
| --- | ----------------------------------------------------------------------------------------------------------------------------- |
| 1   | Session lifecycle e2e: start → stop → list → verify terminated                                                                |
| 2   | Identity bridge e2e: TA execution from boundary container is attributed to correct SRE (username matches)                     |
| 3   | Reaper e2e: session past deadline is auto-terminated                                                                          |
| 4   | Autodiscovery e2e: `zoa deployments` discovers deployments from SSM, `zoa targets <dep>` discovers targets from Access Lambda |
| 5   | Cross-account e2e: MC session works (Access Lambda creates ECS task in MC VPC)                                                |
| 6   | Negative tests: unauthorized caller rejected, non-creator cannot join/stop                                                    |
| 7   | Wired into CI via `openshift/release` Prow job configuration                                                                  |
| 8   | All boundary e2e specs pass on ephemeral environment                                                                          |

**Repos**: `rosa-hyperfleet-zoa` (tests), `rosa-hyperfleet` (CI infra), `openshift/release` (Prow jobs)

#### Plan Details

Wire into CI: `openshift/release` Prow job configuration for boundary e2e (may need separate from existing ZOA e2e due to Central Account dependency).

---

### 6. Documentation

#### Jira Fields

**Title**: ZOA Boundary Documentation — architecture, CLI reference, SRE runbook

**Overview**: Comprehensive documentation covering architecture, user guide, CLI reference, and operations. Updates existing docs to reflect boundary as the deployed access model and adds new user-facing guides and SOPs.

**Scope**:

- Update `docs/boundary/architecture.md`, `docs/boundary/sre-access-guide.md`, `docs/cli-reference.md` (rosa-hyperfleet-zoa)
- Update `docs/design/zoa-architecture.md` (rosa-hyperfleet) where boundary is still marked future
- New or extend `docs/sop/boundary-troubleshooting.md` (rosa-hyperfleet): stuck sessions, reaper, SSM, exec-attached

**Acceptance Criteria**:

| #   | Criterion                                                                                                                              |
| --- | -------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | `docs/design/zoa-architecture.md` (rosa-hyperfleet) updated: boundary sections in main body; `PLANNED` labels removed                  |
| 2   | `README.md` (rosa-hyperfleet-zoa) updated: architecture diagram shows boundary as deployed; `TEMPORARY` path removed                   |
| 3   | New `docs/boundary.md` (rosa-hyperfleet-zoa): user guide for session start/stop/join, autodiscovery, container tooling                 |
| 4   | `docs/cli-reference.md` (rosa-hyperfleet-zoa) updated: `zoa deployments`, `zoa targets`, and `zoa session` command families documented |
| 5   | New `docs/sop/boundary-troubleshooting.md` (rosa-hyperfleet): SOP for stuck sessions, reaper failures, Central Account access          |
| 6   | All markdown passes `prettier` formatting                                                                                              |

**Repos**: `rosa-hyperfleet`, `rosa-hyperfleet-zoa`

#### Plan Details

No additional implementation details beyond scope — documentation deliverables are fully described above.

---

### 7. App-interface: Central Account SAML role for ZOA Access (dev, int, stage)

#### Jira Fields

**Title**: App-interface: Central Account SAML role for ZOA Access invoker (dev, int, stage)

**Overview**: Today the ZOA Access invoker role trusts **`OrganizationAccountAccessRole`** in the environment Central Account (`aws.zoa_access_trusted_assumer_role_names` default in `rosa-hyperfleet/config/defaults.yaml`). That is appropriate for dev jump accounts but not for int/stage SRE workflows, where operators authenticate with **`kinit`** + **`rh-aws-saml-login`** into scoped app-interface IAM roles (same pattern as OSD staging **`rrp-admin`**).

Provision a dedicated Central Account role per HyperFleet environment (dev, integration, stage) via [app-interface](https://gitlab.cee.redhat.com/service/app-interface), mirroring existing SAML hub roles such as [osd-staging-2 `rrp-admin.yml`](https://gitlab.cee.redhat.com/service/app-interface/-/blob/master/data/aws/osd-staging-2/roles/rrp-admin.yml). SREs run:

```text
rh-aws-saml-login <central-account-alias>/<account-id>-<role-name>
# e.g. osd-staging-2/811685182089-rrp-admin → assumed-role/811685182089-rrp-admin/slopezma
```

Then update HyperFleet config so **`zoa-access` invoker trust** allows that role **name** (not org admin), and the SAML role can **`sts:AssumeRole`** into `{deployment}-zoa-access-invoker` in the RC account.

**Scope**:

- app-interface: new or extended IAM role(s) in each Central Account used by HyperFleet dev / int / stage (exact account aliases TBD with platform — follow osd-staging-2 pattern)
- SAML role permissions: **`sts:AssumeRole`** on `arn:aws:iam::*:role/*-zoa-access-invoker` (narrow to known RC account IDs per env); **`ssm:GetParameter`** on `/zoa/deployments` (and any paths required for `zoa deployments`); deny broad RC/MC admin where policy allows scoping
- `rosa-hyperfleet`: override `aws.zoa_access_trusted_assumer_role_names` in `config/dev/`, `config/integration/`, `config/stage/` (not global default); run `uv run scripts/render.py`; Terraform apply per region
- Docs: `docs/boundary/sre-access-guide.md` — prerequisite block (kinit, SAML profile, verify with `aws sts get-caller-identity` before `zoa deployments`)

**Acceptance Criteria**:

| #   | Criterion                                                                                                                                                                                                  |
| --- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | app-interface merges define a SAML-assumable Central Account role per target env (dev, int, stage) with least-privilege policy for ZOA laptop workflow                                                     |
| 2   | SRE can `rh-aws-saml-login` into that role and `aws sts get-caller-identity` shows `assumed-role/<account-id>-<role>/<username>`                                                                           |
| 3   | Same credentials can `sts:AssumeRole` into `{deployment}-zoa-access-invoker` and call Access Function URL (`zoa targets`, `zoa session start`)                                                             |
| 4   | `aws.zoa_access_trusted_assumer_role_names` lists the SAML role **name** (e.g. `811685182089-rrp-admin` or env-specific equivalent) — **`OrganizationAccountAccessRole` removed** from int/stage overrides |
| 5   | `make pre-push` / render diff committed; at least one int or stage region validated end-to-end                                                                                                             |
| 6   | SRE access guide documents the SAML role name / login command per environment                                                                                                                              |

**Repos**: `service/app-interface` (GitLab), `rosa-hyperfleet` (config + docs)

**Depends on**: Story 2 (invoker role + `trusted_assumer_role_names` wiring) — can proceed in parallel once module exists.

**Blocks**: Removing org-admin from the ZOA laptop path in int/stage; future Central Account permission tightening (see [Future: Direct Access Restriction](#future-direct-access-restriction-break-glass-epic-prerequisite)).

#### Plan Details

**Terraform trust chain (already implemented in `terraform/modules/zoa-access/`):**

1. SRE credentials in Central Account (SAML role session name = Kerberos username).
2. Invoker role trust policy: `Principal` = Central Account ID + `aws:PrincipalArn` matching `arn:aws:iam::<central>:role/<name>` for each entry in `trusted_assumer_role_names`.
3. Invoker role → `lambda:InvokeFunctionUrl` on Access Lambda; Access Lambda → `sts:AssumeRole` + `ecs:RunTask` using deployment config.

**Config pattern (after app-interface lands):**

```yaml
# config/integration/defaults.yaml (example — replace with real role name from app-interface)
aws:
  zoa_access_trusted_assumer_role_names:
    - "<central-account-id>-zoa-sre" # exact name from app-interface merge
```

**Reference implementation:** OSD staging `811685182089-rrp-admin` demonstrates SAML login, one-hour creds, and stable session name for ownership — HyperFleet needs an equivalent role whose **only** elevated cross-account action is assuming the regional invoker role (plus SSM read for discovery).

**Jira:** Create under epic [ROSAENG-60291](https://redhat.atlassian.net/browse/ROSAENG-60291), type Story, team HyperFleet. After creation, replace **ROSAENG-TBD** in the [Jira index](#jira-index) with the issued key.

---

## Key Design Considerations

### SRE Identity at SigV4 Level — Foundational for Approval Workflow

Every ZOA API request (both ZOA Access and per-VPC Lambda) must carry the **originating SRE's identity** at the SigV4 level. This is not just for audit — it is foundational for the future **approval workflow epic**, where the system must answer:

- **Who is requesting?** (SRE A requests break-glass)
- **Who is approving?** (SRE B approves — must be a different person)
- **What role/group do they belong to?** (SRE, team lead, manager, director — approval policies may vary by requester/approver seniority)
- **Are they authorized for this scope?** (e.g., only on-call SREs can request kube-admin break-glass)

The identity model chosen now constrains what the approval workflow can enforce later. Three possible approaches — the decision does NOT need to be made in this epic, but the boundary design must not preclude any of them:

**Current plan: Single shared role, LDAP for future approvals**

With `rh-aws-saml-login`, the SRE gets a temporary IAM role in the Central Account based on their app-interface configuration. The SigV4 ARN encodes the SRE's identity:

```
arn:aws:sts::CENTRAL_ACCOUNT:assumed-role/sre-role/slopezma
                                          ^^^^^^^^ ^^^^^^^^
                                          shared role  SRE identity (Kerberos principal)
```

All SREs share one role — approval authorization is **not** encoded in IAM. The future approval workflow will use **LDAP group membership** to determine who can approve:

- SRE username extracted from SigV4 session name (e.g., `slopezma`)
- Lambda queries LDAP (or a cached group dump) to check membership in `zoa-approvers`, `zoa-directors`, etc.
- This decouples approval tiers from IAM entirely — adding new approval groups only requires LDAP changes, not IAM + app-interface changes

**What this epic must ensure:**

- The SRE's **username** (not just an opaque ARN) is captured and stored in `boundary-sessions` and propagated to all downstream tables (`executions`, `audit`)
- The identity bridge preserves the original SRE identity through the ECS task role boundary — the per-VPC Lambda must know who the SRE is, not just that "an ECS task called me"
- The DynamoDB schema for `boundary-sessions` should store both the username AND the full ARN — the ARN for audit/forensic purposes, the username as the human-readable key for LDAP lookup
- No hard dependency on a specific LDAP integration — the approval workflow epic will decide the exact group names and caching mechanism

### Approval Workflow Readiness — Routes Prepared, Logic Deferred

The `/approve/{id}` and `/reject/{id}` routes are included in **both** the `access` and `api` handler modes from day one, even though the approval workflow is a separate epic. This ensures:

- **No session required to approve**: An approver only needs `kinit` → `rh-aws-saml-login` → `zoa approve <id>` from their laptop. The request goes through the Access Lambda Function URL (via invoker role). No ECS task creation, no SSM session — minimum friction.
- **Approve from inside boundary too**: If an SRE is already inside a boundary container and a peer requests approval, they can approve from there via the per-VPC API Lambda. Same code, same DynamoDB write.
- **Reconciler picks up approvals**: The per-VPC Worker Lambda reconciler detects `status=approved` on the next tick and dispatches the execution. The approval writer (Access or API Lambda) does NOT execute TAs — it only changes state in DynamoDB.

For this epic, the routes return a stub response (e.g., `501 Not Implemented — approval workflow not yet enabled`). The approval workflow epic will implement the full logic: validation (approver != requester), notification (SNS → Slack), policy evaluation (OPA/Rego), and the reconciler dispatch path.

### Tamper-Proof Identity Model — No ABAC Required

ZOA uses a **scoped credentials** model instead of ABAC (Attribute-Based Access Control). The Access Lambda is the single trust boundary that validates identity, enforces authorization, and vends per-operation scoped credentials. No shared IAM roles are exposed to SREs.

**Why not ABAC**: ABAC requires a shared IAM role with tag-based conditions (e.g., `ecs:ResourceTag/operator == aws:PrincipalTag/operator`). This adds OIDC → STS → session tag plumbing, requires a Keycloak mapper, and creates a shared role that must be protected. Scoped credentials are simpler and strictly more secure — the credential itself encodes the authorization, eliminating an entire class of misconfiguration.

**Identity resolution by caller context:**

| Caller location             | SigV4 identity                                                                                               | Resolution method                                                | Tamper-proof?                                                          |
| --------------------------- | ------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------- | ---------------------------------------------------------------------- |
| **Laptop → Access Lambda**  | Invoker role in RC: `assumed-role/zoa-access-invoker/slopezma` (session name preserved from Central Account) | Extract username from ARN session name                           | ✅ Central-trusted role, session name from hub login                   |
| **ECS → per-VPC Lambda**    | Shared task role: `assumed-role/zoa-boundary-task/<ecs-task-uuid>`                                           | Extract task UUID from ARN → sessions DynamoDB lookup → operator | ✅ Task UUID assigned by AWS, sessions table written by trusted Lambda |
| **Laptop → approve/reject** | Personal IAM role                                                                                            | Extract username from ARN session name → LDAP for manager chain  | ✅ Same as laptop path                                                 |

**ECS Exec isolation (no ABAC needed)**: When an SRE calls `zoa session join`, the Access Lambda validates ownership (session.operator must match the SigV4 caller), then vends per-task scoped STS credentials with an inline policy restricting `ecs:ExecuteCommand` to that specific task ARN. The SRE literally cannot exec into another SRE's task because the credential only works for one task. The Lambda sets `RoleSessionName` to the SRE's username, so CloudTrail shows `assumed-role/zoa-exec-scoped/slopezma`.

**Adversarial analysis — attacks an SRE could attempt:**

| Attack                                           | Result  | Why it fails                                                                                                         |
| ------------------------------------------------ | ------- | -------------------------------------------------------------------------------------------------------------------- |
| Change env vars inside ECS to alter identity     | ❌      | API Lambda ignores env vars — resolves identity entirely from SigV4 task UUID → DynamoDB                             |
| Craft HTTP request with fake `X-Operator` header | ❌      | Header is overwritten by SigV4 identity at Lambda level; identity bridge resolves from ARN, not headers              |
| Set `X-Session-ID` header to another session     | ❌      | No such header exists — session ID resolved server-side from `task-id-index` GSI lookup                              |
| Call Access Lambda Function URL from inside ECS  | ❌      | Task role has no `lambda:InvokeFunctionUrl` on Access Lambda; Function URL resource policy restricts to invoker role |
| Modify session record in DynamoDB                | ❌      | Task role has no DynamoDB permissions                                                                                |
| Modify own ECS task tags                         | ❌      | Task role has no `ecs:TagResource` permission                                                                        |
| Assume Lambda's IAM role                         | ❌      | Lambda role trust: `Principal: lambda.amazonaws.com` only                                                            |
| Exec into another SRE's task                     | ❌      | Scoped credentials restrict to one task ARN                                                                          |
| Container breakout → ECS metadata endpoint       | Limited | Fargate microVM isolation; task role scoped to SSM + CW + Function URL only                                          |

**Session ID and SignerARN — dual-field forensics**: Every execution and audit entry stores both the resolved `operator` (human-readable, stable across re-auth) and the raw `signerARN` (full SigV4 caller ARN for forensic reconstruction). The `sessionID` field links all operations back to the originating boundary session — derived server-side from the same `task-id-index` GSI lookup that resolves the operator, so the SRE cannot forge or redirect session linkage. Implementation detail and GSI policy: [boundary-identity-and-storage.md](./boundary-identity-and-storage.md).

**IAM scoping rules (enforced in Terraform):**

```
Task role MUST have:
  ✅ ssmmessages:* (ECS Exec)
  ✅ logs:PutLogEvents (CloudWatch)
  ✅ Lambda Function URL invoke (ZOA API — per-VPC)
  ✅ bedrock:InvokeModel (in-region Haiku via application inference profile)

Task role MUST NOT have:
  ❌ dynamodb:* (no direct table access)
  ❌ ecs:TagResource (no tag modification)
  ❌ sts:AssumeRole (except future break-glass, gated by breakglass_role_arns)
  ❌ iam:* (no IAM modification)
```

**ECS task tags — tamper-proof SRE identification at AWS level:**

The Access Lambda sets AWS-level tags on every ECS task at creation. These tags are tamper-proof (the task role has no `ecs:TagResource` permission) and provide a second independent path for SRE identification without touching DynamoDB:

| Tag          | Value               | Example       |
| ------------ | ------------------- | ------------- |
| `sre`        | Operator username   | `slopezma`    |
| `sessionId`  | Boundary session ID | `sess-abc123` |
| `deployment` | Deployment name     | `us-east-1`   |
| `target`     | Target cluster      | `mc01`        |

Use case: `aws ecs describe-tasks --tasks <task-id>` → tags → SRE. This is critical for Bedrock billing investigation (see below).

### Bedrock Observability — Regional Models, Invocation Logging, and Billing Attribution

Claude Code in boundary containers uses Amazon Bedrock via the ECS task role. Three design decisions govern this integration:

**1. Regional model sovereignty (data residency)**

Claude Code uses **classic Amazon Bedrock Invoke** with an **application inference profile** per cluster (source = in-region Haiku foundation model). IAM allows `bedrock:InvokeModel` on that profile and foundation model ARN only. No `us.`/`eu.`/`global.` system inference profiles.

If Haiku 4.5 is unavailable in a region, change `claude_bedrock_foundation_model_id` (or defer boundary Claude) for that region — no geo-profile fallback.

**2. Bedrock model invocation logging (metadata only)**

Bedrock invocation logging is an account-level setting that captures metadata for every API call. We enable it to CloudWatch Logs **without** payload capture (no prompt/response content, no S3):

| Field captured            | Purpose                                                 |
| ------------------------- | ------------------------------------------------------- |
| `identity.arn`            | IAM principal — contains ECS task ID as RoleSessionName |
| `modelId`                 | Which model was used                                    |
| `input.inputTokenCount`   | Input tokens consumed                                   |
| `output.outputTokenCount` | Output tokens consumed                                  |
| `requestId`               | Unique request identifier                               |

Payload capture (S3) is explicitly disabled — SSM session recording already captures the human-readable conversation displayed in the terminal. Bedrock invocation logging adds only the **cost-relevant metadata** that SSM cannot capture.

**Three types of CloudWatch logs (not to be confused):**

| Log type                    | What it captures                                  | Log group                        | Per-task?                        |
| --------------------------- | ------------------------------------------------- | -------------------------------- | -------------------------------- |
| **Container stdout/stderr** | Entrypoint messages, process output               | `/ecs/zoa-boundary`              | Shared group, per-task stream    |
| **SSM session recording**   | Interactive terminal I/O (keystrokes + screen)    | `/ecs/zoa-boundary/ssm-sessions` | Shared group, per-session stream |
| **Bedrock invocation**      | Token counts, model ID, IAM identity per API call | `/aws/bedrock/model-invocations` | Shared (account-level)           |

**3. Billing attribution — "investigate when needed"**

Bedrock bills through the AWS account, separate from individual SRE AI budgets. We do NOT need alerts or enforcement now — we need the ability to investigate if usage grows unexpectedly.

**Investigation chain (tamper-proof, ~1 command):**

```
Bedrock invocation log:
  identity.arn = "assumed-role/zoa-boundary-task-role/abc123def456"
                                                      └── task ID

$ aws ecs describe-tasks --tasks abc123def456 --cluster zoa-boundary
  tags: [{ key: "sre", value: "slopezma" }]   ← tamper-proof
```

Or via DynamoDB: task ID → `task-id-index` GSI on sessions table → `operator: "slopezma"`.

Two independent paths, both tamper-proof. The SRE cannot change the task ID (AWS-assigned) or the ECS task tags (no `ecs:TagResource`).

**CloudWatch Insights query for aggregate per-task usage:**

```sql
fields identity.arn as principal,
       input.inputTokenCount as inTokens,
       output.outputTokenCount as outTokens
| parse principal "*/zoa-boundary-task-role/*" as @prefix, @taskId
| stats sum(inTokens) as totalInput,
        sum(outTokens) as totalOutput,
        count() as calls
        by @taskId
| sort totalOutput desc
```

**Why STS session tags are not possible on ECS**: ECS task role credentials are assumed internally by the ECS agent. AWS does not support injecting custom `RoleSessionName` or STS session tags into this `AssumeRole` call ([open feature request](https://github.com/aws/containers-roadmap/issues/2426)). The tamper-proof ECS task tags + sessions table correlation provides equivalent investigative capability without this AWS limitation being a blocker.

### Break-Glass Architecture — Detailed Design for Future Epic

Break-glass is a separate epic, but the boundary container and Terraform are designed now to support it without redesign. The core requirement: when break-glass is approved, the SRE types `kubectl` or `aws` and it just works — zero manual credential setup.

#### Kubectl break-glass (kube-read / kube-write / kube-admin)

**End-to-end flow:**

```mermaid
sequenceDiagram
    participant SRE as SRE (inside boundary)
    participant CLI as ZOA CLI
    participant FU as Per-VPC Lambda
    participant EKS as Target EKS
    participant K8s as K8s RBAC

    SRE->>CLI: zoa breakglass request --scope kube-write
    CLI->>FU: POST /breakglass/request (SigV4 with task role)
    FU->>FU: Identity bridge: task UUID → slopezma
    FU->>FU: Store request in DynamoDB (pending_approval)
    FU-->>CLI: {requestId, status: pending_approval}

    Note over SRE: Approver on laptop...
    Note over FU: Approver: zoa approve <requestId>

    FU->>FU: Reconciler picks up approved request
    FU->>EKS: Create EKS Access Entry for break-glass role
    FU->>K8s: Create RBAC: ClusterRoleBinding (breakglass-write)
    FU->>K8s: Create RBAC: impersonation permission for sre:slopezma only
    FU->>FU: Update DynamoDB: status=active, expiresAt=now+4h

    SRE->>CLI: zoa breakglass connect <requestId>
    CLI->>FU: GET /breakglass/<requestId>
    FU-->>CLI: {eksEndpoint, eksCA, clusterName, impersonateAs: sre:slopezma}
    CLI->>CLI: Write ~/.kube/config with EKS exec plugin + impersonation

    SRE->>EKS: kubectl get pods (transparent)
    Note over EKS: Token: aws eks get-token (15-min presigned URL, auto-regenerated)<br/>Identity in K8s audit: sre:slopezma (via impersonation)<br/>No stored credentials — ECS task role auto-refreshed
```

**Credential lifecycle:**

| Credential                            | Duration                               | Refresh mechanism                                    |
| ------------------------------------- | -------------------------------------- | ---------------------------------------------------- |
| ECS task role (metadata endpoint)     | ∞ (auto-refreshed by ECS agent)        | Transparent — valid for entire task lifetime         |
| `aws eks get-token` per-request token | 15 minutes                             | Regenerated by kubectl exec plugin on every API call |
| EKS Access Entry                      | Until break-glass expires (4h default) | Reconciler revokes on expiry                         |
| K8s RBAC bindings                     | Until break-glass expires              | Reconciler deletes on expiry                         |

**K8s audit log attribution**: The kubeconfig uses `--as=sre:<username>` for K8s impersonation. The RBAC only allows impersonating the specific SRE (`resourceNames: ["sre:slopezma"]`). K8s audit log shows: `user.username: "sre:slopezma"`, `impersonatedUser: true`. Cannot be spoofed — the SRE could edit the kubeconfig but K8s RBAC rejects impersonation of other users.

**Zero stored credentials**: The kubeconfig is a pointer file — it tells kubectl to run `aws eks get-token` on every API call. The token is a 15-minute presigned STS URL derived from the ECS task role (which is auto-refreshed by the ECS agent). No secrets on disk, no expiry within the session window.

#### AWS break-glass (aws-read / aws-write / aws-admin)

**End-to-end flow:**

```mermaid
sequenceDiagram
    participant SRE as SRE (inside boundary)
    participant CLI as ZOA CLI
    participant FU as Per-VPC Lambda

    SRE->>CLI: zoa breakglass request --scope aws-read
    CLI->>FU: POST /breakglass/request
    FU-->>CLI: {requestId, status: pending_approval}

    Note over FU: Approver approves...
    FU->>FU: Reconciler: activate break-glass

    SRE->>CLI: zoa breakglass connect <requestId>
    CLI->>FU: GET /breakglass/<requestId>
    FU->>FU: AssumeRole to break-glass role with RoleSessionName=slopezma
    FU-->>CLI: {roleArn, expiresAt}
    CLI->>CLI: Write ~/.aws/config with [profile breakglass] using credential_source=EcsContainer + role_arn

    SRE->>SRE: aws ec2 describe-instances --profile breakglass (transparent)
    Note over SRE: CloudTrail: assumed-role/zoa-breakglass-read/slopezma
```

**Credential chain**: The break-glass AWS profile uses `credential_source = EcsContainer` (reads task role from metadata endpoint) + `role_arn = <break-glass-role>` + `role_session_name = <sre-username>`. AWS SDK handles the chain transparently. The `~/.aws/config` file contains no secrets — just role ARNs and the credential source pointer.

**CloudTrail attribution**: `RoleSessionName` is set to the SRE's username by the Lambda (not by the SRE). CloudTrail shows `assumed-role/zoa-breakglass-read/slopezma` — direct SRE identification without any lookup.

**Break-glass role duration**: Set `max_session_duration = 14400` (4h) on the break-glass IAM role in Terraform. The assumed credentials last the full session, with the reaper enforcing the hard deadline by stopping the ECS task.

#### What this epic prepares for break-glass

| Layer                 | Prepared now                                                                                           | Break-glass epic adds                                                           |
| --------------------- | ------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------- |
| **Containerfile**     | kubectl and aws CLI installed. `~/.kube/` and `~/.aws/` writable (created by `useradd`).               | Nothing — container is ready                                                    |
| **ECS task role**     | `breakglass_role_arns = []` variable. No `sts:AssumeRole` today.                                       | Populate variable with per-scope role ARNs                                      |
| **Security group**    | Egress to EKS API (443) — network path exists                                                          | Nothing — SG is ready                                                           |
| **EKS access**        | No EKS Access Entry for task role (zero standing access)                                               | Dynamic Access Entry creation/revocation per break-glass request                |
| **DynamoDB sessions** | Optional break-glass fields: `breakglassScope`, `breakglassStatus`, `breakglassExpiresAt` (NULL today) | Populate on break-glass activation                                              |
| **Container env**     | `ZOA_BREAKGLASS_ROLE_ARN` reserved (empty)                                                             | Lambda injects per-scope role ARN via RunTask overrides                         |
| **PS1 prompt**        | Shows `[sre@zoa:us-east-1/mc01]`                                                                       | Could extend to show break-glass scope: `[sre@zoa:us-east-1/mc01 🔓kube-write]` |

**Key principle**: Zero standing EKS/AWS access. Break-glass dynamically injects access, the `zoa breakglass connect` command configures the SRE's shell, and the reconciler revokes everything on expiry. All break-glass operations are attributed to the SRE in CloudTrail (via `RoleSessionName`) and K8s audit (via impersonation).

### Future: Direct Access Restriction (Break-Glass Epic Prerequisite)

This epic does **not** restrict SRE direct access to RC/MC accounts. Today SREs can switch-role from the Central Account to RC/MC accounts with admin-like permissions, and CI pipelines use the same path. This remains unchanged.

**Why not restrict now:** Restricting direct access requires three controls to ship together — they are a single atomic change:

1. **Lambda resource-based policy** — restrict Function URL callers to boundary task role + CI pipeline role only
2. **Central Account permission restriction** — narrow SRE's app-interface role to SSM read + invoker role assume only (no switch-role to RC/MC)
3. **Break-glass escape hatch** — `zoa breakglass` as the safety valve for emergencies

Without break-glass, restricting direct access leaves SREs with no path to handle emergencies that TAs can't cover. The break-glass epic will implement all three controls together.

**Dev/ephemeral will never restrict**: Developers and CI need direct account access for testing, debugging, and iterating. The restriction is an operational security control for int/stage/prod only.

**CI keeps direct Function URL access permanently**: CI pipeline roles stay in the RC/MC accounts and call Function URLs directly. CI tests TA logic, not the boundary path. Boundary-specific e2e tests (Story 5) validate session lifecycle separately.

### Network Path: Function URL via NAT Gateway

The ZOA Boundary container reaches per-VPC Lambda Function URLs through NAT Gateway. This is because Lambda Function URLs are **public HTTPS endpoints** (`*.lambda-url.region.on.aws`) even when the Lambda is VPC-attached — VPC attachment only controls the Lambda's outbound execution network (ENIs in private subnets for reaching EKS, DynamoDB, etc.), not its inbound invocation endpoint.

Traffic from NAT Gateway to a Lambda Function URL in the same region stays on the AWS backbone network (does not traverse the public internet), and Function URLs require SigV4 authentication (unauthenticated requests are rejected before Lambda code runs). The resource-based policy will restrict callers to ONLY ZOA Boundary task roles.

A future hardening story could place a Private API Gateway with VPC Endpoint in front of the per-VPC Lambda — eliminating the publicly-addressable Function URL entirely while keeping the same HTTP semantics. The SDK-based `lambda:Invoke` approach is NOT viable because the ZOA API requires HTTP routing and native response streaming (`RESPONSE_STREAM` invoke mode) — SDK invocation would break both. This is not required for this epic but the container and CLI design do not preclude a Private APIGW migration.

---

## Open Questions / Dependencies

1. **Central Account cross-account IAM**: The Central Account already exists per environment, but we need a "pipeline writer" IAM role that RC pipeline roles can assume for `ssm:PutParameter`. Validate with the platform team whether this role can be created via app-interface or needs manual provisioning.
2. **Break-glass interaction**: The reaper and the boundary container design should account for future break-glass EKS access entries. Not implementing break-glass in this epic, but IAM and network design must not preclude it.
3. **Bedrock model availability per region**: Verify Haiku 4.5 foundation model + application inference profile in each HyperFleet deployment region; set `claude_bedrock_foundation_model_id` per region if needed.
4. **rh-aws-saml-login session name stability**: **Resolved** — session name matches Kerberos principal (e.g. `slopezma`). Provision dedicated Central Account SAML roles per env (Story 7) and list them in `aws.zoa_access_trusted_assumer_role_names` instead of relying on `OrganizationAccountAccessRole` for day-2 SRE access.
5. **Ephemeral SSM parameter lifecycle**: In the dev Central Account, ephemeral deployment entries must be reliably cleaned up on teardown. If an ephemeral teardown fails or is abandoned, stale entries will accumulate. The reaper or a separate GC mechanism may need to detect and clean orphaned entries.
6. **LDAP integration for future approvals**: Confirm that LDAP group membership (e.g., `zoa-approvers`) is the approved mechanism for approval authorization. Validate network path from Lambda to LDAP, or plan a caching strategy (S3 group dump, refreshed periodically).
