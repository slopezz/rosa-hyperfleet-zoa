#!/usr/bin/env bats

setup() {
  REPO_ROOT="$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)"
  BANNER="${REPO_ROOT}/boundary/zoa-boundary-banner.sh"
  SESSION_ID_RC="${REPO_ROOT}/boundary/home-sre/.bashrc.d/09-zoa-session-id.bashrc"
}

@test "session limits use aligned hard and inactivity termination copy" {
  run env \
    ZOA_SESSION_DEADLINE="2099-01-01T12:00:00Z" \
    ZOA_SESSION_IDLE_TIMEOUT_SECONDS="3600" \
    ZOA_DEPLOYMENT="us-east-1-eph" \
    ZOA_SESSION_ID="sess-abc123" \
    bash -c '
      source "$1"
      source "$2"
      zoa_boundary_print_session_limits
    ' bash "${BANNER}" "${SESSION_ID_RC}"

  [ "${status}" -eq 0 ]
  [[ "${output}" == *"Max wall-clock from session start."* ]]
  [[ "${output}" == *"Task ends even if before the hard termination deadline."* ]]
  [[ "${output}" == *"terminate from laptop:"* ]]
  [[ "${output}" != *"hard limit"* ]]
}

@test "quick start omits --reason when ZOA_REASON is set" {
  run env ZOA_REASON="ROSAENG-1234" bash -c '
    source "$1"
    zoa_boundary_print_quick_start
  ' bash "${BANNER}"

  [ "${status}" -eq 0 ]
  [[ "${output}" == *"zoa run <action> ..."* ]]
  [[ "${output}" != *"--reason"* ]]
}

@test "quick start requires --reason when ZOA_REASON unset" {
  run bash -c '
    unset ZOA_REASON
    source "$1"
    zoa_boundary_print_quick_start
  ' bash "${BANNER}"

  [ "${status}" -eq 0 ]
  [[ "${output}" == *"--reason ROSAENG-1234"* ]]
}
