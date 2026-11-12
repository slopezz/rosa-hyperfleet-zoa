# Break-glass active grant indicator (see docs/break-glass/README.md).
zoa_breakglass_ps1_suffix() {
  local active
  active="${HOME}/.config/zoa/breakglass/active"
  if [[ -f "${active}" ]]; then
    printf ' [BG:%s]' "$(tr -d '\n' < "${active}")"
  fi
}
