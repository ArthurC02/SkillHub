#!/usr/bin/env bash
set -euo pipefail

: "${ALERTMANAGER_IMAGE:?set ALERTMANAGER_IMAGE}"
: "${PROBE_TARGET_IMAGE:?set PROBE_TARGET_IMAGE}"
export MSYS_NO_PATHCONV=1
scope="skillhub.alertmanager-smoke=$RANDOM-$RANDOM-$RANDOM"
WORK="$(mktemp -d)"
WORK_HOST="$(cd "$WORK" && (pwd -W 2>/dev/null || pwd))"
chmod 755 "$WORK"

cleanup() {
  local status="$?" containers networks container
  local -a owned
  containers=$(docker ps -aq --filter "label=$scope")
  if [ -n "$containers" ]; then
    mapfile -t owned <<<"$containers"
    if [ "$status" -ne 0 ]; then
      for container in "${owned[@]}"; do docker logs "$container" >&2 || true; done
    fi
    docker rm -fv "${owned[@]}" >/dev/null
  fi
  networks=$(docker network ls -q --filter "label=$scope")
  if [ -n "$networks" ]; then
    mapfile -t owned <<<"$networks"
    docker network rm "${owned[@]}" >/dev/null
  fi
  rm -f "$WORK/alertmanager.yml" "$WORK/payload.json" "$WORK/query.json"
  rmdir "$WORK"
  return "$status"
}
trap cleanup EXIT

for binary in alertmanager amtool; do
  version=$(docker run --rm --label "$scope" --network none --read-only --cap-drop ALL \
    --entrypoint "/bin/$binary" "$ALERTMANAGER_IMAGE" --version)
  [[ "$version" = *"version 0.34.2"* ]]
done

cat >"$WORK/alertmanager.yml" <<'YAML'
route:
  receiver: probe
  group_by: [alertname]
  group_wait: 0s
  group_interval: 1s
  repeat_interval: 1h
receivers:
  - name: probe
    webhook_configs:
      - url: http://sink:8000
        send_resolved: true
YAML
chmod 644 "$WORK/alertmanager.yml"
network=$(docker network create --internal --label "$scope" "skillhub-alertmanager-$RANDOM-$RANDOM")
docker run --rm -d --label "$scope" --read-only --cap-drop ALL --security-opt no-new-privileges \
  --network "$network" --network-alias sink "$PROBE_TARGET_IMAGE" python -B -u -c '
from http.server import BaseHTTPRequestHandler, HTTPServer
import json

events = []

class Sink(BaseHTTPRequestHandler):
    def do_POST(self):
        events.append(json.loads(self.rfile.read(int(self.headers["Content-Length"]))))
        self.send_response(200)
        self.end_headers()

    def do_GET(self):
        assert self.path == "/events"
        data = json.dumps(events).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

HTTPServer(("0.0.0.0", 8000), Sink).serve_forever()
' >/dev/null
docker run -d --label "$scope" --read-only --cap-drop ALL --security-opt no-new-privileges \
  --network "$network" --network-alias alertmanager \
  --mount "type=bind,src=$WORK_HOST,dst=/fixture,readonly" \
  --tmpfs /alertmanager:rw,noexec,nosuid,size=16m,uid=65534,gid=65534 \
  "$ALERTMANAGER_IMAGE" --config.file=/fixture/alertmanager.yml --storage.path=/alertmanager --cluster.listen-address= >/dev/null

