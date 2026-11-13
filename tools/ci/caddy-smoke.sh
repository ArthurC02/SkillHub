#!/usr/bin/env bash
set -euo pipefail

: "${CADDY_IMAGE:?set CADDY_IMAGE}"
: "${PROBE_TARGET_IMAGE:?set PROBE_TARGET_IMAGE}"
ROOT="$(cd "$(dirname "$0")/../.." && (pwd -W 2>/dev/null || pwd))"
export MSYS_NO_PATHCONV=1
scope="skillhub.caddy-smoke=$RANDOM-$RANDOM-$RANDOM"

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

version=$(docker run --rm --label "$scope" "$CADDY_IMAGE" caddy version)
[[ "$version" = "v2.11.9 "* ]]
docker run --rm --label "$scope" --tmpfs /data --tmpfs /config \
  -e SKILLHUB_DOMAIN=skillhub.example -e SKILLHUB_ACME_EMAIL=owner@skillhub.example \
  --mount "type=bind,src=$ROOT/infra/deploy/control-plane/Caddyfile,dst=/etc/caddy/Caddyfile,readonly" \
  "$CADDY_IMAGE" caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile

network=$(docker network create --internal --label "$scope" "skillhub-caddy-$RANDOM-$RANDOM")
backend=$(docker run --rm -d --label "$scope" --read-only --cap-drop ALL --security-opt no-new-privileges \
  --network "$network" --network-alias web "$PROBE_TARGET_IMAGE" python -B -u -c '
from http.server import BaseHTTPRequestHandler, HTTPServer
class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200 if self.path == "/" else 404)
        self.end_headers()
        self.wfile.write(b"proxy-smoke\n" * 1024 if self.path == "/" else b"missing\n")
HTTPServer(("0.0.0.0", 80), Handler).serve_forever()
')
docker run --rm -d --label "$scope" --read-only --cap-drop ALL --security-opt no-new-privileges \
  --tmpfs /data --tmpfs /config --network "$network" --network-alias proxy \
  -e SKILLHUB_DOMAIN=http://:8080 -e SKILLHUB_ACME_EMAIL=owner@skillhub.example \
  --mount "type=bind,src=$ROOT/infra/deploy/control-plane/Caddyfile,dst=/etc/caddy/Caddyfile,readonly" \
  "$CADDY_IMAGE" >/dev/null

probe() {
  docker run --rm -i --label "$scope" --network "$network" "$PROBE_TARGET_IMAGE" python -B - "$1" <<'PY'
from compression import zstd
import gzip
from http.client import HTTPConnection
import socket
import sys
import time

if sys.argv[1] == "healthy":
    for host, port in (("web", 80), ("proxy", 8080)):
        for _ in range(40):
            try:
                with socket.create_connection((host, port), timeout=1):
                    break
            except OSError:
                time.sleep(0.25)
        else:
            raise AssertionError("fixture listener unavailable: " + host)
    cases = (("/", "identity", 200), ("/", "gzip", 200), ("/", "zstd", 200), ("/missing", "identity", 404))
else:
    cases = (("/", "identity", 502),)
for path, encoding, status in cases:
    connection = HTTPConnection("proxy", 8080, timeout=5)
    connection.request("GET", path, headers={"Accept-Encoding": encoding})
    response = connection.getresponse()
    body = response.read()
    assert response.status == status, (path, encoding, response.status)
    if sys.argv[1] == "healthy":
        assert response.getheader("Strict-Transport-Security") == "max-age=31536000"
        assert response.getheader("Server") is None
    if status == 200:
        assert response.getheader("Content-Encoding") == (None if encoding == "identity" else encoding)
        if encoding == "gzip": body = gzip.decompress(body)
        if encoding == "zstd": body = zstd.decompress(body)
        assert body == b"proxy-smoke\n" * 1024
    if status == 404:
        assert body == b"missing\n"
    connection.close()
    print("Caddy proxy: path=%s encoding=%s status=%d" % (path, encoding, status))
PY
}

probe healthy
docker rm -fv "$backend" >/dev/null
probe unavailable
echo 'Caddy smoke passed; identity/gzip/zstd, security headers, 404 and unavailable upstream observed'
