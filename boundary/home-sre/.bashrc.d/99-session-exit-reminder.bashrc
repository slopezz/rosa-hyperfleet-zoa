# Remind SREs to stop the boundary session from the laptop when ECS Exec ends.
# Does not call Access or stop the task — exit/Ctrl+D only disconnects the shell.

zoa_boundary_print_session_stop_hint() {
  local deployment="${ZOA_DEPLOYMENT:-<deployment>}"
  local session_id="${ZOA_SESSION_ID:-}"

  if [[ -z "${session_id}" ]]; then
    return 0
  fi

  printf '%s\n' '' '--- ZOA boundary ---' \
    'ECS Exec ended; the boundary task is still running.' \
    'On your laptop:' \
    "  zoa session stop ${deployment}/${session_id}" \
    "  zoa session list -d ${deployment}" \
    '---'
}

if [[ $- == *i* ]] && [[ -n "${ZOA_SESSION_ID:-}" ]]; then
  trap zoa_boundary_print_session_stop_hint EXIT
fi
