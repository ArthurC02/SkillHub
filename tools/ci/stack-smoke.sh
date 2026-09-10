#!/usr/bin/env bash
# Start the service images this build just produced and check that they answer.
#
# Why this exists. `images` built three images, listed them and pushed them to
# GHCR, and no container was ever started from any of them: a broken entrypoint,
# a missing environment default or an nginx.conf that does not parse reached the
# registry green. The 2026-09-09 CSP change is the case in point -- its only
# check reads nginx.conf as text, so nginx was never asked whether the file is
# valid. This runs before the push for that reason.
#
# What it does. Four HTTP assertions against the running stack, and then
# tools/ci/stack-browser.mjs drives a real browser over every route the router
# declares, signed in and signed out (04 丙-221).
#
# The catalogue assertion below runs BEFORE anything is seeded, and that order
# is the assertion: Go's encoding/json writes a nil slice as `null`, a nil slice
# is exactly what an empty result set produces, and `results` is `required` and
# `type: array` in contracts/openapi/public.yaml. That `null` crashed the web
# client on 2026-09-06. An empty catalogue is the state that exposes it, so it
# is checked while the database is still empty; the browser pass seeds itself
# afterwards.
#
# Usage:
#   PLATFORM_IMAGE=... WEB_IMAGE=... LLM_IMAGE=... bash tools/ci/stack-smoke.sh
set -euo pipefail

: "${PLATFORM_IMAGE:?PLATFORM_IMAGE must name the built platform image}"
: "${WEB_IMAGE:?WEB_IMAGE must name the built web image}"
: "${LLM_IMAGE:?LLM_IMAGE must name the built llm image}"

# Same pins as the platform job's service containers: this file must not be the
# place where a second, drifting version of the data layer appears.
PG_IMAGE="docker.io/pgvector/pgvector:pg17@sha256:cf134a767f474095eeba57e0117be8e568e011a63f33fbf252f14c9b760f8e6f"
S3_IMAGE="docker.io/chrislusf/seaweedfs:3.80@sha256:1055999e08eed1789b0ae45d235126e4495e23d3fb9d6396293fd42539b1ae6a"
# Every HTTP check below runs from inside the network rather than through a
# published port. Measured on the machine this was written on: Docker
# Desktop's host-side proxy for `-p 127.0.0.1:...` took over a minute to start
# forwarding, long after the API was answering, so a readiness wait on the
# published port timed out against a healthy service. In-network also exercises
# the DNS name infra/images/web/nginx.conf actually dials.
CURL_IMAGE="docker.io/curlimages/curl@sha256:c1fe1679c34d9784c1b0d1e5f62ac0a79fca01fb6377cdd33e90473c6f9f9a69"
# The version apps/web pins for @playwright/test, so the engine here is the one
# the rest of the browser tier is written against.
PLAYWRIGHT_IMAGE="mcr.microsoft.com/playwright:v1.62.1-noble"

NET="skillhub-smoke"

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
# Git Bash hands Docker an MSYS path it cannot mount, and rewrites arguments
# that start with `/`. Both are no-ops on Linux.
export MSYS_NO_PATHCONV=1
if command -v cygpath >/dev/null 2>&1; then
	HOST_ROOT="$(cygpath -m "$REPO_ROOT")"
else
	HOST_ROOT="$REPO_ROOT"
fi

names=(smoke-web smoke-llm smoke-worker smoke-api smoke-s3 smoke-pg)

dump() {
	for c in "${names[@]}"; do
		if docker inspect "$c" >/dev/null 2>&1; then
			echo "::group::docker logs $c"
			docker logs --tail 80 "$c" 2>&1 || true
			echo "::endgroup::"
		fi
	done
}

cleanup() {
	rc=$?
	if [ "$rc" -ne 0 ]; then dump; fi
	# SMOKE_KEEP=1 leaves the stack up to be poked at by hand. Never set in CI:
	# the containers hold ports and a network the next run would collide with.
	if [ "${SMOKE_KEEP:-}" = "1" ]; then
		echo "stack-smoke: SMOKE_KEEP=1, leaving ${names[*]} and network $NET up" >&2
		return "$rc"
	fi
	docker rm -f "${names[@]}" >/dev/null 2>&1 || true
	docker network rm "$NET" >/dev/null 2>&1 || true
	return "$rc"
}
trap cleanup EXIT

