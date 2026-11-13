#!/usr/bin/env bats

load test-env

setup() {
  REPO_ROOT="$(zoa_shell_test_repo_root)"
  ENTRYPOINT="${REPO_ROOT}/boundary/zoa-boundary-entrypoint.sh"
  ZOA_BIN="${REPO_ROOT}/bin/zoa"
  BANNER="$(zoa_shell_test_banner_path)"
  STUB_BIN="$(zoa_shell_test_stub_bin_dir)"
  zoa_shell_test_export_banner
}

@test "entrypoint is safely sourceable" {
  run bash -c 'source "$1"; printf "%s\n" sourced' bash "${ENTRYPOINT}"

  [ "${status}" -eq 0 ]
  [ "${output}" = "sourced" ]
}

@test "zoa_session_md_path and zoa_actions_catalog_path are fixed under home sre claude" {
  run env HOME=/home/sre bash -c '
    source "$1"
    zoa_session_md_path
    zoa_actions_catalog_path
  ' bash "${ENTRYPOINT}"

  [ "${status}" -eq 0 ]
  [ "${lines[0]}" = "/home/sre/.claude/ZOA_SESSION.md" ]
  [ "${lines[1]}" = "/home/sre/.claude/ZOA_ACTIONS.md" ]
}

@test "zoa_target_type uses ZOA_TARGET_TYPE when set" {
  run env ZOA_TARGET_TYPE="rc" bash -c '
    source "$1"
    zoa_target_type
  ' bash "${ENTRYPOINT}"

  [ "${status}" -eq 0 ]
  [ "${output}" = "rc" ]
}

@test "zoa_target_type defaults to mc when unset" {
  run bash -c '
    unset ZOA_TARGET_TYPE
    source "$1"
    zoa_target_type
  ' bash "${ENTRYPOINT}"

  [ "${status}" -eq 0 ]
  [ "${output}" = "mc" ]
}

@test "write_zoa_session_md includes session id and target type" {
  run env \
    ZOA_BOUNDARY_BANNER_PATH="${BANNER}" \
    ZOA_BOUNDARY_BANNER="${BANNER}" \
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

@test "write_zoa_session_md includes reason and session deadline when set" {
  run env \
    ZOA_BOUNDARY_BANNER_PATH="${BANNER}" \
    ZOA_BOUNDARY_BANNER="${BANNER}" \
    ZOA_SESSION_ID="sess-raw-uuid" \
    ZOA_OPERATOR="operator1" \
    ZOA_DEPLOYMENT="us-east-1-eph" \
    ZOA_TARGET="mc01" \
    ZOA_TARGET_TYPE="mc" \
    ZOA_REASON="ROSAENG-9999" \
    ZOA_SESSION_DEADLINE="2030-06-01T12:00:00Z" \
    ZOA_SESSION_IDLE_TIMEOUT_SECONDS="7200" \
    ZOA_API_URL="https://example.lambda-url.on.aws" \
    AWS_REGION="us-east-1" \
    HOME="/tmp/zoa-bats-home-reason" \
    bash -c '
      rm -rf "${HOME}/.claude"
      source "$1"
      write_zoa_session_md
      cat "$(zoa_session_md_path)"
    ' bash "${ENTRYPOINT}"

  [ "${status}" -eq 0 ]
  [[ "${output}" == *"| Reason | ROSAENG-9999 |"* ]]
  [[ "${output}" == *"| Session deadline (UTC) | 2030-06-01T12:00:00Z |"* ]]
  [[ "${output}" == *"Inactivity termination"* ]]
}

@test "install_breakglass_scopes_catalog renders markdown via zoa breakglass scopes --offline" {
  if [[ ! -x "${ZOA_BIN}" ]]; then
    skip "build zoa first: make build"
  fi

  run env \
    ZOA_BOUNDARY_BANNER_PATH="${BANNER}" \
    ZOA_BOUNDARY_BANNER="${BANNER}" \
    HOME="/tmp/zoa-bats-bg-catalog" \
    ZOA_TARGET_TYPE="rc" \
    PATH="$(dirname "${ZOA_BIN}"):${PATH}" \
    bash -c '
      rm -rf "${HOME}/.claude"
      source "$1"
      install_breakglass_scopes_catalog
      head -n 3 "$(zoa_breakglass_scopes_catalog_path)"
    ' bash "${ENTRYPOINT}"

  [ "${status}" -eq 0 ]
  [[ "${output}" == *"break-glass scope catalog"* ]]
  [[ "${output}" == *"Target type"* ]]
}

@test "install_actions_catalog renders markdown via zoa actions --offline" {
  if [[ ! -x "${ZOA_BIN}" ]]; then
    skip "build zoa first: make build"
  fi

  run env \
    ZOA_BOUNDARY_BANNER_PATH="${BANNER}" \
    ZOA_BOUNDARY_BANNER="${BANNER}" \
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
    ZOA_BOUNDARY_BANNER_PATH="${BANNER}" \
    ZOA_BOUNDARY_BANNER="${BANNER}" \
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
    source "$1"
    PATH="$2:/usr/bin:/bin"
    require_ecs_exec_logging_bins
    printf ok
  ' bash "${ENTRYPOINT}" "${STUB_BIN}"

  [ "${status}" -eq 0 ]
  [ "${output}" = "ok" ]
}

@test "print_startup_banner includes session fields and catalog paths" {
  run env \
    ZOA_BOUNDARY_BANNER_PATH="${BANNER}" \
    ZOA_BOUNDARY_BANNER="${BANNER}" \
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
  [[ "${output}" == *"=== ZOA Boundary Session (container startup) ==="* ]]
  [[ "${output}" == *"sess-banner"* ]]
  [[ "${output}" == *"Target type: rc"* ]]
  [[ "${output}" == *"ZOA_SESSION.md"* ]]
  [[ "${output}" == *"ZOA_ACTIONS.md"* ]]
  [[ "${output}" == *"/home/sre/.claude/"* ]]
  [[ "${output}" == *"zoa actions"* ]] || [[ "${output}" == *"zoa catalog --offline"* ]]
}

@test "main writes session md and catalog then stubs exec" {
  if [[ ! -x "${ZOA_BIN}" ]]; then
    skip "build zoa first: make build"
  fi

  run env \
    ZOA_BOUNDARY_BANNER_PATH="${BANNER}" \
    ZOA_BOUNDARY_BANNER="${BANNER}" \
    HOME="/tmp/zoa-bats-main" \
    ZOA_SESSION_ID="main-sess" \
    ZOA_TARGET_TYPE="mc" \
    PATH="$(dirname "${ZOA_BIN}"):${STUB_BIN}:/usr/bin:/bin" \
    bash -c '
      rm -rf "${HOME}/.claude"
      source "$1"
      PATH="$(dirname "$2"):$3:/usr/bin:/bin"
      exec() { printf "exec-stub:%s\n" "$*"; }
      main :
      test -f "$(zoa_session_md_path)"
      test -f "$(zoa_actions_catalog_path)"
      printf "files-ok"
    ' bash "${ENTRYPOINT}" "${ZOA_BIN}" "${STUB_BIN}"

  [ "${status}" -eq 0 ]
  [[ "${output}" == *"exec-stub:"* ]]
  [[ "${output}" == *"files-ok"* ]]
}
