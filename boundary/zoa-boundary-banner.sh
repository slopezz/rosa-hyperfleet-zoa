#!/bin/bash
# Shared ZOA Boundary banners: MOTD (ECS Exec), container startup log, exit hints.
# UTF-8 block-letter banner; no ANSI color codes (FedRAMP-friendly transcripts).

zoa_boundary_ascii_art() {
  cat <<'EOF'
███████╗ ██████╗  █████╗
╚══███╔╝██╔═══██╗██╔══██╗
  ███╔╝ ██║   ██║███████║
 ███╔╝  ██║   ██║██╔══██║
███████╗╚██████╔╝██║  ██║
╚══════╝ ╚═════╝ ╚═╝  ╚═╝
EOF
}

zoa_boundary_target_type() {
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

# Requires zoa_compound_session_id from 09-zoa-session-id.bashrc when used from shell MOTD.
zoa_boundary_print_motd() {
  local operator target_type compound deployment target region
  operator="${ZOA_OPERATOR:-sre}"
  deployment="${ZOA_DEPLOYMENT:-unknown}"
  target="${ZOA_TARGET:-unknown}"
  region="${AWS_REGION:-unknown}"
  target_type="$(zoa_boundary_target_type)"
  compound=""
  if declare -F zoa_compound_session_id &>/dev/null; then
    compound="$(zoa_compound_session_id)"
  elif [[ -n "${ZOA_SESSION_ID:-}" ]]; then
    if [[ "${ZOA_SESSION_ID}" == */* ]]; then
      compound="${ZOA_SESSION_ID}"
    elif [[ -n "${deployment}" && "${deployment}" != "unknown" ]]; then
      compound="${deployment}/${ZOA_SESSION_ID}"
    else
      compound="${ZOA_SESSION_ID}"
    fi
  fi

  zoa_boundary_ascii_art
  echo ""
  printf 'Hello, %s\n' "${operator}"
  echo ""
  echo "==> ZOA Boundary session"
  printf '    Operator:    %s\n' "${operator}"
  if [[ -n "${compound}" ]]; then
    printf '    Session:     %s\n' "${compound}"
  fi
  printf '    Deployment:  %s\n' "${deployment}"
  printf '    Target:      %s  (type: %s)\n' "${target}" "${target_type}"
  printf '    Region:      %s\n' "${region}"
  echo ""
  echo "    Audited container in the target VPC. Run Trusted Actions with zoa run;"
  echo "    operator attribution uses the identity bridge."
  echo ""
  echo "==> Quick start"
  echo "    zoa run <action> ... --jira ROSAENG-1234"
  echo ""
  echo "==> Agent and session docs (Claude Code + humans)"
  echo "    ~/.claude/CLAUDE.md       — ROSA HyperFleet architecture, boundary rules, RC vs MC, identity bridge, TA workflow"
  echo "    ~/.claude/ZOA_SESSION.md  — this session (deployment, target, target type)"
  echo "    ~/.claude/ZOA_ACTIONS.md  — offline TA catalog (live API: zoa actions)"
  echo ""
  echo "==> Remember"
  echo "    exit / Ctrl+D disconnects Exec only — stop from your laptop:"
  if [[ -n "${compound}" ]]; then
    printf '    zoa session stop %s\n' "${compound}"
  else
    echo "    zoa session stop <deployment>/<session-id>"
  fi
  echo ""
}

zoa_boundary_print_exit_hint() {
  local deployment compound
  deployment="${ZOA_DEPLOYMENT:-<deployment>}"
  compound=""
  if declare -F zoa_compound_session_id &>/dev/null; then
    compound="$(zoa_compound_session_id)"
  fi

  if [[ -z "${compound}" ]]; then
    return 0
  fi

  echo ""
  echo "==> ZOA boundary — Exec ended (task still running)"
  echo "    Stop session:             zoa session stop ${compound}"
  echo "    Re-join session:          zoa session join ${compound}"
  echo "    Your sessions:            zoa session list ${deployment}"
  echo "    All operators (audit):    zoa session history ${deployment}"
  echo ""
}

zoa_boundary_print_container_startup_banner() {
  local target_type
  target_type="$(zoa_boundary_target_type)"
  zoa_boundary_ascii_art
  echo ""
  echo "=== ZOA Boundary Session (container startup) ==="
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
  echo "Docs: CLAUDE.md, ZOA_SESSION.md, ZOA_ACTIONS.md under /home/sre/.claude/"
  echo "Execute TAs:   zoa run <action> ... --jira ROSAENG-1234"
  echo "List actions:  zoa actions  |  zoa catalog --offline  |  zoa describe <action>"
  echo ""
  echo "Boundary is ready. Waiting for ECS Exec connections..."
  echo "Container will stay running until the task is stopped."
  echo ""
}
