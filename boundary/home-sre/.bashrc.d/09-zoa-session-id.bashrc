# Compound session id for CLI (deployment/uuid). ZOA_SESSION_ID in the task is the raw uuid.

zoa_compound_session_id() {
  local deployment="${ZOA_DEPLOYMENT:-}"
  local raw="${ZOA_SESSION_ID:-}"

  if [[ -z "${raw}" ]]; then
    return 0
  fi
  if [[ "${raw}" == */* ]]; then
    printf '%s' "${raw}"
    return 0
  fi
  if [[ -n "${deployment}" ]]; then
    printf '%s/%s' "${deployment}" "${raw}"
    return 0
  fi
  printf '%s' "${raw}"
}

zoa_prompt_session_line() {
  local compound
  compound="$(zoa_compound_session_id)"
  if [[ -z "${compound}" ]]; then
    printf 'sessionId:unknown'
    return 0
  fi
  printf 'sessionId:%s' "${compound}"
}
