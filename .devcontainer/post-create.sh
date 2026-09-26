#!/usr/bin/env bash
set -euo pipefail

cd "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

skip_bootstrap="${SKILLHUB_SKIP_BOOTSTRAP:-0}"
required_commands=(go npm docker sha256sum)
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

required_env_keys=(
  DATABASE_URL
  OBJSTORE_ENDPOINT
  OBJSTORE_ACCESS_KEY
  OBJSTORE_SECRET_KEY
  LITELLM_BASE_URL
)
for key in "${required_env_keys[@]}"; do
  if ! grep -q "^${key}=" .env; then
    printf "missing required .env key: %s\n" "${key}" >&2
    exit 1
  fi
done

bootstrap_inputs=(
  apps/platform/go.sum
  apps/sandbox/go.sum
  apps/web/package-lock.json
  packages/api-client-ts/package-lock.json
  apps/llm/uv.lock
)

if [ "${skip_bootstrap}" = "1" ]; then
  echo "SKILLHUB_SKIP_BOOTSTRAP=1 set; skipping bootstrap"
  exit 0
fi

bootstrap_hash="$(sha256sum "${bootstrap_inputs[@]}" | sha256sum | awk '{print $1}')"
bootstrap_stamp=.devcontainer/.bootstrap.stamp

if [ -f "${bootstrap_stamp}" ] && [ "$(cat "${bootstrap_stamp}")" = "${bootstrap_hash}" ]; then
  echo "bootstrap already up to date; skipping"
  exit 0
fi

go -C tools/devctl run . bootstrap
stamp_tmp="${bootstrap_stamp}.tmp"
trap 'rm -f "${stamp_tmp}"' EXIT INT TERM
printf "%s" "${bootstrap_hash}" >"${stamp_tmp}"
mv "${stamp_tmp}" "${bootstrap_stamp}"
trap - EXIT INT TERM
