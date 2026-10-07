# Session limits and Jira prompt after MOTD (15-zoa-motd.bashrc).

ZOA_BOUNDARY_BANNER="/etc/zoa-boundary/banner.sh"

zoa_boundary_session_login_once() {
  if [[ -n "${ZOA_SESSION_LOGIN_SHOWN:-}" ]]; then
    return 0
  fi
  if [[ ! -f "${ZOA_BOUNDARY_BANNER}" ]]; then
    return 0
  fi
  # shellcheck source=/dev/null
  source "${ZOA_BOUNDARY_BANNER}"
  zoa_boundary_print_session_limits
  zoa_boundary_prompt_session_jira
  export ZOA_SESSION_LOGIN_SHOWN=1
}

if [[ $- == *i* ]] && [[ -n "${ZOA_SESSION_ID:-}" ]]; then
  zoa_boundary_session_login_once
fi
