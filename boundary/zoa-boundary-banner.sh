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

  echo ""
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
  echo "    zoa run <action> ...   (session Jira set below; or --jira TICKET)"
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
  if [[ -n "${ZOA_SESSION_DEADLINE:-}" ]]; then
    zoa_boundary_print_deadline_line "    Hard stop (UTC):        "
  fi
  if [[ -n "${ZOA_SESSION_IDLE_TIMEOUT_SECONDS:-}" ]]; then
    printf '    Idle stop:              %s without Exec terminal activity\n' "$(zoa_boundary_format_idle_timeout "${ZOA_SESSION_IDLE_TIMEOUT_SECONDS}")"
  fi
  echo "    Stop session:             zoa session stop ${compound}"
  echo "    Re-join session:          zoa session join ${compound}"
  echo "    Your sessions:            zoa session list ${deployment}"
  echo "    All operators (audit):    zoa session history ${deployment}"
  echo ""
}

zoa_boundary_session_md_path() {
  echo "/home/sre/.claude/ZOA_SESSION.md"
}

zoa_boundary_format_idle_timeout() {
  local seconds="${1:-0}"
  if [[ ! "${seconds}" =~ ^[0-9]+$ ]] || [[ "${seconds}" -le 0 ]]; then
    echo "unknown"
    return 0
  fi
  if [[ "${seconds}" -ge 3600 ]] && [[ $((seconds % 3600)) -eq 0 ]]; then
    local hours=$((seconds / 3600))
    if [[ "${hours}" -eq 1 ]]; then
      echo "1 hour"
      return 0
    fi
    printf '%s hours' "${hours}"
    return 0
  fi
  if [[ "${seconds}" -ge 60 ]] && [[ $((seconds % 60)) -eq 0 ]]; then
    local minutes=$((seconds / 60))
    printf '%s minutes' "${minutes}"
    return 0
  fi
  printf '%s seconds' "${seconds}"
}

zoa_boundary_print_deadline_line() {
  local prefix="${1:-}"
  local deadline="${ZOA_SESSION_DEADLINE:-}"
  if [[ -z "${deadline}" ]]; then
    return 0
  fi
  local human
  human="$(zoa_boundary_deadline_human "${deadline}")"
  if [[ -n "${human}" ]]; then
    printf '%s%s\n' "${prefix}" "${human}"
  fi
}

zoa_boundary_deadline_human() {
  local deadline="${1:-}"
  local epoch_now epoch_end remaining
  epoch_now="$(date -u '+%s')"
  epoch_end="$(date -u -d "${deadline}" '+%s' 2>/dev/null || true)"
  if [[ -z "${epoch_end}" ]]; then
    printf '%s' "${deadline}"
    return 0
  fi
  remaining=$((epoch_end - epoch_now))
  if [[ "${remaining}" -le 0 ]]; then
    printf '%s (deadline passed)' "$(date -u -d "${deadline}" '+%Y-%m-%d %H:%M UTC' 2>/dev/null || echo "${deadline}")"
    return 0
  fi
  local hours=$((remaining / 3600))
  local minutes=$(((remaining % 3600) / 60))
  local stamp
  stamp="$(date -u -d "${deadline}" '+%Y-%m-%d %H:%M UTC' 2>/dev/null || echo "${deadline}")"
  if [[ "${hours}" -gt 0 ]]; then
    printf '%s (~%sh %sm from now)' "${stamp}" "${hours}" "${minutes}"
    return 0
  fi
  printf '%s (~%sm from now)' "${stamp}" "${minutes}"
}

zoa_boundary_print_session_limits() {
  echo "==> Session limits"
  if [[ -n "${ZOA_SESSION_DEADLINE:-}" ]]; then
    printf '    Hard stop:  %s\n' "$(zoa_boundary_deadline_human "${ZOA_SESSION_DEADLINE}")"
    echo "                — max session length from start"
  else
    echo "    Hard stop:  (see zoa session history on laptop)"
  fi
  if [[ -n "${ZOA_SESSION_IDLE_TIMEOUT_SECONDS:-}" ]]; then
    printf '    Idle stop:  %s without Exec terminal activity\n' "$(zoa_boundary_format_idle_timeout "${ZOA_SESSION_IDLE_TIMEOUT_SECONDS}")"
    echo "                — task stopped even if before the hard stop"
  fi
  echo "    Note:        exit / Ctrl+D only disconnects Exec; stop from your laptop:"
  local compound=""
  if declare -F zoa_compound_session_id &>/dev/null; then
    compound="$(zoa_compound_session_id)"
  fi
  if [[ -n "${compound}" ]]; then
    printf '                zoa session stop %s\n' "${compound}"
  else
    echo "                zoa session stop <deployment>/<session-id>"
  fi
  echo ""
}

zoa_boundary_jira_valid() {
  local ticket="${1:-}"
  [[ "${ticket}" =~ ^[A-Z][A-Z0-9]+-[0-9]+$ ]]
}

