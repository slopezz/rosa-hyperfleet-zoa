#!/usr/bin/env bats

setup() {
  REPO_ROOT="$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)"
  REMINDER="${REPO_ROOT}/boundary/home-sre/.bashrc.d/99-session-exit-reminder.bashrc"
}

@test "session stop hint includes deployment and session id" {
  run env \
    ZOA_DEPLOYMENT="us-east-1-eph" \
    ZOA_SESSION_ID="sess-abc123" \
    bash -c '
      source "$1"
      zoa_boundary_print_session_stop_hint
    ' bash "${REMINDER}"

  [ "${status}" -eq 0 ]
  [[ "${output}" == *"zoa session stop us-east-1-eph/sess-abc123"* ]]
  [[ "${output}" == *"zoa session list -d us-east-1-eph"* ]]
  [[ "${output}" == *"still running"* ]]
}

@test "session stop hint is silent when ZOA_SESSION_ID unset" {
  run env \
    ZOA_DEPLOYMENT="us-east-1-eph" \
    bash -c '
      source "$1"
      zoa_boundary_print_session_stop_hint
    ' bash "${REMINDER}"

  [ "${status}" -eq 0 ]
  [ "${output}" = "" ]
}

@test "non-interactive shell does not install EXIT trap" {
  run env \
    ZOA_SESSION_ID="sess-abc123" \
    bash -c '
      source "$1"
      trap -p EXIT
    ' bash "${REMINDER}"

  [ "${status}" -eq 0 ]
  [[ "${output}" != *"zoa_boundary_print_session_stop_hint"* ]]
}

@test "EXIT trap prints session stop hint on shell exit" {
  run env \
    ZOA_DEPLOYMENT="us-east-1-eph" \
    ZOA_SESSION_ID="sess-abc123" \
    bash -c '
      source "$1"
      trap zoa_boundary_print_session_stop_hint EXIT
      exit
    ' bash "${REMINDER}"

  [ "${status}" -eq 0 ]
  [[ "${output}" == *"zoa session stop us-east-1-eph/sess-abc123"* ]]
}
