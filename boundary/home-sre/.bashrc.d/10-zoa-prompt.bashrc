# Two-line PS1 for ECS Exec: session id (audit handle) + operator@deployment/target.
# Env vars are set at RunTask (ZOA_SESSION_ID, ZOA_OPERATOR) and in the task definition.

zoa_prompt_primary() {
  local sid="${ZOA_SESSION_ID:-session:unknown}"
  if [[ "${sid}" != session:* ]]; then
    sid="session:${sid}"
  fi
  printf '%s' "${sid}"
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
