# ZOA Boundary — ECS Exec and CloudWatch logging

## Two log groups (one KMS key)

ZOA boundary uses **two CloudWatch log groups** encrypted with the **same CMK** (shared ZOA key on RC; per-cluster key on MC when no `kms_key_arn`).

| Purpose | Log group | Stream naming | What it captures |
| -------- | --------- | -------------- | ---------------- |
| **Container stdout** | `/ecs/<cluster_id>/zoa-boundary` | `container/zoa-boundary/<task-id>` | Task startup script only (banner, tool check, “ready for connections”). |
| **ECS Exec sessions** | `/ecs/<cluster_id>/zoa-boundary/ssm-sessions` | `ecs-execute-command-<session-id>` | Interactive shell I/O after `zoa session join` — bash, `zoa run`, etc. |

`<cluster_id>` is the HyperFleet cluster id (`regional_id` on RC, `management_id` on MC).

**Do not** search session content in the container group. **Do not** use the exec group for “did the task start?” — use container logs.

Terraform: `terraform/modules/zoa-boundary/` in [rosa-hyperfleet](https://github.com/openshift-online/rosa-hyperfleet) (`main.tf`, `task.tf`, module `README.md`).

FedRAMP AU-09: exec group + KMS + `cloudWatchEncryptionEnabled = true` on cluster `executeCommandConfiguration`.

## AWS requirements

From [Monitor Amazon ECS Exec commands using CloudWatch Logs](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/ecs-exec-logging.html):

1. **Cluster** — `logging = OVERRIDE`, exec log group = `.../ssm-sessions`, same CMK as both log groups.
2. **Task role** — `logs:DescribeLogGroups`; `CreateLogStream` / `DescribeLogStreams` / `PutLogEvents` on **exec log group only**; KMS `GenerateDataKey` / `Decrypt` on the shared CMK.
3. **Caller** — KMS for encrypted exec channel (e.g. `OrganizationAccountAccessRole` on RC ZOA key).
4. **RunTask** — `EnableExecuteCommand: true` (`pkg/awsecs/client.go`).
5. **Image** — **`script` and `cat`** (`util-linux` in `Containerfile.boundary`); startup fails if missing.

**Historical root cause (eph, Oct 2026):** Missing `script` in the image; exec transcripts never shipped. Secondary confusion: only reading `container/...` streams in the container log group.

## What exec logging captures

- Interactive shell and commands run from it (including `zoa run` stdout in the session).
- Claude Code TUI: partial terminal capture; Bedrock payloads → optional [model invocation logging](https://docs.aws.amazon.com/bedrock/latest/userguide/model-invocation-logging.html) (`enable_bedrock_logging`), not ECS exec logs.

## Task stop behavior (`stopTimeout`)

Fargate `stopTimeout` is **30 seconds** (same scale as the default Kubernetes `terminationGracePeriodSeconds` and the Fargate platform default). ZOA boundary has no persistent home volume or shutdown sync — only a grace period for SIGTERM and CloudWatch agent flush after the session ends.

## Verification after deploy

1. Rebuild boundary image; bump image tag; `terraform apply` (new log group `.../ssm-sessions`, task definition, IAM).
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

## ZOA TA audit (separate)

`zoa run` is recorded in **ZOA DynamoDB audit** regardless of CloudWatch exec logging. Use exec logs for shell forensics; use ZOA audit for TA proof.
