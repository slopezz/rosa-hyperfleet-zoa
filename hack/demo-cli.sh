#!/usr/bin/env bash
# Demo script for the ZOA CLI.
# Exercises every command and feature against a live Lambda endpoint.
#
# Prerequisites:
#   - AWS credentials configured for the target account (SigV4 auth)
#   - ZOA_API_URL set to the Lambda Function URL
#   - ./bin/zoa built (make build)
#
# Usage:
#   export ZOA_API_URL=https://<id>.lambda-url.<region>.on.aws/
#   ./hack/demo-cli.sh [--dry-run] [--step]
#
# With --dry-run, only local commands run (no API calls).
# With --step, pauses for Enter between each step (interactive demo).

set -euo pipefail

ZOA="${ZOA_BIN:-./bin/zoa}"
DRY_RUN=false
STEP_MODE=false
for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=true ;;
    --step)    STEP_MODE=true ;;
  esac
done

DIVIDER="━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
STEP=0
TOTAL_STEPS=0

count_steps() {
  TOTAL_STEPS=$(grep -cE '^\s*run_cmd(_capture)?\s+"' "$0")
}
count_steps

next_step() {
  STEP=$((STEP + 1))
}

wait_for_enter() {
  if [[ "$STEP_MODE" == "true" ]]; then
    echo -n "  [Press Enter to continue] "
    read -r
  fi
}

run_cmd() {
  local desc="$1"; shift
  next_step
  echo ""
  echo "$DIVIDER"
  echo "▶ [$STEP/$TOTAL_STEPS] $desc"
  echo "$ $*"
  echo "$DIVIDER"
  "$@" 2>&1 || true
  echo ""
  wait_for_enter
}

run_cmd_capture() {
  local desc="$1"; shift
  next_step
  echo ""
  echo "$DIVIDER"
  echo "▶ [$STEP/$TOTAL_STEPS] $desc"
  echo "$ $*"
  echo "$DIVIDER"
  local errfile
  errfile=$(mktemp)
  "$@" 2> >(tee "$errfile" >&2) || true
  capture_exec_id "$(cat "$errfile")"
  rm -f "$errfile"
  echo ""
  wait_for_enter
}

capture_exec_id() {
  local output="$1"
  local id
  id=$(echo "$output" | grep -oP '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}' | head -1)
  if [[ -n "$id" ]]; then
    EXEC_ID="$id"
    echo "  [captured exec ID: $EXEC_ID]"
  fi
}

requires_api() {
  if [[ "$DRY_RUN" == "true" ]]; then
    echo "  [SKIPPED — dry-run mode]"
    return 1
  fi
  if [[ -z "${ZOA_API_URL:-}" ]]; then
    echo "  [SKIPPED — ZOA_API_URL not set]"
    return 1
  fi
  return 0
}

echo "╔══════════════════════════════════════════════════════════════════════════════╗"
echo "║                        ZOA CLI — Full Demo Session                         ║"
echo "╠══════════════════════════════════════════════════════════════════════════════╣"
echo "║  Binary:       $ZOA"
echo "║  ZOA_API_URL:  ${ZOA_API_URL:-<not set>}"
echo "║  Dry-run:      $DRY_RUN"
echo "╚══════════════════════════════════════════════════════════════════════════════╝"

EXEC_ID=""
ASYNC_ID=""

# ═══════════════════════════════════════════════════════════════════════════════
# Section 1: Help & Version
# ═══════════════════════════════════════════════════════════════════════════════

run_cmd "Root help" $ZOA --help
run_cmd "Version (client + server)" $ZOA version

# ═══════════════════════════════════════════════════════════════════════════════
# Section 2: Actions Catalog
# ═══════════════════════════════════════════════════════════════════════════════

if requires_api; then
  run_cmd "List all Trusted Actions" $ZOA actions
  run_cmd "Describe a read TA" $ZOA describe get_resource
  run_cmd "Describe a write TA" $ZOA describe rollout_restart
  run_cmd "Describe an AWS TA (JSON)" $ZOA describe describe_eks_cluster -o json
fi

# ═══════════════════════════════════════════════════════════════════════════════
# Section 3: Sync Read — Kubernetes TAs
# TA output is JSON by design (structured data for automation). Pipe to jq for
# human-readable formatting, or use 'zoa download' to save to a file.
# ═══════════════════════════════════════════════════════════════════════════════

if requires_api; then
  run_cmd "Kube read — pods in kube-system" \
    $ZOA run get_resource --reason DEMO-001 --namespace kube-system --resource pods

  run_cmd "Kube read — deployments in argocd" \
    $ZOA run get_resource --reason DEMO-002 --namespace argocd --resource deployments

  run_cmd "Kube read — nodes" \
    $ZOA run get_resource --reason DEMO-003 --resource nodes

  # get_secret shows metadata only by default; add -v/--verbose to see base64 key names
  run_cmd "Kube read — secret (metadata only; use -v for values)" \
    $ZOA run get_secret --reason DEMO-004 --namespace argocd --name argocd-secret
fi

# ═══════════════════════════════════════════════════════════════════════════════
# Section 4: Sync Read — AWS TAs
# ═══════════════════════════════════════════════════════════════════════════════

if requires_api; then
  run_cmd "AWS read — list EKS clusters" \
    $ZOA run list_eks_clusters --reason DEMO-010

  # Use the target from version output to get a valid cluster name
  CLUSTER_NAME=$($ZOA version 2>&1 | grep "Target:" | awk '{print $2}')
  if [[ -n "$CLUSTER_NAME" ]]; then
    run_cmd "AWS read — describe EKS cluster" \
      $ZOA run describe_eks_cluster --reason DEMO-011 --name "$CLUSTER_NAME"
  fi

  run_cmd "AWS read — describe VPC endpoint (invalid ID → expected error)" \
    $ZOA run describe_vpc_endpoint --reason DEMO-012 --name vpce-does-not-exist
