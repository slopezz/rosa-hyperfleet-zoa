# Session limits after MOTD (Jira block is in zoa_boundary_print_motd; 15-zoa-motd.bashrc).

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
  export ZOA_SESSION_LOGIN_SHOWN=1
}

if [[ $- == *i* ]] && [[ -n "${ZOA_SESSION_ID:-}" ]]; then
  zoa_boundary_session_login_once
fi
