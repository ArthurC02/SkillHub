#!/usr/bin/env bash
set -euo pipefail

: "${POSTGRES_IMAGE:?set POSTGRES_IMAGE}"
: "${POSTGRES_EXPORTER_IMAGE:?set POSTGRES_EXPORTER_IMAGE}"
export MSYS_NO_PATHCONV=1
scope="skillhub.exporter-smoke=$RANDOM-$RANDOM-$RANDOM"

cleanup() {
  local containers networks
  local -a owned
  containers=$(docker ps -aq --filter "label=$scope")
  if [ -n "$containers" ]; then
    mapfile -t owned <<<"$containers"
    docker rm -fv "${owned[@]}" >/dev/null
  fi
  networks=$(docker network ls -q --filter "label=$scope")
  if [ -n "$networks" ]; then
    mapfile -t owned <<<"$networks"
    docker network rm "${owned[@]}" >/dev/null
  fi
}
trap cleanup EXIT

network=$(docker network create --internal --label "$scope" "skillhub-exporter-$RANDOM-$RANDOM")
database=$(docker run --rm -d --label "$scope" --network "$network" --network-alias database \
  --tmpfs /var/lib/postgresql -e POSTGRES_USER=skillhub -e POSTGRES_PASSWORD=smoke-only \
  -e POSTGRES_DB=skillhub "$POSTGRES_IMAGE" -c max_connections=40)
for _ in {1..40}; do
  if docker exec "$database" pg_isready -U skillhub -d skillhub >/dev/null 2>&1; then break; fi
  sleep 0.25
done
docker exec "$database" pg_isready -U skillhub -d skillhub
docker exec "$database" psql -U skillhub -d skillhub -v ON_ERROR_STOP=1 \
  -c "CREATE USER exporter WITH PASSWORD 'smoke-only'; GRANT pg_monitor TO exporter;"

check_exporter() {
  local password="$1" up="$2" error="$3" container metrics ready=0
  container=$(docker run --rm -d --label "$scope" --read-only --cap-drop ALL \
    --security-opt no-new-privileges --network "$network" --network-alias exporter \
    -e "DATA_SOURCE_NAME=postgresql://exporter:$password@database:5432/skillhub?sslmode=disable" \
    "$POSTGRES_EXPORTER_IMAGE")
  for _ in {1..40}; do
    if metrics=$(docker run --rm --label "$scope" --network "$network" --entrypoint /bin/wget \
      "$POSTGRES_EXPORTER_IMAGE" -T 2 -q -O - http://exporter:9187/metrics); then ready=1; break; fi
    sleep 0.25
  done
  if [ "$ready" != 1 ]; then docker logs "$container"; return 1; fi
  grep -Eq "^pg_up $up$" <<<"$metrics"
  grep -Eq "^pg_exporter_last_scrape_error $error$" <<<"$metrics"
  grep -Eq '^postgres_exporter_build_info\{.*version="0\.20\.1".*\} 1$' <<<"$metrics"
  if [ "$up" = 1 ]; then
    grep -Eq '^pg_settings_max_connections(\{[^}]*\})? 40$' <<<"$metrics"
  fi
  docker rm -fv "$container" >/dev/null
  echo "Postgres exporter: pg_up=$up, scrape_error=$error"
}

check_exporter smoke-only 1 0
check_exporter wrong-password 0 1
echo 'Postgres exporter smoke passed; valid metrics and rejected credentials observed'