probe() {
  docker run --rm -i --label "$scope" --network "$network" --read-only --cap-drop ALL \
    --mount "type=bind,src=$WORK_HOST,dst=/fixture,readonly" "$PROBE_TARGET_IMAGE" python -B - "$1" <<'PY'
from datetime import datetime, timedelta, timezone
from pathlib import Path
import gzip
import json
import re
import sys
import time
from urllib.error import HTTPError, URLError
from urllib.parse import urljoin, urlsplit
from urllib.request import Request, urlopen

base = 'http://alertmanager:9093'

def read(path, data=None, headers=None):
    with urlopen(Request(base + path, data=data, headers=headers or {}), timeout=2) as response:
        return response.status, response.headers, response.read()

def wait_for(function):
    end = time.monotonic() + 10
    while time.monotonic() < end:
        try:
            result = function()
            if result:
                return result
        except (URLError, ConnectionError):
            pass
        time.sleep(0.05)
    raise AssertionError('Expected service/event observation did not arrive')

def delivered(state):
    with urlopen('http://sink:8000/events', timeout=2) as response:
        events = json.load(response)
    return any(event['status'] == state and event['receiver'] == 'probe' and any(alert['labels'].get('alertname') == 'OwnedImageProbe' and alert['status'] == state for alert in event['alerts']) for event in events)

wait_for(lambda: read('/-/ready')[0] == 200)
if sys.argv[1] == 'start':
    status, _, page = read('/')
    assert status == 200 and b'Alertmanager' in page
    match = re.search(rb'<script[^>]+src="([^"]+)"', page)
    assert match is not None, page[:500]
    asset_url = urlsplit(urljoin(base + '/', match[1].decode()))
    assert asset_url.netloc == 'alertmanager:9093' and asset_url.scheme == 'http'
    asset = asset_url.path
    assert asset.startswith('/assets/') and asset.endswith('.js'), asset
    status, headers, script = read(asset)
    assert status == 200 and 'javascript' in headers['Content-Type'] and len(script) > 1000
    status, headers, compressed = read(asset, headers={'Accept-Encoding': 'gzip'})
    assert status == 200 and headers['Content-Encoding'] == 'gzip' and gzip.decompress(compressed) == script
    try:
        read('/api/v2/alerts', data=b'{', headers={'Content-Type': 'application/json'})
    except HTTPError as error:
        assert error.code == 400, error.code
    else:
        raise AssertionError('Malformed alert JSON was accepted')
    now = datetime.now(timezone.utc)
    alert = {'labels': {'alertname': 'OwnedImageProbe', 'severity': 'warning'}, 'annotations': {'summary': 'Local image verification'}, 'startsAt': (now - timedelta(minutes=2)).isoformat(), 'endsAt': (now + timedelta(minutes=2)).isoformat()}
    assert read('/api/v2/alerts', json.dumps([alert]).encode(), {'Content-Type': 'application/json'})[0] == 200
    active = json.loads(read('/api/v2/alerts')[2])
    assert len(active) == 1 and active[0]['labels'] == alert['labels'] and active[0]['status']['state'] == 'active', active
    wait_for(lambda: delivered('firing'))
    print(json.dumps({'alert': alert, 'ui_and_gzip': True, 'malformed_json_refused': True, 'active_alert': True, 'firing_webhook': True}))
else:
    assert sys.argv[1] == 'resolve'
    alert = json.loads(Path('/fixture/payload.json').read_text())['alert']
    query = json.loads(Path('/fixture/query.json').read_text())
    assert len(query) == 1 and query[0]['labels'] == alert['labels'] and query[0]['status']['state'] == 'active', query
    alert['endsAt'] = (datetime.now(timezone.utc) - timedelta(seconds=1)).isoformat()
    assert read('/api/v2/alerts', json.dumps([alert]).encode(), {'Content-Type': 'application/json'})[0] == 200
    wait_for(lambda: delivered('resolved'))
    print(json.dumps({'resolved_webhook': True}))
PY
}

probe start >"$WORK/payload.json"
docker run --rm --label "$scope" --network "$network" --read-only --cap-drop ALL \
  --entrypoint /bin/amtool "$ALERTMANAGER_IMAGE" --alertmanager.url=http://alertmanager:9093 \
  --output=json alert query alertname=OwnedImageProbe >"$WORK/query.json"
chmod 644 "$WORK/payload.json" "$WORK/query.json"
probe resolve
echo 'Alertmanager smoke passed; UI/gzip, rejected JSON, active alert, amtool and firing/resolved local webhooks observed'
