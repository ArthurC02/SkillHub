#!/usr/bin/env bash
set -euo pipefail

cd "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

skip_bootstrap="${SKILLHUB_SKIP_BOOTSTRAP:-0}"
required_commands=(go npm docker flock)
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

if command -v sudo >/dev/null 2>&1 && ! pgrep dockerd >/dev/null 2>&1; then
  sudo nohup dockerd --group docker --host=unix:///var/run/docker.sock >/tmp/dockerd.log 2>&1 &
fi
i=0
until docker info >/dev/null 2>&1; do
  i=$((i + 1))
  if [ "${i}" -ge 60 ]; then
    if command -v sudo >/dev/null 2>&1 && [ -f /tmp/dockerd.log ]; then
      sudo tail -80 /tmp/dockerd.log >&2 || true
    fi
    echo "docker daemon not ready" >&2
    exit 1
  fi
  sleep 1
done

if [ -d /go ] && [ -d /home/vscode ]; then
  mkdir -p /go/pkg/mod /home/vscode/.npm /home/vscode/.cache/uv
  if command -v sudo >/dev/null 2>&1; then
    for cache_dir in /go/pkg/mod /home/vscode/.npm /home/vscode/.cache/uv; do
      owner="$(stat -c '%U:%G' "${cache_dir}" 2>/dev/null || true)"
      if [ "${owner}" != "vscode:vscode" ]; then
        sudo chown vscode:vscode "${cache_dir}"
      fi
    done
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

read_env_value() {
  awk -F= -v want="${1}" '
    {
      line=$0
      sub(/^[[:space:]]+/, "", line)
      if (line == "" || substr(line, 1, 1) == "#") {
        next
      }
      if (index(line, "export ") == 1) {
        line = substr(line, 8)
        sub(/^[[:space:]]+/, "", line)
      }
      eq = index(line, "=")
      if (eq == 0) {
        next
      }
      parsed_key = substr(line, 1, eq - 1)
      sub(/[[:space:]]+$/, "", parsed_key)
      if (parsed_key == want) {
        print substr(line, eq + 1)
        exit
      }
    }
  ' .env
}

for key in "${required_env_keys[@]}"; do
  value="$(read_env_value "${key}")"
  value="$(printf "%s" "${value}" | sed -E 's/^[[:space:]]+|[[:space:]]+$//g')"
  value="${value#\"}"
  value="${value%\"}"
  value="${value#\'}"
  value="${value%\'}"
  if printf "%s" "${value}" | grep -Eq '(\$[A-Za-z_][A-Za-z0-9_]*|\$\{[^}]+\})'; then
    printf "unresolved placeholder in .env key: %s\n" "${key}" >&2
    exit 1
  fi
  if [ -z "${value}" ]; then
    printf "missing or empty required .env key: %s\n" "${key}" >&2
    exit 1
  fi
done

bootstrap_lock_key="$(pwd | cksum | awk '{print $1}')"
bootstrap_lock="/tmp/skillhub-devcontainer-bootstrap-${bootstrap_lock_key}.lock"
exec 9>"${bootstrap_lock}"
if ! flock -w 120 9; then
  echo "timed out waiting for bootstrap lock" >&2
  exit 1
fi

go -C tools/devctl run . bootstrap
