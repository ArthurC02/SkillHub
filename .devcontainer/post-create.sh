#!/usr/bin/env bash
set -euo pipefail

cd "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

skip_bootstrap="${SKILLHUB_SKIP_BOOTSTRAP:-0}"
required_commands=(go npm docker)
if [ "${skip_bootstrap}" != "1" ]; then
  required_commands+=(uv)
fi
missing_commands=()
for cmd in "${required_commands[@]}"; do
  if ! command -v "${cmd}" >/dev/null 2>&1; then
    missing_commands+=("${cmd}")
  fi
done
if [ "${#missing_commands[@]}" -gt 0 ]; then
  printf "missing required commands: %s\n" "${missing_commands[*]}" >&2
  exit 1
fi

if [ -d /go ] && [ -d /home/vscode ]; then
  mkdir -p /go/pkg/mod /home/vscode/.npm /home/vscode/.cache/uv
  if command -v sudo >/dev/null 2>&1; then
    sudo chown -R vscode:vscode /go/pkg/mod /home/vscode/.npm /home/vscode/.cache/uv
  fi
fi

go -C tools/devctl run . env-init

if [ "${skip_bootstrap}" = "1" ]; then
  echo "SKILLHUB_SKIP_BOOTSTRAP=1 set; skipping bootstrap and runtime env validation"
  exit 0
fi

required_env_keys=(
  DATABASE_URL
  OBJSTORE_ENDPOINT
  OBJSTORE_ACCESS_KEY
  OBJSTORE_SECRET_KEY
  LITELLM_BASE_URL
)
for key in "${required_env_keys[@]}"; do
  value="$(
    awk -F= -v want="${key}" '
      {
        line=$0
        sub(/^[[:space:]]+/, "", line)
        if (line == "" || substr(line, 1, 1) == "#") {
          next
        }
        split(line, pair, "=")
        parsed_key=pair[1]
        sub(/[[:space:]]+$/, "", parsed_key)
        if (parsed_key == want) {
          sub(/^[^=]*=/, "", line)
          print line
          exit
        }
      }
    ' .env
  )"
  if [ -z "${value}" ]; then
    printf "missing required .env key: %s\n" "${key}" >&2
    exit 1
  fi
  value="$(printf "%s" "${value}" | sed -E 's/^[[:space:]]+|[[:space:]]+$//g')"
  value="${value#\"}"
  value="${value%\"}"
  value="${value#\'}"
  value="${value%\'}"
  if [ -z "${value}" ]; then
    printf "empty required .env value: %s\n" "${key}" >&2
    exit 1
  fi
done

bootstrap_lock=.devcontainer/.bootstrap.lock
lock_wait=0
until mkdir "${bootstrap_lock}" 2>/dev/null; do
  lock_wait=$((lock_wait + 1))
  if [ "${lock_wait}" -ge 120 ]; then
    echo "timed out waiting for bootstrap lock" >&2
    exit 1
  fi
  sleep 1
done

cleanup_lock() {
  rm -rf "${bootstrap_lock}"
}
trap cleanup_lock EXIT INT TERM

go -C tools/devctl run . bootstrap
trap - INT TERM
