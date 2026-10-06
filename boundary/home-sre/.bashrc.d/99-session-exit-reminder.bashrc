# Remind SREs to stop the boundary session from the laptop when ECS Exec ends.
# Does not call Access or stop the task — exit/Ctrl+D only disconnects the shell.

zoa_boundary_print_session_stop_hint() {
  local deployment="${ZOA_DEPLOYMENT:-<deployment>}"
  local compound
  compound="$(zoa_compound_session_id)"

  if [[ -z "${compound}" ]]; then
    return 0
  fi

  printf '%s\n' '' '--- ZOA boundary ---' \
    'ECS Exec ended; the ZOA boundary task is still running.' \
    'Possible next steps on your laptop:' \
    "  Stop session:    zoa session stop ${compound}" \
    "  Re-join session: zoa session join ${compound}" \
    "  Your sessions:            zoa session list ${deployment}" \
    "  All operators sessions:   zoa session history ${deployment}" \
    '---'
}

if [[ $- == *i* ]] && [[ -n "${ZOA_SESSION_ID:-}" ]]; then
  trap zoa_boundary_print_session_stop_hint EXIT
fi
