# Break-glass: parallel kube + aws active grants (see docs/break-glass/README.md).

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
  local now end diff
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
  if (( diff < 3600 )); then
    printf '~%sm' $((diff / 60))
    return
  fi
  # Round up so a 4h grant does not show ~3h for most of the window.
  printf '~%sh' $(((diff + 3599) / 3600))
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
  local active creds
  active="$(zoa_breakglass_active_file)"
  unset KUBECONFIG
  if [[ -z "${AWS_SHARED_CREDENTIALS_FILE:-}" || "${AWS_SHARED_CREDENTIALS_FILE}" == "${HOME}/.config/zoa/breakglass/aws-credentials" ]]; then
    unset AWS_PROFILE AWS_DEFAULT_PROFILE
  fi
  if [[ ! -f "${active}" ]] || ! command -v jq >/dev/null 2>&1; then
    return
  fi
  zoa_breakglass_expire_stanzas

  local kube_rid aws_rid kcfg
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
      export AWS_PROFILE="zoa-bg-${aws_rid}"
    fi
  fi
}

zoa_breakglass_ps1_suffix() {
  local active parts plane scope exp left
  zoa_breakglass_apply_env
  active="$(zoa_breakglass_active_file)"
  if [[ ! -f "${active}" ]] || ! command -v jq >/dev/null 2>&1; then
    return
  fi
  parts=()
  for plane in kube aws; do
    scope="$(jq -r --arg p "${plane}" '.[$p].scope // empty' "${active}")"
    if [[ -z "${scope}" ]]; then
      continue
    fi
    exp="$(jq -r --arg p "${plane}" '.[$p].grantExpiresAt // empty' "${active}")"
    left="$(zoa_breakglass_time_left "${exp}")"
    if [[ -n "${left}" ]]; then
      parts+=("${plane}=${scope} grant${left} cred≤1h")
    else
      parts+=("${plane}=${scope} cred≤1h")
    fi
  done
  if ((${#parts[@]} == 0)); then
    return
  fi
  local joined
  joined="$(IFS=' '; printf '%s' "${parts[*]}")"
  printf ' [BG: %s]' "${joined}"
}

# Wire KUBECONFIG / AWS_PROFILE before every prompt (and after zoa breakglass activate).
zoa_breakglass_precmd() {
  zoa_breakglass_apply_env
}

if [[ -n "${ZOA_SESSION_ID:-}" || -n "${ZOA_API_URL:-}" ]]; then
  case "${PROMPT_COMMAND:-}" in
    *zoa_breakglass_precmd*) ;;
    *) PROMPT_COMMAND="${PROMPT_COMMAND:+$PROMPT_COMMAND; }zoa_breakglass_precmd" ;;
  esac
  zoa() {
    local rc=0
    command zoa "$@" || rc=$?
    if (( rc == 0 )) && [[ "${1:-}" == "breakglass" && "${2:-}" == "activate" ]]; then
      zoa_breakglass_apply_env
    fi
    return "${rc}"
  }
fi
