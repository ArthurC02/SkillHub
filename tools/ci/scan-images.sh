#!/usr/bin/env bash
set -euo pipefail

: "${SYFT:?set SYFT to a digest-pinned anchore/syft ref}"
: "${GRYPE:?set GRYPE to a digest-pinned anchore/grype ref}"

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUT="${SCAN_OUT:-$ROOT/scan}"
FAIL_ON_UPSTREAM="${FAIL_ON_UPSTREAM:-0}"

mkdir -p "$OUT"
summary="$OUT/summary.md"
: >"$summary"

DB_VOLUME=skillhub-grype-db
docker volume create "$DB_VOLUME" >/dev/null

grype_with_shared_db() {
  docker run --rm -v "$DB_VOLUME:/db" -e GRYPE_DB_CACHE_DIR=/db "$@"
}

upstream_images() {
  sed -n 's/^[[:space:]]*image:[[:space:]]*//p' "$ROOT/infra/compose/docker-compose.yml" |
    tr -d "\"'" | sort -u
}

scan() {
  local name="$1" ref="$2" tier="$3" status=0
  local dir="$OUT/$name"
  mkdir -p "$dir"
  docker run --rm -v /var/run/docker.sock:/var/run/docker.sock -v "$dir:/scan" \
    "$SYFT" "$ref" -o spdx-json=/scan/sbom.spdx.json
  grype_with_shared_db -v "$dir:/scan" "$GRYPE" sbom:/scan/sbom.spdx.json \
    -o table --file /scan/vulnerabilities.txt
  grype_with_shared_db -v "$dir:/scan" "$GRYPE" sbom:/scan/sbom.spdx.json \
    -o json --file /scan/vulnerabilities.json
  grype_with_shared_db -v "$dir:/scan" "$GRYPE" sbom:/scan/sbom.spdx.json \
    --only-fixed --fail-on high -o table --file /scan/fixable-high.txt || status=$?
  if [ ! -s "$dir/fixable-high.txt" ]; then
    echo "$name: grype produced no report; treating as a scan failure" >&2
    exit 1
  fi

  local verdict="clean"
  if [ "$status" -ne 0 ]; then
    if [ "$tier" = "deployed" ] || { [ "$tier" = "upstream" ] && [ "$FAIL_ON_UPSTREAM" = "1" ]; }; then
      verdict="FAIL"
      FAILED="${FAILED} $name"
    else
      verdict="fixable High, reported"
    fi
  fi
  printf '| `%s` | %s | %s |\n' "$name" "$tier" "$verdict" >>"$summary"
  printf '%s: %s\n' "$name" "$verdict" >&2
}

FAILED=""
printf '| image | tier | fixable Critical/High |\n| --- | --- | --- |\n' >>"$summary"

while read -r ref; do
  [ -n "$ref" ] || continue
  docker pull -q "$ref" >/dev/null
  scan "$(printf '%s' "$ref" | sed 's#@.*##; s#:.*##; s#.*/##')" "$ref" upstream
done <<EOF
$(upstream_images)
EOF

build_and_scan() {
  local name="$1" context="$2" dockerfile="$3"
  docker build -q -t "skillhub-scan/$name:local" -f "$ROOT/$dockerfile" "$ROOT/$context" >/dev/null
  scan "$name" "skillhub-scan/$name:local" "$4"
}

build_and_scan platform . infra/images/platform/Dockerfile deployed
build_and_scan web . infra/images/web/Dockerfile deployed
build_and_scan llm apps/llm infra/images/llm/Dockerfile deployed
build_and_scan devtools infra/images/devtools infra/images/devtools/Dockerfile dev-only

if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
  {
    echo '### Image scan'
    cat "$summary"
    echo
    echo 'Deployed images block on a fixable Critical/High. Upstream images block only on the scheduled run. The dev container never blocks.'
  } >>"$GITHUB_STEP_SUMMARY"
fi

if [ -n "$FAILED" ]; then
  echo "fixable Critical/High in:$FAILED" >&2
  exit 1
fi
