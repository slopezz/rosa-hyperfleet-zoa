# Two-line PS1 for ECS Exec: session id (audit handle) + operator@deployment/target.
# Env vars are set at RunTask (ZOA_SESSION_ID, ZOA_OPERATOR) and in the task definition.

zoa_prompt_primary() {
  local line
  line="$(zoa_prompt_session_line)"
  if declare -F zoa_breakglass_ps1_suffix &>/dev/null; then
    line="${line}$(zoa_breakglass_ps1_suffix)"
  fi
  printf '%s' "${line}"
}

zoa_prompt_secondary() {
  local op="${ZOA_OPERATOR:-sre}"
  local dep="${ZOA_DEPLOYMENT:-unknown}"
  local tgt="${ZOA_TARGET:-unknown}"
  printf '%s@zoa:%s/%s' "${op}" "${dep}" "${tgt}"
}

if [[ -n "${ZOA_SESSION_ID:-}" || -n "${ZOA_DEPLOYMENT:-}" ]]; then
  PS1='$(zoa_prompt_primary)
$(zoa_prompt_secondary) \w \$ '
fi
