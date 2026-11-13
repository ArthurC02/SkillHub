#!/usr/bin/env bash
set -euo pipefail

: "${BLACKBOX_EXPORTER_IMAGE:?set BLACKBOX_EXPORTER_IMAGE}"
: "${PROBE_TARGET_IMAGE:?set PROBE_TARGET_IMAGE}"
export MSYS_NO_PATHCONV=1
scope="skillhub.blackbox-smoke=$RANDOM-$RANDOM-$RANDOM"

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

network=$(docker network create --internal --label "$scope" "skillhub-blackbox-$RANDOM-$RANDOM")
docker run --rm -d --label "$scope" --read-only --cap-drop ALL --security-opt no-new-privileges \
  --network "$network" --network-alias target "$PROBE_TARGET_IMAGE" python -B -u -c '
from http.server import BaseHTTPRequestHandler, HTTPServer
class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200 if self.path == "/ready" else 503)
        self.end_headers()
        self.wfile.write(b"probe-only\n")
HTTPServer(("0.0.0.0", 8000), Handler).serve_forever()
' >/dev/null
exporter=$(docker run --rm -d --label "$scope" --read-only --cap-drop ALL \
  --security-opt no-new-privileges --network "$network" --network-alias blackbox "$BLACKBOX_EXPORTER_IMAGE")

fetch() {
  docker run --rm --label "$scope" --network "$network" --entrypoint /bin/wget \
    "$BLACKBOX_EXPORTER_IMAGE" -T 5 -q -O - "$1"
}

ready=0
for _ in {1..40}; do
  if metrics=$(fetch http://blackbox:9115/metrics); then ready=1; break; fi
  sleep 0.25
done
if [ "$ready" != 1 ]; then docker logs "$exporter"; exit 1; fi
grep -Eq '^blackbox_exporter_build_info\{.*version="0\.28\.0".*\} 1$' <<<"$metrics"
for _ in {1..40}; do
  if fetch http://target:8000/ready >/dev/null; then ready=2; break; fi
  sleep 0.25
done
[ "$ready" = 2 ]

check_probe() {
  local module="$1" target="$2" success="$3" status="$4" metrics
  metrics=$(fetch "http://blackbox:9115/probe?module=$module&target=$target")
  grep -Eq "^probe_success $success$" <<<"$metrics"
  if [ -n "$status" ]; then grep -Eq "^probe_http_status_code $status$" <<<"$metrics"; fi
  echo "Blackbox probe: module=$module target=$target success=$success status=$status"
}

check_probe http_2xx http://target:8000/ready 1 200
check_probe http_2xx http://target:8000/broken 0 503
check_probe tcp_connect target:8000 1 ''
check_probe tcp_connect target:8001 0 ''
echo 'Blackbox exporter smoke passed; healthy and failed HTTP/TCP probes observed'
