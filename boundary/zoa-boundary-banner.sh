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
  zoa_boundary_prompt_session_reason
  zoa_boundary_print_quick_start
  echo "==> Agent and session docs (Claude Code + humans)"
  echo "    ~/.claude/CLAUDE.md       — ROSA HyperFleet architecture, boundary rules, RC vs MC, identity bridge, TA workflow"
  echo "    ~/.claude/ZOA_SESSION.md  — this session (deployment, target, target type)"
  echo "    ~/.claude/ZOA_ACTIONS.md  — offline TA catalog (live API: zoa actions)"
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
  zoa_boundary_print_hard_and_inactivity_termination 1
  echo "    Terminate session:        zoa session terminate ${compound}"
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

# Column alignment for session limits / exit timeout lines (after 4-space indent).
zoa_boundary_session_limits_label_width() {
  echo 26
}

zoa_boundary_print_session_limits_continuation() {
  local text="${1:-}"
  local width
  width="$(zoa_boundary_session_limits_label_width)"
  printf '%*s%s\n' $((4 + width)) '' "${text}"
}

zoa_boundary_print_session_limits_label_value() {
  local label="${1:-}"
  local value="${2:-}"
  local width
  width="$(zoa_boundary_session_limits_label_width)"
  # shellcheck disable=SC2059
  printf '    %-'"${width}"'s%s\n' "${label}" "${value}"
}

# compact=1: one line each (Exec exit hint); compact=0: sub-lines (login limits).
zoa_boundary_print_hard_and_inactivity_termination() {
  local compact="${1:-0}"
  if [[ -n "${ZOA_SESSION_DEADLINE:-}" ]]; then
    zoa_boundary_print_session_limits_label_value "Hard termination:" "$(zoa_boundary_deadline_human "${ZOA_SESSION_DEADLINE}")"
    if [[ "${compact}" != "1" ]]; then
      zoa_boundary_print_session_limits_continuation "Max wall-clock from session start."
    fi
  elif [[ "${compact}" != "1" ]]; then
    zoa_boundary_print_session_limits_label_value "Hard termination:" "(see zoa session history on laptop)"
    zoa_boundary_print_session_limits_continuation "Max wall-clock from session start."
  fi
  if [[ -n "${ZOA_SESSION_IDLE_TIMEOUT_SECONDS:-}" ]]; then
    local idle_text
    idle_text="$(zoa_boundary_format_idle_timeout "${ZOA_SESSION_IDLE_TIMEOUT_SECONDS}") without Exec terminal activity"
    zoa_boundary_print_session_limits_label_value "Inactivity termination:" "${idle_text}"
    if [[ "${compact}" != "1" ]]; then
      zoa_boundary_print_session_limits_continuation "Task ends even if before the hard termination deadline."
    fi
  fi
}

zoa_boundary_print_quick_start() {
  echo "==> Quick start"
  if [[ -n "${ZOA_REASON:-}" ]] && zoa_boundary_reason_valid "${ZOA_REASON}"; then
    echo "    zoa run <action> ..."
  else
    echo "    zoa run <action> ... --reason ROSAENG-1234"
  fi
  echo ""
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
  echo "==> ZOA Boundary session limits"
  zoa_boundary_print_hard_and_inactivity_termination 0
  zoa_boundary_print_session_limits_label_value "Note:" "exit / Ctrl+D only disconnects Exec; terminate from laptop:"
  local compound=""
  if declare -F zoa_compound_session_id &>/dev/null; then
    compound="$(zoa_compound_session_id)"
  fi
  if [[ -n "${compound}" ]]; then
    zoa_boundary_print_session_limits_continuation "zoa session terminate ${compound}"
  else
    zoa_boundary_print_session_limits_continuation "zoa session terminate <deployment>/<session-id>"
  fi
  echo ""
}

