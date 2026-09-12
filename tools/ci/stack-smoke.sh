#!/usr/bin/env bash
# Starts the platform/web/llm images this build produced as real containers
# and asserts they answer, then drives a real browser against them.
# Usage: PLATFORM_IMAGE=... WEB_IMAGE=... LLM_IMAGE=... bash tools/ci/stack-smoke.sh
set -euo pipefail

: "${PLATFORM_IMAGE:?PLATFORM_IMAGE must name the built platform image}"
: "${WEB_IMAGE:?WEB_IMAGE must name the built web image}"
: "${LLM_IMAGE:?LLM_IMAGE must name the built llm image}"

PG_IMAGE="docker.io/pgvector/pgvector:pg17@sha256:cf134a767f474095eeba57e0117be8e568e011a63f33fbf252f14c9b760f8e6f"
S3_IMAGE="docker.io/chrislusf/seaweedfs:4.46@sha256:08d516132314207d10c8e37cbffc1f32b147d870169688734cc61c6231625b62"
# Runs every check from inside the docker network rather than through a
# published host port, which also exercises the DNS name nginx.conf dials.
CURL_IMAGE="docker.io/curlimages/curl@sha256:c1fe1679c34d9784c1b0d1e5f62ac0a79fca01fb6377cdd33e90473c6f9f9a69"
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
# `-1` runs each file as one batch/transaction; at least one migration takes a
# table lock that requires it.
for f in "$REPO_ROOT"/db/migrations/*.sql; do
	docker exec -i smoke-pg psql -q -1 -v ON_ERROR_STOP=1 -U skillhub -d skillhub <"$f" >/dev/null
done

echo "--- Object store"
docker run -d --name smoke-s3 --network "$NET" \
	-v "$HOST_ROOT/infra/compose/seaweedfs-s3.json:/etc/seaweedfs/s3.json:ro" \
	"$S3_IMAGE" server -s3 -s3.config=/etc/seaweedfs/s3.json -dir=/data >/dev/null
wait_for "SeaweedFS" 90 docker exec smoke-s3 wget -q -O /dev/null http://127.0.0.1:8333/status

# `platform-api` is a network alias, not just a container name: nginx.conf
# resolves that exact hostname, so it is part of what this test checks.
api_env=(
	-e "DATABASE_URL=postgresql://skillhub:skillhub@smoke-pg:5432/skillhub"
	-e OBJSTORE_ENDPOINT=smoke-s3:8333
	-e OBJSTORE_ACCESS_KEY=skillhubdev
	-e OBJSTORE_SECRET_KEY=skillhubdevsecret
	-e OBJSTORE_BUCKET=skillhub
	-e OBJSTORE_SSL=0
	# Enables the offline login provider for the browser pass; the API requires
	# COOKIE_INSECURE=1 alongside it.
	-e COOKIE_INSECURE=1
	-e DEV_LOGIN=1
	# Required (no code default) so the packaging route is exercised rather
	# than short-circuited by a 503; set to an obviously-throwaway value.
	-e DOWNLOAD_ARTIFACT_RETENTION=1h
)

echo "--- platform-api"
docker run -d --name smoke-api --network "$NET" --network-alias platform-api \
	"${api_env[@]}" "$PLATFORM_IMAGE" api >/dev/null
wait_for "platform-api" 90 hget -fs -o /dev/null http://platform-api:8080/healthz

echo "--- platform-worker"
docker run -d --name smoke-worker --network "$NET" "${api_env[@]}" "$PLATFORM_IMAGE" worker >/dev/null

echo "--- web"
# nginx.conf routes on the Accept header: without text/html a request is
# treated as a fetch() and falls through to the API instead of the SPA shell.
docker run -d --name smoke-web --network "$NET" "$WEB_IMAGE" >/dev/null
wait_for "web" 60 hget -fs -o /dev/null -H "Accept: text/html" http://smoke-web/

# Started with no gateway on purpose: a missing LiteLLM must be a request-time
# error, never a startup failure.
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

# 2. `-I` would take nginx.conf's `= /healthz` branch, so this asks for the
#    actual document a browser gets instead.
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

sleep 5
running="$(docker inspect -f '{{.State.Running}}' smoke-worker)"
[ "$running" = "true" ] && rc=0 || rc=1
check "platform-worker is still running" "$rc"

# 5. `npm i` fetches only the JS package; the Playwright image already
#    supplies the browser binaries.
echo "--- browser"
docker run --rm --network "$NET" \
	-v "$HOST_ROOT:/work:ro" -w /work \
	-e BASE_URL=http://smoke-web \
	"$PLAYWRIGHT_IMAGE" \
	sh -c 'cd /tmp && npm i --no-save --silent --no-audit --no-fund playwright@1.62.1 >/dev/null 2>&1 &&
	       cp /work/tools/ci/stack-browser.mjs /work/tools/ci/stack-seed.mjs /tmp/ && node /tmp/stack-browser.mjs' && rc=0 || rc=1
check "public routes render in a browser against the real API" "$rc"

# 6. Credit on the real images: refused at 0, an operator grant, then allowed.
#    Operator ids and the creation entry are read at startup, so the API restarts
#    with them; nginx restarts too so it dials the new container.
echo "--- credit"
login="$(hget -sS -o /dev/null -w '%{http_code}' -X POST -H 'content-type: application/json' \
	-d '{"user":"smoke-operator"}' http://platform-api:8080/auth/dev/login)" || login=""
operator="$(docker exec -e PGPASSWORD=skillhub smoke-pg psql -h 127.0.0.1 -U skillhub -d skillhub -tAc \
	"select id from users where email = 'smoke-operator@dev.local'")" || operator=""
limits="$(grep '^CREATION_LIMITS_JSON=' "$REPO_ROOT/.env.example" | cut -d= -f2-)"
if [ "$login" = 204 ] && [ -n "$operator" ] && [ -n "$limits" ]; then
	docker rm -f smoke-api >/dev/null
	docker run -d --name smoke-api --network "$NET" --network-alias platform-api \
		"${api_env[@]}" \
		-e "OPERATOR_USER_IDS=$operator" -e CREATION_EXPOSED=on -e GENERATE_SKILL_EXPOSED=on \
		-e "CREATION_LIMITS_JSON=$limits" -e CREATION_WORKER_INTERNAL_ADDR=:8091 \
		-e CREATION_WORKER_INTERNAL_URL=http://smoke-worker:8091 -e CREATION_WORKER_INTERNAL_TOKEN=smoke-only \
		"$PLATFORM_IMAGE" api >/dev/null
	docker restart smoke-web >/dev/null
	if wait_for "platform-api (credit)" 90 hget -fs -o /dev/null http://platform-api:8080/healthz &&
		wait_for "web (credit)" 60 hget -fs -o /dev/null -H "Accept: text/html" http://smoke-web/ &&
		docker run --rm --network "$NET" -v "$HOST_ROOT:/work:ro" -w /work -e BASE_URL=http://smoke-web \
			"$PLAYWRIGHT_IMAGE" \
			sh -c 'cd /tmp && npm i --no-save --silent --no-audit --no-fund playwright@1.62.1 >/dev/null 2>&1 &&
			       cp /work/tools/ci/stack-credit.mjs /tmp/ && node /tmp/stack-credit.mjs'; then
		rc=0
	else
		rc=1
	fi
else
	echo "credit setup: login=$login operator=${operator:-none} limits=${limits:+set}" >&2
	rc=1
fi
check "credit refuses at 0 and an operator grant unblocks it, on the real images" "$rc"

if [ "$fail" -ne 0 ]; then
	echo "stack-smoke: at least one assertion failed" >&2
	exit 1
fi
echo "stack-smoke: all assertions passed"
