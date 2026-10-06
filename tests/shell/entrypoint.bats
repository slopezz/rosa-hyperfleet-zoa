#!/usr/bin/env bats

setup() {
  REPO_ROOT="$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)"
  ENTRYPOINT="${REPO_ROOT}/boundary/zoa-boundary-entrypoint.sh"
  ZOA_BIN="${REPO_ROOT}/bin/zoa"
}

@test "entrypoint is safely sourceable" {
  run bash -c 'source "$1"; printf "%s\n" sourced' bash "${ENTRYPOINT}"

  [ "${status}" -eq 0 ]
  [ "${output}" = "sourced" ]
}

@test "zoa_session_md_path and zoa_actions_catalog_path are fixed under home sre claude" {
  run bash -c '
    source "$1"
    zoa_session_md_path
    zoa_actions_catalog_path
  ' bash "${ENTRYPOINT}"

  [ "${status}" -eq 0 ]
  [ "${lines[0]}" = "/home/sre/.claude/ZOA_SESSION.md" ]
  [ "${lines[1]}" = "/home/sre/.claude/ZOA_ACTIONS.md" ]
}

@test "zoa_target_type prefers ZOA_TARGET_TYPE over legacy ZOA_DEPLOYMENT_TARGET" {
  run env ZOA_TARGET_TYPE="rc" ZOA_DEPLOYMENT_TARGET="mc" bash -c '
    source "$1"
    zoa_target_type
  ' bash "${ENTRYPOINT}"

  [ "${status}" -eq 0 ]
  [ "${output}" = "rc" ]
}

@test "zoa_target_type falls back to ZOA_DEPLOYMENT_TARGET when ZOA_TARGET_TYPE unset" {
  run env ZOA_DEPLOYMENT_TARGET="rc" bash -c '
    unset ZOA_TARGET_TYPE
    source "$1"
    zoa_target_type
  ' bash "${ENTRYPOINT}"

  [ "${status}" -eq 0 ]
  [ "${output}" = "rc" ]
}

@test "zoa_target_type defaults to mc when unset" {
  run bash -c '
    unset ZOA_TARGET_TYPE ZOA_DEPLOYMENT_TARGET
    source "$1"
    zoa_target_type
  ' bash "${ENTRYPOINT}"

  [ "${status}" -eq 0 ]
  [ "${output}" = "mc" ]
}

@test "write_zoa_session_md includes session id and target type" {
  run env \
    ZOA_SESSION_ID="us-east-1-eph/d77d58c8-1111-2222" \
    ZOA_OPERATOR="slopezma" \
    ZOA_DEPLOYMENT="us-east-1-eph" \
    ZOA_TARGET="mc01" \
    ZOA_TARGET_TYPE="mc" \
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
  [[ "${output}" == *"Target type | mc"* ]]
  [[ "${output}" == *"ZOA_ACTIONS.md"* ]]
  [[ "${output}" == *"offline render"* ]]
}

@test "write_zoa_session_md uses legacy ZOA_DEPLOYMENT_TARGET for target type row" {
  run env \
    ZOA_SESSION_ID="sess-1" \
    ZOA_DEPLOYMENT_TARGET="rc" \
    HOME="/tmp/zoa-bats-home-legacy" \
    bash -c '
      unset ZOA_TARGET_TYPE
      rm -rf "${HOME}/.claude"
      source "$1"
      write_zoa_session_md
      grep "Target type" "$(zoa_session_md_path)"
    ' bash "${ENTRYPOINT}"

  [ "${status}" -eq 0 ]
  [[ "${output}" == *"Target type | rc"* ]]
}

@test "install_actions_catalog renders markdown via zoa actions --offline" {
  if [[ ! -x "${ZOA_BIN}" ]]; then
    skip "build zoa first: make build"
  fi

  run env \
    HOME="/tmp/zoa-bats-catalog-render" \
    ZOA_TARGET_TYPE="rc" \
    PATH="$(dirname "${ZOA_BIN}"):${PATH}" \
    bash -c '
      rm -rf "${HOME}/.claude"
      source "$1"
      install_actions_catalog
      head -n 3 "$(zoa_actions_catalog_path)"
    ' bash "${ENTRYPOINT}"

  [ "${status}" -eq 0 ]
  [[ "${output}" == *"ZOA Trusted Actions catalog"* ]]
  [[ "${output}" == *"Target type"* ]]
}

@test "install_actions_catalog exits when zoa CLI is missing" {
  run env \
    HOME="/tmp/zoa-bats-no-zoa" \
    PATH="/usr/bin:/bin" \
    bash -c '
      rm -rf "${HOME}/.claude"
      source "$1"
      install_actions_catalog
    ' bash "${ENTRYPOINT}"

  [ "${status}" -ne 0 ]
  [[ "${output}" == *"zoa CLI not found"* ]]
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

@test "print_startup_banner includes session fields and catalog paths" {
  run env \
    ZOA_SESSION_ID="sess-banner" \
    ZOA_OPERATOR="operator1" \
    ZOA_DEPLOYMENT="dep1" \
    ZOA_TARGET="target1" \
    ZOA_TARGET_TYPE="rc" \
    bash -c '
      source "$1"
      print_startup_banner
    ' bash "${ENTRYPOINT}"

  [ "${status}" -eq 0 ]
  [[ "${output}" == *"=== ZOA Boundary Session ==="* ]]
  [[ "${output}" == *"sess-banner"* ]]
  [[ "${output}" == *"Target type: rc"* ]]
  [[ "${output}" == *"/home/sre/.claude/ZOA_SESSION.md"* ]]
  [[ "${output}" == *"/home/sre/.claude/ZOA_ACTIONS.md"* ]]
  [[ "${output}" == *"zoa catalog --offline"* ]]
}

@test "main writes session md and catalog then stubs exec" {
  if [[ ! -x "${ZOA_BIN}" ]]; then
    skip "build zoa first: make build"
  fi

  run env \
    HOME="/tmp/zoa-bats-main" \
    ZOA_SESSION_ID="main-sess" \
    ZOA_TARGET_TYPE="mc" \
    PATH="$(dirname "${ZOA_BIN}"):/usr/bin:/bin" \
    bash -c '
      rm -rf "${HOME}/.claude"
      source "$1"
      exec() { printf "exec-stub:%s\n" "$*"; }
      main :
      test -f "$(zoa_session_md_path)"
      test -f "$(zoa_actions_catalog_path)"
      printf "files-ok"
    ' bash "${ENTRYPOINT}"

  [ "${status}" -eq 0 ]
  [[ "${output}" == *"exec-stub:"* ]]
  [[ "${output}" == *"files-ok"* ]]
}
