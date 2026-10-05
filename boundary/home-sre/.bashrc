# ZOA Boundary — non-login shell defaults (ECS Exec).
export PATH="/usr/local/bin:/usr/local/aws-cli/v2/current/bin:${PATH:-/usr/bin:/bin}"

if [[ -d "${HOME}/.bashrc.d" ]]; then
  for rc in "${HOME}"/.bashrc.d/*.bashrc; do
    # shellcheck source=/dev/null
    [[ -f "${rc}" ]] && source "${rc}"
  done
fi

if [[ -f /etc/bash_completion/bash_completion ]]; then
  # shellcheck source=/dev/null
  source /etc/bash_completion/bash_completion
fi

if [[ -d /etc/bash_completion.d ]]; then
  for completion in /etc/bash_completion.d/*; do
    # shellcheck source=/dev/null
    [[ -f "${completion}" ]] && source "${completion}"
  done
fi