fi

# ═══════════════════════════════════════════════════════════════════════════════
# Section 5: Sync Write — Kubernetes TAs
# ═══════════════════════════════════════════════════════════════════════════════

if requires_api; then
  run_cmd "Write — rollout_restart dry-run (preview via get_resource)" \
    $ZOA run rollout_restart --reason DEMO-020 --namespace kube-system --resource deployment --name coredns --dry-run

  run_cmd "Write — rollout_restart (real)" \
    $ZOA run rollout_restart --reason DEMO-021 --namespace kube-system --resource deployment --name coredns

  run_cmd "Write — rollout_restart (hits write cooldown)" \
    $ZOA run rollout_restart --reason DEMO-022 --namespace kube-system --resource deployment --name coredns

  run_cmd "Write — rollout_restart with --force (bypass cooldown + max concurrent)" \
    $ZOA run rollout_restart --reason DEMO-023 --namespace kube-system --resource deployment --name coredns --force
fi

# ═══════════════════════════════════════════════════════════════════════════════
# Section 6: Async Execution
# ═══════════════════════════════════════════════════════════════════════════════

if requires_api; then
  run_cmd_capture "Async — fire and forget (get nodes)" \
    $ZOA run get_resource --reason DEMO-030 --resource nodes --execution-mode async
  ASYNC_ID="$EXEC_ID"

  # One --wait call (~1min for the EventBridge reconciler tick).
  run_cmd "Async — write TA (rollout_restart, forced)" \
    $ZOA run rollout_restart --reason DEMO-033 --namespace kube-system --resource deployment --name coredns --execution-mode async --force --wait
fi

# ═══════════════════════════════════════════════════════════════════════════════
# Section 7: Get / Output / Logs / Download
# ═══════════════════════════════════════════════════════════════════════════════

if requires_api; then
  if [[ -z "$EXEC_ID" ]]; then
    EXEC_ID="${ZOA_DEMO_EXEC_ID:-}"
  fi

  if [[ -n "$EXEC_ID" ]]; then
    run_cmd "Get — execution metadata (table)" $ZOA get "$EXEC_ID"
    run_cmd "Get — with output included" $ZOA get "$EXEC_ID" --include-output
    run_cmd "Get — with logs included" $ZOA get "$EXEC_ID" --include-logs
    run_cmd "Get — full details (output + logs)" $ZOA get "$EXEC_ID" --include-all
    run_cmd "Get — JSON format" $ZOA get "$EXEC_ID" -o json
    run_cmd "Output — raw TA output (stdout)" $ZOA output "$EXEC_ID"
    run_cmd "Logs — raw execution log" $ZOA logs "$EXEC_ID"
    run_cmd "Download — save output to file" $ZOA download "$EXEC_ID"
  else
    echo "  [SKIPPED — no execution ID captured; set ZOA_DEMO_EXEC_ID]"
  fi
fi

# ═══════════════════════════════════════════════════════════════════════════════
# Section 8: Runs (execution history) — output formats
# ═══════════════════════════════════════════════════════════════════════════════

if requires_api; then
  run_cmd "Runs — default table" $ZOA runs --limit 10
  run_cmd "Runs — wide (all columns, timestamps, bytes)" $ZOA runs --limit 10 -o wide
  run_cmd "Runs — failed only" $ZOA runs --status failed --since 1h
  run_cmd "Runs — writes only" $ZOA runs --type write --since 1h
  run_cmd "Runs — async only" $ZOA runs --execution-mode async --since 1h
  run_cmd "Runs — dry-run only" $ZOA runs --dry-run --since 1h
  run_cmd "Runs — by action name" $ZOA runs --action get_resource --since 1h --limit 5
  run_cmd "Runs — by scope (aws-api)" $ZOA runs --scope aws-api --since 1h
  run_cmd "Runs — JSON with jq (slow runs >5s)" \
    bash -c "$ZOA runs -o json --since 1h | jq '[.items[] | select(.duration_ms > 5000) | {id, action, duration_ms, execution_mode}]'"
fi

# ═══════════════════════════════════════════════════════════════════════════════
# Section 9: Audit Log
# ═══════════════════════════════════════════════════════════════════════════════

if requires_api; then
  run_cmd "Audit — recent entries" $ZOA audit --limit 15
  run_cmd "Audit — POST only (dispatches)" $ZOA audit --method POST --since 1h
  run_cmd "Audit — errors (4xx/5xx)" \
    bash -c "$ZOA audit -o json --since 1h | jq '[.items[] | select(.status_code >= 400) | {timestamp, method, path, status_code, action}]'"
fi

# ═══════════════════════════════════════════════════════════════════════════════
# Section 10: Completion & Subcommand Help
# ═══════════════════════════════════════════════════════════════════════════════

run_cmd "Help — run (all flags)" $ZOA run --help
run_cmd "Help — runs (filters)" $ZOA runs --help
run_cmd "Help — get" $ZOA get --help
run_cmd "Help — download" $ZOA download --help
run_cmd "Help — audit" $ZOA audit --help

echo ""
echo "╔══════════════════════════════════════════════════════════════════════════════╗"
echo "║                         Demo session complete ✓                            ║"
echo "╠══════════════════════════════════════════════════════════════════════════════╣"
if [[ -n "${ASYNC_ID:-}" ]]; then
echo "║  Async ID: $ASYNC_ID"
echo "║  Try: zoa get $ASYNC_ID --include-all"
fi
echo "╚══════════════════════════════════════════════════════════════════════════════╝"