zoa_boundary_reason_valid() {
  local value="${1:-}"
  [[ "${value}" =~ ^[A-Z][A-Z0-9]+-[0-9]+$ ]] && return 0
  [[ "${value}" =~ ^#[0-9]+$ ]] && return 0
  return 1
}

zoa_boundary_reason_get_from_md() {
  local path
  path="$(zoa_boundary_session_md_path)"
  if [[ ! -f "${path}" ]]; then
    return 1
  fi
  local line value
  while IFS= read -r line; do
    if [[ "${line}" =~ ^\|[[:space:]]*Reason[[:space:]]*\| ]]; then
      value="$(echo "${line}" | awk -F'|' '{print $3}' | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
      if [[ -z "${value}" ]] || [[ "${value}" == "*(not set)*" ]] || [[ "${value,,}" == "not set" ]]; then
        return 1
      fi
      if zoa_boundary_reason_valid "${value}"; then
        printf '%s' "${value}"
        return 0
      fi
      return 1
    fi
  done < "${path}"
  return 1
}

zoa_boundary_reason_set_in_md() {
  local value="${1:-}"
  local path
  path="$(zoa_boundary_session_md_path)"
  if [[ ! -f "${path}" ]]; then
    echo "Session file missing: ${path}" >&2
    return 1
  fi
  sed -i --regexp-extended "s/\\|[[:space:]]*Reason[[:space:]]*\\|[^|]*\\|/| Reason | ${value} |/" "${path}"
}

# Session default for zoa run: export ZOA_REASON (CLI) and mirror to ZOA_SESSION.md (Claude).
zoa_boundary_reason_apply() {
  local value="${1:-}"
  export ZOA_REASON="${value}"
  zoa_boundary_reason_set_in_md "${value}"
}

# Rejoin: new shell — reload ZOA_REASON from md written on this task.
zoa_boundary_reason_hydrate_from_md() {
  local value=""
  if ! value="$(zoa_boundary_reason_get_from_md)"; then
    return 0
  fi
  export ZOA_REASON="${value}"
}

zoa_boundary_prompt_session_reason() {
  # Task env from RunTask (session start --reason) is authoritative; md is for Claude.
  if [[ -n "${ZOA_REASON:-}" ]] && zoa_boundary_reason_valid "${ZOA_REASON}"; then
    zoa_boundary_reason_set_in_md "${ZOA_REASON}"
  else
    zoa_boundary_reason_hydrate_from_md
  fi

  local current="${ZOA_REASON:-}"
  if [[ -n "${current}" ]] && zoa_boundary_reason_valid "${current}"; then
    echo "==> Session reason"
    printf '    %s — default for every zoa run on this task (ZOA_REASON).\n' "${current}"
    echo ""
    printf '    %-32s%s\n' "reason <ticket>" "new session default"
    printf '    %-32s%s\n' "zoa run ... --reason <ticket>" "one-off override only"
    echo ""
    return 0
  fi

  echo "==> Session reason"
  echo "    Set a default for all zoa run in this session (including agents)."
  echo "    Press Enter to skip — then use zoa run ... --reason each time."
  echo ""
  local reply=""
  if [[ -t 0 ]]; then
    read -r -p "    Reason (e.g. Jira ROSAENG-1234 or PagerDuty #123456): " reply </dev/tty || read -r -p "    Reason (e.g. Jira ROSAENG-1234 or PagerDuty #123456): " reply
  else
    read -r -p "    Reason (e.g. Jira ROSAENG-1234 or PagerDuty #123456): " reply
  fi
  reply="$(echo "${reply}" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
  if [[ -z "${reply}" ]]; then
    echo "    No session default — use zoa run ... --reason <ticket> each time."
    echo ""
    return 0
  fi
  if ! zoa_boundary_reason_valid "${reply}"; then
    echo "    Not a valid reason (Jira issue ROSAENG-1234 or PagerDuty incident #123456). Session default not set." >&2
    echo ""
    return 1
  fi
  zoa_boundary_reason_apply "${reply}"
  printf '    Set session reason to %s (exported ZOA_REASON).\n' "${reply}"
  echo ""
  printf '    %-32s%s\n' "reason <ticket>" "new session default"
  printf '    %-32s%s\n' "zoa run ... --reason <ticket>" "one-off override only"
  echo ""
}

reason() {
  if [[ $# -eq 0 ]]; then
    zoa_boundary_reason_hydrate_from_md
    if [[ -n "${ZOA_REASON:-}" ]]; then
      printf 'Session reason: %s (ZOA_REASON)\n' "${ZOA_REASON}"
      return 0
    fi
    echo "No session reason set. Try: reason ROSAENG-1234  or  reason #123456"
    return 0
  fi
  local value="${1}"
  value="$(echo "${value}" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
  if ! zoa_boundary_reason_valid "${value}"; then
    echo "Not a valid reason (Jira issue ROSAENG-1234 or PagerDuty incident #123456)" >&2
    return 1
  fi
  local previous="${ZOA_REASON:-}"
  if [[ -z "${previous}" ]]; then
    previous="$(zoa_boundary_reason_get_from_md)" || previous=""
  fi
  if [[ "${previous}" == "${value}" ]]; then
    zoa_boundary_reason_apply "${value}"
    printf 'Already using session reason %s.\n' "${value}"
    return 0
  fi
  zoa_boundary_reason_apply "${value}"
  if [[ -n "${previous}" ]]; then
    printf 'Switched session reason: %s → %s\n' "${previous}" "${value}"
    return 0
  fi
  printf 'Set session reason to %s.\n' "${value}"
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
  echo "Execute TAs:   zoa run <action> ... --reason ROSAENG-1234"
  echo "List actions:  zoa actions  |  zoa catalog --offline  |  zoa describe <action>"
  echo ""
  echo "Boundary is ready. Waiting for ECS Exec connections..."
  echo "Container will stay running until the task is terminated."
  echo ""
}
