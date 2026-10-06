#!/bin/bash
# ZOA Boundary ECS task entrypoint — session bootstrap then keep-alive for ECS Exec.
# Runtime env (ZOA_API_URL, ZOA_TARGET, ZOA_DEPLOYMENT, ZOA_TARGET_TYPE,
# ZOA_SESSION_ID, ZOA_OPERATOR, …) is injected at RunTask (Access Lambda) and/or the task definition (Terraform).

set -euo pipefail

export PATH="/usr/local/bin:/usr/local/aws-cli/v2/current/bin:/usr/bin:/bin"

zoa_session_md_path() {
  echo "/home/sre/.claude/ZOA_SESSION.md"
}

zoa_actions_catalog_path() {
  echo "/home/sre/.claude/ZOA_ACTIONS.md"
}

zoa_target_type() {
  if [[ -n "${ZOA_TARGET_TYPE:-}" ]]; then
    echo "${ZOA_TARGET_TYPE}"
    return 0
  fi
  if [[ -n "${ZOA_DEPLOYMENT_TARGET:-}" ]]; then
    echo "${ZOA_DEPLOYMENT_TARGET}"
    return 0
  fi
  echo "mc"
}

write_zoa_session_md() {
  local target_type
  target_type="$(zoa_target_type)"
  mkdir --parents /home/sre/.claude
  {
    echo "# Active ZOA Boundary session"
    echo ""
    echo "| Field | Value |"
    echo "|-------|-------|"
    echo "| Session ID | ${ZOA_SESSION_ID:-unknown} |"
    echo "| Operator | ${ZOA_OPERATOR:-unknown} |"
    echo "| Deployment | ${ZOA_DEPLOYMENT:-unknown} |"
    echo "| Target | ${ZOA_TARGET:-unknown} |"
    echo "| Target type | ${target_type} |"
    echo "| AWS region | ${AWS_REGION:-unknown} |"
    echo "| ZOA API | ${ZOA_API_URL:-unknown} |"
    echo ""
    echo "**Target type \`${target_type}\`** matches **TYPE** in \`zoa targets\` — \`rc\` = Regional Cluster EKS (one per region); \`mc\` = Management Cluster EKS (HyperShift; may be several per region). kube-api TAs use this cluster; aws-api TAs use this cluster's AWS account/region."
    echo ""
    echo "Trusted Actions catalog: \`$(zoa_actions_catalog_path)\` (offline render from embedded CLI registry; live API is authoritative when connected)."
    echo "Live details: \`zoa describe <action>\` · Offline: \`zoa actions --offline\` · See CLAUDE.md"
  } > "$(zoa_session_md_path)"
}

install_actions_catalog() {
  local catalog_path
  catalog_path="$(zoa_actions_catalog_path)"
  mkdir --parents /home/sre/.claude

  if ! command -v zoa &>/dev/null; then
    echo "FATAL: zoa CLI not found — cannot render offline actions catalog" >&2
    exit 1
  fi

  if ! zoa actions --offline -o markdown >"${catalog_path}"; then
    echo "FATAL: zoa actions --offline -o markdown failed" >&2
    exit 1
  fi
}

require_ecs_exec_logging_bins() {
  local bin
  for bin in script cat; do
    if ! command -v "${bin}" &>/dev/null; then
      echo "FATAL: missing ${bin} — ECS Exec cannot upload session transcripts to CloudWatch" >&2
      exit 1
    fi
  done
}

print_startup_banner() {
  local target_type
  target_type="$(zoa_target_type)"
  echo "=== ZOA Boundary Session ==="
  echo "Started at $(date -u '+%Y-%m-%d %H:%M:%S UTC')"
  echo "Session:    ${ZOA_SESSION_ID:-unknown}"
  echo "Operator:   ${ZOA_OPERATOR:-unknown}"
  echo "Deployment: ${ZOA_DEPLOYMENT:-unknown}"
  echo "Target:     ${ZOA_TARGET:-unknown}"
  echo "Target type: ${target_type} (rc|mc; TYPE in zoa targets)"
  echo "Unix user:  $(id -un) (uid=$(id -u))"
  echo ""

  echo "Available tools:"
  local tool
  for tool in zoa kubectl jq claude script; do
    if command -v "${tool}" &>/dev/null; then
      echo "  - ${tool}"
    else
      echo "  - ${tool} (not found)"
    fi
  done
  if /usr/local/bin/aws --version &>/dev/null; then
    echo "  - aws"
  else
    echo "  - aws (not found at /usr/local/bin/aws)"
  fi
  echo ""

  echo "=== Boundary ready for connections ==="
  echo "Session facts: $(zoa_session_md_path)"
  echo "TA catalog:    $(zoa_actions_catalog_path)"
  echo "Execute TAs:   zoa run <action> ... --jira ROSAENG-1234"
  echo "List actions:  zoa actions  |  zoa catalog --offline  |  zoa describe <action>"
  echo ""
  echo "Boundary is ready. Waiting for ECS Exec connections..."
  echo "Container will stay running until the task is stopped."
  echo ""
}

main() {
  write_zoa_session_md
  install_actions_catalog
  require_ecs_exec_logging_bins
  print_startup_banner
  exec "$@"
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  main "$@"
fi
