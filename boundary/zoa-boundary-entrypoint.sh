#!/bin/bash
# ZOA Boundary ECS task entrypoint — session bootstrap then keep-alive for ECS Exec.
# Runtime env (ZOA_API_URL, ZOA_TARGET, ZOA_DEPLOYMENT, ZOA_DEPLOYMENT_TARGET,
# ZOA_SESSION_ID, ZOA_OPERATOR, …) is injected at RunTask (Access Lambda) and/or the task definition (Terraform).

set -euo pipefail

export PATH="/usr/local/bin:/usr/local/aws-cli/v2/current/bin:/usr/bin:/bin"

zoa_session_md_path() {
  echo "/home/sre/.claude/ZOA_SESSION.md"
}

zoa_actions_catalog_path() {
  echo "/home/sre/.claude/ZOA_ACTIONS.md"
}

baked_actions_catalog_source() {
  local target="${ZOA_DEPLOYMENT_TARGET:-mc}"
  echo "/usr/share/zoa/catalog/ZOA_ACTIONS.${target}.md"
}

write_zoa_session_md() {
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
    echo "| Deployment target | ${ZOA_DEPLOYMENT_TARGET:-unknown} |"
    echo "| AWS region | ${AWS_REGION:-unknown} |"
    echo "| ZOA API | ${ZOA_API_URL:-unknown} |"
    echo ""
    echo "Trusted Actions catalog: \`$(zoa_actions_catalog_path)\` (baked at image build for \`${ZOA_DEPLOYMENT_TARGET:-mc}\`)."
    echo "Live details: \`zoa describe <action>\` · JSON: \`zoa describe <action> -o json\`"
    echo "See CLAUDE.md for CLI rules, Jira format, and working style."
  } > "$(zoa_session_md_path)"
}

install_actions_catalog() {
  local catalog_path source_path
  catalog_path="$(zoa_actions_catalog_path)"
  source_path="$(baked_actions_catalog_source)"
  mkdir --parents /home/sre/.claude

  if [[ -f "${source_path}" ]]; then
    cp "${source_path}" "${catalog_path}"
    return 0
  fi

  {
    echo "# ZOA actions catalog"
    echo ""
    echo "_Baked catalog missing at \`${source_path}\` — use \`zoa actions\` and \`zoa describe <action>\`._"
    echo ""
    echo "Set \`ZOA_DEPLOYMENT_TARGET\` to \`rc\` or \`mc\` when starting the task (Access Lambda sets this from target metadata)."
  } > "${catalog_path}"
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
  echo "Session:    ${ZOA_SESSION_ID:-unknown}"
  echo "Operator:   ${ZOA_OPERATOR:-unknown}"
  echo "Deployment: ${ZOA_DEPLOYMENT:-unknown}"
  echo "Target:     ${ZOA_TARGET:-unknown}"
  echo "TA target:  ${ZOA_DEPLOYMENT_TARGET:-mc} (rc|mc catalog)"
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
  echo "List actions:  zoa actions  |  zoa describe <action>"
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
