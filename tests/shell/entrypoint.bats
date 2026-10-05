#!/usr/bin/env bats

setup() {
  REPO_ROOT="$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)"
  ENTRYPOINT="${REPO_ROOT}/boundary/zoa-boundary-entrypoint.sh"
}

@test "entrypoint is safely sourceable" {
  run bash -c 'source "$1"; printf "%s\n" sourced' bash "${ENTRYPOINT}"

  [ "${status}" -eq 0 ]
  [ "${output}" = "sourced" ]
}

@test "write_zoa_session_md includes session id and operator" {
  run env \
    ZOA_SESSION_ID="us-east-1-eph/d77d58c8-1111-2222" \
    ZOA_OPERATOR="slopezma" \
    ZOA_DEPLOYMENT="us-east-1-eph" \
    ZOA_TARGET="mc01" \
    ZOA_API_URL="https://example.lambda-url.on.aws" \
    AWS_REGION="us-east-1" \
    HOME="/tmp/zoa-bats-home" \
    bash -c '
      rm -rf "${HOME}/.claude"
      source "$1"
      write_zoa_session_md
      cat "$(zoa_session_md_path)"
    ' bash "${ENTRYPOINT}"

  [ "${status}" -eq 0 ]
  [[ "${output}" == *"us-east-1-eph/d77d58c8-1111-2222"* ]]
  [[ "${output}" == *"slopezma"* ]]
}

@test "install_actions_catalog copies baked mc catalog when present" {
  run env \
    HOME="/tmp/zoa-bats-catalog-1" \
    ZOA_DEPLOYMENT_TARGET="mc" \
    bash -c '
      rm -rf "${HOME}/.claude"
      mkdir --parents /usr/share/zoa/catalog
      printf "# test mc catalog\n" > /usr/share/zoa/catalog/ZOA_ACTIONS.mc.md
      source "$1"
      install_actions_catalog
      cat "$(zoa_actions_catalog_path)"
    ' bash "${ENTRYPOINT}"

  [ "${status}" -eq 0 ]
  [[ "${output}" == *"test mc catalog"* ]]
}

@test "install_actions_catalog writes placeholder when baked file missing" {
  run env \
    HOME="/tmp/zoa-bats-catalog-2" \
    ZOA_DEPLOYMENT_TARGET="rc" \
    bash -c '
      rm -rf "${HOME}/.claude"
      rm -f /usr/share/zoa/catalog/ZOA_ACTIONS.rc.md
      source "$1"
      install_actions_catalog
      cat "$(zoa_actions_catalog_path)"
    ' bash "${ENTRYPOINT}"

  [ "${status}" -eq 0 ]
  [[ "${output}" == *"Baked catalog missing"* ]]
}

@test "require_ecs_exec_logging_bins fails when script is missing" {
  run bash -c '
    PATH="/usr/bin:/bin"
    source "$1"
    require_ecs_exec_logging_bins
  ' bash "${ENTRYPOINT}"

  [ "${status}" -ne 0 ]
  [[ "${output}" == *"missing script"* ]]
}

@test "require_ecs_exec_logging_bins succeeds when script and cat exist" {
  run bash -c '
    PATH="/usr/bin:/bin"
    source "$1"
    require_ecs_exec_logging_bins
    printf ok
  ' bash "${ENTRYPOINT}"

  [ "${status}" -eq 0 ]
  [ "${output}" = "ok" ]
}
