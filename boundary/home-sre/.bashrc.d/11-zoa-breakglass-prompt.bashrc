# Break-glass: parallel kube + aws active grants (see docs/break-glass/README.md).
# AWS: AWS_SHARED_CREDENTIALS_FILE with [default] STS keys when aws grant active (plain `aws` CLI).
# Zoa CLI uses ECS task role only (see internal/cli/awsboundary.go).

zoa_breakglass_active_file() {
  printf '%s' "${HOME}/.config/zoa/breakglass/active"
}

zoa_breakglass_now_epoch() {
  date --utc +%s
}

zoa_breakglass_expire_to_epoch() {
  local exp="$1"
  date --utc --date="${exp}" +%s 2>/dev/null || printf ''
}

zoa_breakglass_time_left() {
  local exp="$1"
  local now end diff h m
  end="$(zoa_breakglass_expire_to_epoch "${exp}")"
  if [[ -z "${end}" ]]; then
    printf ''
    return
  fi
  now="$(zoa_breakglass_now_epoch)"
  diff=$((end - now))
  if (( diff < 0 )); then
    printf 'expired'
    return
  fi
  if (( diff < 60 )); then
    printf '~%ss' "${diff}"
    return
  fi
  if (( diff < 3600 )); then
    printf '~%sm' $((diff / 60))
    return
  fi
  h=$((diff / 3600))
  m=$(((diff % 3600) / 60))
  if (( m > 0 )); then
    printf '~%sh%sm' "${h}" "${m}"
    return
  fi
  printf '~%sh' "${h}"
}

zoa_breakglass_expire_stanzas() {
  local active changed now
  active="$(zoa_breakglass_active_file)"
  if [[ ! -f "${active}" ]] || ! command -v jq >/dev/null 2>&1; then
    return
  fi
  now="$(zoa_breakglass_now_epoch)"
  changed=0
  for plane in kube aws; do
    local exp epoch
    exp="$(jq -r --arg p "${plane}" '.[$p].grantExpiresAt // empty' "${active}")"
    if [[ -z "${exp}" ]]; then
      continue
    fi
    epoch="$(zoa_breakglass_expire_to_epoch "${exp}")"
    if [[ -n "${epoch}" && "${epoch}" -le "${now}" ]]; then
      jq --arg p "${plane}" 'del(.[$p])' "${active}" > "${active}.tmp" \
        && mv "${active}.tmp" "${active}"
      changed=1
    fi
  done
  if (( changed )); then
    :
  fi
}

zoa_breakglass_apply_env() {
  local active creds kube_rid aws_rid kcfg
  active="$(zoa_breakglass_active_file)"
  unset KUBECONFIG AWS_PROFILE AWS_DEFAULT_PROFILE AWS_SHARED_CREDENTIALS_FILE

  if [[ ! -f "${active}" ]] || ! command -v jq >/dev/null 2>&1; then
    return
  fi
  zoa_breakglass_expire_stanzas

  kube_rid="$(jq -r '.kube.requestId // empty' "${active}")"
  if [[ -n "${kube_rid}" ]]; then
    kcfg="${HOME}/.config/zoa/breakglass/${kube_rid}/kubeconfig"
    if [[ -f "${kcfg}" ]]; then
      export KUBECONFIG="${kcfg}"
    fi
  fi

  aws_rid="$(jq -r '.aws.requestId // empty' "${active}")"
  if [[ -n "${aws_rid}" ]]; then
    creds="${HOME}/.config/zoa/breakglass/aws-credentials"
    if [[ -f "${creds}" ]]; then
      export AWS_SHARED_CREDENTIALS_FILE="${creds}"
    fi
  fi
}

zoa_breakglass_ps1_part() {
  local plane="$1"
  local active="$2"
  local scope exp grant_left bearer_exp cred_left part
  scope="$(jq -r --arg p "${plane}" '.[$p].scope // empty' "${active}")"
  if [[ -z "${scope}" ]]; then
    return 0
  fi
  exp="$(jq -r --arg p "${plane}" '.[$p].grantExpiresAt // empty' "${active}")"
  grant_left="$(zoa_breakglass_time_left "${exp}")"
  bearer_exp="$(jq -r --arg p "${plane}" '.[$p].bearerExpiresAt // empty' "${active}")"
  cred_left="$(zoa_breakglass_time_left "${bearer_exp}")"
  part="${plane}=${scope}"
  if [[ -n "${grant_left}" ]]; then
    part="${part} grant${grant_left}"
  fi
  if [[ -z "${cred_left}" ]]; then
    part="${part} cred≤1h"
  else
    part="${part} cred${cred_left}"
  fi
  printf '%s' "${part}"
}

zoa_breakglass_ps1_suffix() {
  local active kube_part aws_part
  zoa_breakglass_apply_env
  active="$(zoa_breakglass_active_file)"
  if [[ ! -f "${active}" ]] || ! command -v jq >/dev/null 2>&1; then
    return
  fi
  kube_part="$(zoa_breakglass_ps1_part kube "${active}")"
  aws_part="$(zoa_breakglass_ps1_part aws "${active}")"
  if [[ -z "${kube_part}" && -z "${aws_part}" ]]; then
    return
  fi
  if [[ -n "${kube_part}" && -n "${aws_part}" ]]; then
    printf ' [BG: %s | %s]' "${kube_part}" "${aws_part}"
    return
  fi
  if [[ -n "${kube_part}" ]]; then
    printf ' [BG: %s]' "${kube_part}"
    return
  fi
  printf ' [BG: %s]' "${aws_part}"
}

zoa_breakglass_precmd() {
  zoa_breakglass_apply_env
}

if [[ -n "${ZOA_SESSION_ID:-}" || -n "${ZOA_API_URL:-}" ]]; then
  export AWS_PAGER=""

  case "${PROMPT_COMMAND:-}" in
    *zoa_breakglass_precmd*) ;;
    *) PROMPT_COMMAND="${PROMPT_COMMAND:+$PROMPT_COMMAND; }zoa_breakglass_precmd" ;;
  esac

  zoa() {
    local rc=0
    command zoa "$@" || rc=$?
    zoa_breakglass_apply_env
    return "${rc}"
  }
fi