zoa_boundary_jira_get_from_md() {
  local path
  path="$(zoa_boundary_session_md_path)"
  if [[ ! -f "${path}" ]]; then
    return 1
  fi
  local line value
  while IFS= read -r line; do
    if [[ "${line}" =~ ^\|[[:space:]]*Jira[[:space:]]*\| ]]; then
      value="$(echo "${line}" | awk -F'|' '{print $3}' | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
      if [[ -z "${value}" ]] || [[ "${value}" == "*(not set)*" ]] || [[ "${value,,}" == "not set" ]]; then
        return 1
      fi
      if zoa_boundary_jira_valid "${value}"; then
        printf '%s' "${value}"
        return 0
      fi
      return 1
    fi
  done < "${path}"
  return 1
}

zoa_boundary_jira_set_in_md() {
  local ticket="${1:-}"
  local path
  path="$(zoa_boundary_session_md_path)"
  if [[ ! -f "${path}" ]]; then
    echo "Session file missing: ${path}" >&2
    return 1
  fi
  sed -i --regexp-extended "s/\\|[[:space:]]*Jira[[:space:]]*\\|[^|]*\\|/| Jira | ${ticket} |/" "${path}"
}

# Session default for zoa run: export ZOA_JIRA (CLI) and mirror to ZOA_SESSION.md (Claude).
zoa_boundary_jira_apply() {
  local ticket="${1:-}"
  export ZOA_JIRA="${ticket}"
  zoa_boundary_jira_set_in_md "${ticket}"
}

# Rejoin: new shell — reload ZOA_JIRA from md written on this task.
zoa_boundary_jira_hydrate_from_md() {
  local ticket=""
  if ! ticket="$(zoa_boundary_jira_get_from_md)"; then
    return 0
  fi
  export ZOA_JIRA="${ticket}"
}

zoa_boundary_prompt_session_jira() {
  # Task env from RunTask (session start --jira) is authoritative; md is for Claude.
  if [[ -n "${ZOA_JIRA:-}" ]] && zoa_boundary_jira_valid "${ZOA_JIRA}"; then
    zoa_boundary_jira_set_in_md "${ZOA_JIRA}"
  else
    zoa_boundary_jira_hydrate_from_md
  fi

  local current="${ZOA_JIRA:-}"
  if [[ -n "${current}" ]] && zoa_boundary_jira_valid "${current}"; then
    echo "==> Session Jira"
    printf '    Using %s for zoa run (ZOA_JIRA; saved for this boundary task).\n' "${current}"
    echo "    Change:  jira <ticket>     One-off:  zoa run ... --jira <ticket>"
    echo ""
    return 0
  fi

  echo "==> Session Jira"
  echo "    Default for every zoa run in this session (including Claude)."
  echo "    Press Enter to skip — then use zoa run ... --jira TICKET each time."
  echo ""
  local reply=""
  if [[ -t 0 ]]; then
    read -r -p "    Jira (e.g. ROSAENG-1234): " reply </dev/tty || read -r -p "    Jira (e.g. ROSAENG-1234): " reply
  else
    read -r -p "    Jira (e.g. ROSAENG-1234): " reply
  fi
  reply="$(echo "${reply}" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
  if [[ -z "${reply}" ]]; then
    echo "    No session Jira set."
    echo ""
    return 0
  fi
  if ! zoa_boundary_jira_valid "${reply}"; then
    echo "    Invalid format (use PROJECT-123). No session Jira set." >&2
    echo ""
    return 1
  fi
  zoa_boundary_jira_apply "${reply}"
  printf '    Set session Jira to %s (exported ZOA_JIRA).\n' "${reply}"
  echo "    Change:  jira <ticket>     One-off:  zoa run ... --jira <ticket>"
  echo ""
}

jira() {
  if [[ $# -eq 0 ]]; then
    zoa_boundary_jira_hydrate_from_md
    if [[ -n "${ZOA_JIRA:-}" ]]; then
      printf 'Session Jira: %s (ZOA_JIRA)\n' "${ZOA_JIRA}"
      return 0
    fi
    echo "No session Jira set. Example: jira ROSAENG-1234"
    return 0
  fi
  local ticket="${1}"
  ticket="$(echo "${ticket}" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
  if ! zoa_boundary_jira_valid "${ticket}"; then
    echo "Invalid Jira format (use PROJECT-123, e.g. ROSAENG-1234)" >&2
    return 1
  fi
  local previous="${ZOA_JIRA:-}"
  if [[ -z "${previous}" ]]; then
    previous="$(zoa_boundary_jira_get_from_md)" || previous=""
  fi
  if [[ "${previous}" == "${ticket}" ]]; then
    zoa_boundary_jira_apply "${ticket}"
    printf 'Already using session Jira %s.\n' "${ticket}"
    return 0
  fi
  zoa_boundary_jira_apply "${ticket}"
  if [[ -n "${previous}" ]]; then
    printf 'Switched session Jira: %s → %s\n' "${previous}" "${ticket}"
    return 0
  fi
  printf 'Set session Jira to %s.\n' "${ticket}"
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
