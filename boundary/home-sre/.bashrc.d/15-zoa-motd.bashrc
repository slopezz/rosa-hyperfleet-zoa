# MOTD on interactive ECS Exec login (plain ASCII ZOA + session context).
# Runs after 09-zoa-session-id (compound session id).

ZOA_BOUNDARY_BANNER="/etc/zoa-boundary/banner.sh"

zoa_boundary_maybe_print_motd() {
  if [[ -n "${ZOA_MOTD_SHOWN:-}" ]]; then
    return 0
  fi
  if [[ ! -f "${ZOA_BOUNDARY_BANNER}" ]]; then
    return 0
  fi
  # shellcheck source=/dev/null
  source "${ZOA_BOUNDARY_BANNER}"
  zoa_boundary_print_motd
  export ZOA_MOTD_SHOWN=1
}

if [[ $- == *i* ]] && [[ -n "${ZOA_SESSION_ID:-}" ]]; then
  zoa_boundary_maybe_print_motd
fi
