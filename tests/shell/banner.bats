#!/usr/bin/env bats

setup() {
  REPO_ROOT="$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)"
  BANNER="${REPO_ROOT}/boundary/zoa-boundary-banner.sh"
}

@test "banner is safely sourceable" {
  run bash -c 'source "$1"; printf sourced' bash "${BANNER}"

  [ "${status}" -eq 0 ]
  [ "${output}" = "sourced" ]
}

@test "zoa_boundary_ascii_art prints ZOA banner without ANSI escape codes" {
  run bash -c 'source "$1"; zoa_boundary_ascii_art' bash "${BANNER}"

  [ "${status}" -eq 0 ]
  [[ "${output}" == *"███████"* ]]
  [[ "${output}" != *$'\033'* ]]
}

@test "zoa_boundary_print_motd includes ascii art and Hello operator" {
  run env \
    ZOA_SESSION_ID="d77d58c8-1111-2222" \
    ZOA_OPERATOR="slopezma" \
    ZOA_DEPLOYMENT="us-east-1-eph" \
    ZOA_TARGET="mc01" \
    ZOA_TARGET_TYPE="mc" \
    AWS_REGION="us-east-1" \
    bash -c '
      zoa_compound_session_id() { printf "us-east-1-eph/d77d58c8-1111-2222"; }
      source "$1"
      zoa_boundary_print_motd
    ' bash "${BANNER}"

  [ "${status}" -eq 0 ]
  [[ "${output}" == *"███████"* ]]
  [[ "${output}" == *"Hello, slopezma"* ]]
  [[ "${output}" == *"Operator:    slopezma"* ]]
  [[ "${output}" == *"ROSA HyperFleet architecture"* ]]
  [[ "${output}" != *"not env vars alone"* ]]
  [[ "${output}" == *"==> ZOA Boundary session"* ]]
  [[ "${output}" == *"us-east-1-eph/d77d58c8-1111-2222"* ]]
  [[ "${output}" == *"zoa session stop us-east-1-eph/d77d58c8-1111-2222"* ]]
  [[ "${output}" == *"CLAUDE.md"* ]]
  [[ "${output}" == *"ZOA_SESSION.md"* ]]
}

@test "zoa_boundary_print_exit_hint uses aligned stop and join lines" {
  run env \
    ZOA_DEPLOYMENT="dep-a" \
    bash -c '
      zoa_compound_session_id() { printf "dep-a/sess-1"; }
      source "$1"
      zoa_boundary_print_exit_hint
    ' bash "${BANNER}"

  [ "${status}" -eq 0 ]
  [[ "${output}" == *"==> ZOA boundary — Exec ended"* ]]
  [[ "${output}" == *"zoa session stop dep-a/sess-1"* ]]
  [[ "${output}" == *"zoa session join dep-a/sess-1"* ]]
}

@test "zoa_boundary_print_container_startup_banner includes ascii and session fields" {
  run env \
    ZOA_SESSION_ID="sess-banner" \
    ZOA_OPERATOR="operator1" \
    bash -c '
      source "$1"
      zoa_boundary_print_container_startup_banner
    ' bash "${BANNER}"

  [ "${status}" -eq 0 ]
  [[ "${output}" == *"███████"* ]]
  [[ "${output}" == *"container startup"* ]]
  [[ "${output}" == *"sess-banner"* ]]
}
