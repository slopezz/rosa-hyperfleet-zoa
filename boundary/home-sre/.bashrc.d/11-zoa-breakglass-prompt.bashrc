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
  printf '~%sh' $((diff / 3600))
}

zoa_breakglass_migrate_legacy_active() {
  local active rid meta scope exp plane
  active="$(zoa_breakglass_active_file)"
  if [[ ! -f "${active}" ]] || command -v jq >/dev/null 2>&1 && jq -e . "${active}" >/dev/null 2>&1; then
    return
  fi
  rid="$(tr -d '[:space:]' < "${active}")"
  meta="${HOME}/.config/zoa/breakglass/${rid}/meta.json"
  if [[ ! -f "${meta}" ]]; then
    return
  fi
  scope="$(jq -r '.scope // empty' "${meta}")"
  exp="$(jq -r '.grant_expires_at // empty' "${meta}")"
  if [[ -z "${scope}" ]]; then
    return
  fi
  if [[ "${scope}" == aws-* ]]; then
    plane="aws"
  else
    plane="kube"
  fi
  jq -n \
    --arg rid "${rid}" \
    --arg scope "${scope}" \
    --arg exp "${exp}" \
    --arg plane "${plane}" \
    'if $plane == "aws" then {aws: {requestId: $rid, scope: $scope, grantExpiresAt: $exp}}
     else {kube: {requestId: $rid, scope: $scope, grantExpiresAt: $exp}} end' > "${active}.tmp" \
    && mv "${active}.tmp" "${active}"
}

zoa_breakglass_expire_stanzas() {
  local active changed now
  active="$(zoa_breakglass_active_file)"
  if [[ ! -f "${active}" ]] || ! command -v jq >/dev/null 2>&1; then
    return
  fi
  zoa_breakglass_migrate_legacy_active
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
      parts+=("${plane}=${scope} (${left})")
    else
      parts+=("${plane}=${scope}")
    fi
  done
  if ((${#parts[@]} == 0)); then
    return
  fi
  local joined
  joined="$(IFS=' '; printf '%s' "${parts[*]}")"
  printf ' [BG: %s]' "${joined}"
}
