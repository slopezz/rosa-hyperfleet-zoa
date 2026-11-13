#!/usr/bin/env bats

setup() {
  REPO_ROOT="$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)"
  BG_RC="${REPO_ROOT}/boundary/home-sre/.bashrc.d/11-zoa-breakglass-prompt.bashrc"
  TEST_HOME="$(mktemp --directory)"
  mkdir --parents "${TEST_HOME}/.config/zoa/breakglass"
}

teardown() {
  rm --recursive --force "${TEST_HOME}"
}

@test "ps1 suffix shows kube and aws scopes in parallel" {
  cat > "${TEST_HOME}/.config/zoa/breakglass/active" <<'EOF'
{
  "kube": {
    "requestId": "11111111-1111-1111-1111-111111111111",
    "scope": "kube-read",
    "grantExpiresAt": "2099-06-01T12:00:00Z"
  },
  "aws": {
    "requestId": "22222222-2222-2222-2222-222222222222",
    "scope": "aws-write",
    "grantExpiresAt": "2099-06-01T14:00:00Z"
  }
}
EOF
  run env HOME="${TEST_HOME}" bash -c '
    source "$1"
    zoa_breakglass_ps1_suffix
  ' bash "${BG_RC}"

  [ "${status}" -eq 0 ]
  [[ "${output}" == *"kube=kube-read"* ]]
  [[ "${output}" == *"aws=aws-write"* ]]
  [[ "${output}" == *"|"* ]]
  [[ "${output}" != *"cred~~"* ]]
}

@test "ps1 part does not double tilde on cred countdown" {
  cat > "${TEST_HOME}/.config/zoa/breakglass/active" <<'EOF'
{
  "kube": {
    "requestId": "11111111-1111-1111-1111-111111111111",
    "scope": "kube-read",
    "grantExpiresAt": "2099-06-01T12:00:00Z",
    "bearerExpiresAt": "2099-06-01T11:58:00Z"
  }
}
EOF
  run env HOME="${TEST_HOME}" bash -c '
    source "$1"
    zoa_breakglass_ps1_part kube "${HOME}/.config/zoa/breakglass/active"
  ' bash "${BG_RC}"

  [ "${status}" -eq 0 ]
  [[ "${output}" == *"cred~"* ]]
  [[ "${output}" != *"cred~~"* ]]
}

@test "apply_env uses AWS_SHARED_CREDENTIALS_FILE not AWS_PROFILE" {
  cat > "${TEST_HOME}/.config/zoa/breakglass/active" <<'EOF'
{
  "aws": {
    "requestId": "22222222-2222-2222-2222-222222222222",
    "scope": "aws-read",
    "grantExpiresAt": "2099-06-01T14:00:00Z",
    "bearerExpiresAt": "2099-06-01T13:00:00Z"
  }
}
EOF
  touch "${TEST_HOME}/.config/zoa/breakglass/aws-credentials"

  run env HOME="${TEST_HOME}" bash -c '
    source "$1"
    zoa_breakglass_apply_env
    test -n "${AWS_SHARED_CREDENTIALS_FILE}"
    test -z "${AWS_PROFILE:-}"
  ' bash "${BG_RC}"

  [ "${status}" -eq 0 ]
}
