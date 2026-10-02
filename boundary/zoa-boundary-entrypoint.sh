#!/bin/bash
# ZOA Boundary ECS task entrypoint — session bootstrap then keep-alive for ECS Exec.
# Runtime env (ZOA_API_URL, ZOA_TARGET, ZOA_DEPLOYMENT, AWS_REGION, ANTHROPIC_MODEL, …)
# is injected by the ECS task definition (Terraform); this script only consumes it.

set -euo pipefail

export PATH="/usr/local/bin:/usr/local/aws-cli/v2/current/bin:/usr/bin:/bin"

write_zoa_session_md() {
  mkdir --parents /home/sre/.claude
  {
    echo "# Active ZOA Boundary session"
    echo ""
    echo "| Field | Value |"
    echo "|-------|-------|"
    echo "| Deployment | ${ZOA_DEPLOYMENT:-unknown} |"
    echo "| Target | ${ZOA_TARGET:-unknown} |"
    echo "| AWS region | ${AWS_REGION:-unknown} |"
    echo "| ZOA API | ${ZOA_API_URL:-unknown} |"
    echo ""
    echo "See CLAUDE.md for architecture and allowed tools."
  } > /home/sre/.claude/ZOA_SESSION.md
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
  echo "=== ZOA Boundary Session ==="
  echo "Started at $(date -u '+%Y-%m-%d %H:%M:%S UTC')"
  echo "Cluster:    ${ZOA_TARGET:-unknown}"
  echo "Deployment: ${ZOA_DEPLOYMENT:-unknown}"
  echo "User:       $(id -un) (uid=$(id -u))"
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

  if [ -n "${ZOA_DEPLOYMENT:-}" ] && [ -n "${ZOA_TARGET:-}" ]; then
    export PS1="[\u@zoa:${ZOA_DEPLOYMENT}/${ZOA_TARGET}] \w \$ "
  fi

  echo "=== Boundary ready for connections ==="
  echo "Session facts: /home/sre/.claude/ZOA_SESSION.md"
  echo "Execute TAs with: zoa run <action> [args] --jira TICKET"
  echo "List actions:     zoa actions"
  echo ""
  echo "Boundary is ready. Waiting for ECS Exec connections..."
  echo "Container will stay running until the task is stopped."
  echo ""
}

main() {
  write_zoa_session_md
  require_ecs_exec_logging_bins
  print_startup_banner
  exec "$@"
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  main "$@"
fi
