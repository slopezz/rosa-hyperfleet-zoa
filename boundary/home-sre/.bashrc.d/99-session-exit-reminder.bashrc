# Remind SREs to stop the boundary session from the laptop when ECS Exec ends.
# Does not call Access or stop the task — exit/Ctrl+D only disconnects the shell.

ZOA_BOUNDARY_BANNER="/etc/zoa-boundary/banner.sh"

zoa_boundary_print_session_stop_hint() {
  if [[ ! -f "${ZOA_BOUNDARY_BANNER}" ]]; then
    return 0
  fi
  # shellcheck source=/dev/null
  source "${ZOA_BOUNDARY_BANNER}"
  zoa_boundary_print_exit_hint
}

if [[ $- == *i* ]] && [[ -n "${ZOA_SESSION_ID:-}" ]]; then
  trap zoa_boundary_print_session_stop_hint EXIT
fi