hget() { # curl from inside the network; stdout comes back to this shell
	docker run --rm --network "$NET" "$CURL_IMAGE" "$@"
}

wait_for() { # wait_for <label> <seconds> <command...>
	label="$1"
	limit="$2"
	shift 2
	for _ in $(seq "$limit"); do
		if "$@" >/dev/null 2>&1; then return 0; fi
		sleep 1
	done
	echo "stack-smoke: $label never became ready within ${limit}s" >&2
	return 1
}

docker rm -f "${names[@]}" >/dev/null 2>&1 || true
docker network rm "$NET" >/dev/null 2>&1 || true
docker network create "$NET" >/dev/null

echo "--- Postgres"
docker run -d --name smoke-pg --network "$NET" \
	-e POSTGRES_USER=skillhub -e POSTGRES_PASSWORD=skillhub -e POSTGRES_DB=skillhub \
	"$PG_IMAGE" >/dev/null
# Over TCP: the image's init server answers on the socket before POSTGRES_DB exists.
wait_for "Postgres" 60 docker exec -e PGPASSWORD=skillhub smoke-pg psql -h 127.0.0.1 -U skillhub -d skillhub -tAc "select 1"

echo "--- Schema (db/migrations, in order)"
# The repository has no migration runner: the Go tests apply the files as one
# batch each, and so does this. `-1` is required -- 0031 takes a LOCK TABLE.
for f in "$REPO_ROOT"/db/migrations/*.sql; do
	docker exec -i smoke-pg psql -q -1 -v ON_ERROR_STOP=1 -U skillhub -d skillhub <"$f" >/dev/null
done

echo "--- Object store"
docker run -d --name smoke-s3 --network "$NET" \
	-v "$HOST_ROOT/infra/compose/seaweedfs-s3.json:/etc/seaweedfs/s3.json:ro" \
	"$S3_IMAGE" server -s3 -s3.config=/etc/seaweedfs/s3.json -dir=/data >/dev/null
wait_for "SeaweedFS" 90 docker exec smoke-s3 wget -q -O /dev/null http://127.0.0.1:8333/status

# `platform-api` is the name, not a label: infra/images/web/nginx.conf resolves
# `http://platform-api:8080` through Docker's embedded DNS, so the alias is part
# of what this test is checking.
api_env=(
	-e "DATABASE_URL=postgresql://skillhub:skillhub@smoke-pg:5432/skillhub"
	-e OBJSTORE_ENDPOINT=smoke-s3:8333
	-e OBJSTORE_ACCESS_KEY=skillhubdev
	-e OBJSTORE_SECRET_KEY=skillhubdevsecret
	-e OBJSTORE_BUCKET=skillhub
	-e OBJSTORE_SSL=0
	# ADR-020's offline provider, so the browser pass can look at the pages
	# behind a session. The API refuses to start with DEV_LOGIN=1 and a secure
	# cookie, so the two go together. This stack is thrown away at the end of
	# the run and is never reachable from outside the runner.
	-e COOKIE_INSECURE=1
	-e DEV_LOGIN=1
	# Without it, building a download answers 503 by design -- the value has no
	# default because it is a retention promise made to users, not a parameter
	# (GOV-RETENTION-001), and PDM-006's proposed 90 days is not ratified. `1h`
	# is chosen to be obviously throwaway: this stack is deleted at the end of
	# the run, and no one should be able to read a policy proposal out of it.
	# Set only so the packaging route is exercised rather than short-circuited.
	-e DOWNLOAD_ARTIFACT_RETENTION=1h
)

echo "--- platform-api"
docker run -d --name smoke-api --network "$NET" --network-alias platform-api \
	"${api_env[@]}" "$PLATFORM_IMAGE" api >/dev/null
wait_for "platform-api" 90 hget -fs -o /dev/null http://platform-api:8080/healthz

echo "--- platform-worker"
docker run -d --name smoke-worker --network "$NET" "${api_env[@]}" "$PLATFORM_IMAGE" worker >/dev/null

echo "--- web"
# `Accept: text/html` is not decoration. nginx.conf maps that header to
# $spa_fallback, so a request without it is treated as a fetch() and falls
# through to the API, which answers 404 -- the file says so at the HEALTHCHECK
# comment. Sending it is what makes these three checks browser-shaped.
docker run -d --name smoke-web --network "$NET" "$WEB_IMAGE" >/dev/null
wait_for "web" 60 hget -fs -o /dev/null -H "Accept: text/html" http://smoke-web/

# The third pushed image. It is started alone and on purpose: the compose file
# says a missing gateway is a request-time error and not a startup one, so a
# container that will not boot without LiteLLM is a regression in that promise.
echo "--- llm"
docker run -d --name smoke-llm --network "$NET" "$LLM_IMAGE" >/dev/null
wait_for "llm" 60 docker exec smoke-llm python -c \
	"import urllib.request;urllib.request.urlopen('http://127.0.0.1:8000/healthz').read()"

fail=0
check() { # check <description> <condition-exit-code>
	if [ "$2" -eq 0 ]; then
		echo "ok   $1"
	else
		echo "FAIL $1" >&2
		fail=1
	fi
}

echo "--- assertions"

# 1. The SPA, from the image that ships it.
body="$(hget -fsS -H "Accept: text/html" http://smoke-web/)"
case "$body" in *'id="root"'*) check "web serves the SPA shell" 0 ;; *) check "web serves the SPA shell" 1 ;; esac

# 2. The header nginx.conf's own test can only read as text. `-I` would take the
#    `= /healthz` branch, so this asks for the document the browser gets.
headers="$(hget -fsS -D - -o /dev/null -H "Accept: text/html" http://smoke-web/)"
case "$headers" in
*[Cc]ontent-[Ss]ecurity-[Pp]olicy*) check "web sends Content-Security-Policy" 0 ;;
*) check "web sends Content-Security-Policy" 1 ;;
esac

# 3. Both hops: browser -> nginx -> platform-api -> Postgres. The catalogue is
#    unauthenticated and does no embedding, so nothing here needs a model.
catalog="$(hget -fsS http://smoke-web/api/skills/catalog)" || catalog=""
node -e '
const raw = process.argv[1];
if (!raw) { console.error("empty body from /api/skills/catalog"); process.exit(1); }
const j = JSON.parse(raw);
// contracts/openapi/public.yaml CatalogResponse: results is required and typed
// array. Go writes a nil slice as null, and an empty catalogue is a nil slice.
if (!Array.isArray(j.results)) {
  console.error("results is " + JSON.stringify(j.results) + ", not an array");
  process.exit(1);
}
if (typeof j.total !== "number" || typeof j.truncated !== "boolean") {
  console.error("total/truncated missing or wrong type: " + raw.slice(0, 200));
  process.exit(1);
}
' "$catalog" && rc=0 || rc=1
check "catalogue answers through nginx with a contract-shaped body" "$rc"

# 4. A worker that exits is a worker that consumed nothing, and the API in front
#    of it stays green either way -- which is what makes this worth asserting.
sleep 5
running="$(docker inspect -f '{{.State.Running}}' smoke-worker)"
[ "$running" = "true" ] && rc=0 || rc=1
check "platform-worker is still running" "$rc"

# 5. A real browser against the real backend (04 丙-221). In its own container
#    on the same network: the browsers are already in the Playwright image, and
#    running it here rather than through a published port keeps this working the
#    same way on a laptop and on a runner. `npm i` fetches only the JS package
#    -- the image supplies the browser binaries.
echo "--- browser"
docker run --rm --network "$NET" \
	-v "$HOST_ROOT:/work:ro" -w /work \
	-e BASE_URL=http://smoke-web \
	"$PLAYWRIGHT_IMAGE" \
	sh -c 'cd /tmp && npm i --no-save --silent --no-audit --no-fund playwright@1.62.1 >/dev/null 2>&1 &&
	       cp /work/tools/ci/stack-browser.mjs /work/tools/ci/stack-seed.mjs /tmp/ && node /tmp/stack-browser.mjs' && rc=0 || rc=1
check "public routes render in a browser against the real API" "$rc"

if [ "$fail" -ne 0 ]; then
	echo "stack-smoke: at least one assertion failed" >&2
	exit 1
fi
echo "stack-smoke: all assertions passed"
