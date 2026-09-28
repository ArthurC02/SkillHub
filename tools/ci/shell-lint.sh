#!/usr/bin/env bash
set -euo pipefail

SHELLCHECK=koalaman/shellcheck:v0.11.0@sha256:61862eba1fcf09a484ebcc6feea46f1782532571a34ed51fedf90dd25f925a8d

ROOT="$(cd "$(dirname "$0")/../.." && (pwd -W 2>/dev/null || pwd))"
cd "$ROOT"

mapfile -t scripts < <(git ls-files '*.sh')
if [ "${#scripts[@]}" -eq 0 ]; then
  echo "shell-lint: git lists no *.sh file; the check is looking at the wrong tree" >&2
  exit 1
fi

MSYS_NO_PATHCONV=1 docker run --rm -v "$ROOT:/repo:ro" -w /repo "$SHELLCHECK" "${scripts[@]}"
echo "shell scripts: ${#scripts[@]} checked, ok"
