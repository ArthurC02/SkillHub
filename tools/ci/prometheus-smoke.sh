#!/usr/bin/env bash
set -euo pipefail

: "${PROMETHEUS_IMAGE:?set PROMETHEUS_IMAGE}"
: "${PROBE_TARGET_IMAGE:?set PROBE_TARGET_IMAGE}"
export MSYS_NO_PATHCONV=1
ROOT="$(cd "$(dirname "$0")/../.." && (pwd -W 2>/dev/null || pwd))"
BASELINE=prom/prometheus:v3.16.0@sha256:e04987641b57704f408e31f6373ba4138ecf27cb4a165d24aabb6c38fddd0f06
scope="skillhub.prometheus-smoke=$RANDOM-$RANDOM-$RANDOM"
WORK="$(mktemp -d)"
WORK_HOST="$(cd "$WORK" && (pwd -W 2>/dev/null || pwd))"
chmod 755 "$WORK"
network=""
volume=""
server=""
cleanup() {
  status=$?
  if [ "$status" -ne 0 ]; then
    for id in $(docker ps -aq --filter "label=$scope"); do docker logs "$id" >&2 || true; done
  fi
  for id in $(docker ps -aq --filter "label=$scope"); do docker rm -fv "$id" >/dev/null || true; done
  if [ -n "$network" ]; then docker network rm "$network" >/dev/null || true; fi
  if [ -n "$volume" ]; then docker volume rm "$volume" >/dev/null || true; fi
  rm -rf "$WORK"
  exit "$status"
}
trap cleanup EXIT

for binary in prometheus promtool; do
  version=$(docker run --rm --label "$scope" --network none --read-only --cap-drop ALL \
    --entrypoint "/bin/$binary" "$PROMETHEUS_IMAGE" --version 2>&1)
  [[ "$version" = *"version 3.16.0"* ]]
  [[ "$version" = *"revision: e2e28adeee2c5883d000fc82aca6008cfbf7e96d"* ]]
done
cat >"$WORK/prometheus.yml" <<'YAML'
global:
  scrape_interval: 1s
  evaluation_interval: 1s
rule_files: [/fixture/rules.yml]
alerting:
  alertmanagers:
    - static_configs:
        - targets: [target:8000]
scrape_configs:
  - job_name: probe
    static_configs:
      - targets: [target:8000]
YAML
cat >"$WORK/rules.yml" <<'YAML'
groups:
  - name: probe
    rules:
      - alert: OwnedImageHigh
        expr: owned_image_gauge > 42
        labels:
          severity: warning
YAML
cat >"$WORK/client.py" <<'PY'
import gzip
import hashlib
import json
import re
import sys
import time
from urllib.error import HTTPError, URLError
from urllib.parse import urlencode, urljoin, urlsplit
from urllib.request import Request, urlopen

base = 'http://prometheus:9090'

def get(path):
    with urlopen(base + path, timeout=3) as response:
        return response.read()

def wait_for(function):
    end = time.monotonic() + 15
    while time.monotonic() < end:
        try:
            result = function()
            if result:
                return result
        except (URLError, ConnectionError):
            pass
        time.sleep(0.1)
    raise AssertionError('Expected metric/readiness/notification observation did not arrive')

def value(expected, at=None):
    params = {'query': 'owned_image_gauge'}
    if at is not None:
        params['time'] = at
    response = json.loads(get('/api/v1/query?' + urlencode(params)))
    assert response['status'] == 'success'
    vector = response['data']['result']
    return vector if len(vector) == 1 and vector[0]['value'][1] == str(expected) else None


def ui():
    page = get('/query')
    assert b'Prometheus' in page
    match = re.search(rb'<script[^>]+src="([^"]+)"', page)
    assert match is not None, page[:800]
    asset = urlsplit(urljoin(base + '/query', match[1].decode()))
    assert asset.scheme == 'http' and asset.netloc == 'prometheus:9090'
    with urlopen(asset.geturl(), timeout=3) as response:
        script = response.read()
        assert response.status == 200 and 'javascript' in response.headers['Content-Type'] and len(script) > 1000
    with urlopen(Request(asset.geturl(), headers={'Accept-Encoding': 'gzip'}), timeout=3) as response:
        encoding = response.headers.get('Content-Encoding', 'identity')
        assert encoding in ('gzip', 'identity'), encoding
        body = response.read()
        assert (gzip.decompress(body) if encoding == 'gzip' else body) == script
    return {'asset': asset.path, 'sha256': hashlib.sha256(script).hexdigest(), 'negotiated_encoding': encoding}

wait_for(lambda: get('/-/ready') == b'Prometheus Server is Ready.\n')
for payload, expected in ((b'\x00', 200), (b'\x80\x80\x80\x10', 400), (b'\x81\x80\x80\x10', 400)):
    request = Request(base + '/api/v1/read', data=payload, headers={'Content-Type': 'application/x-protobuf', 'Content-Encoding': 'snappy', 'X-Prometheus-Remote-Read-Version': '0.1.0'})
    try:
        with urlopen(request, timeout=3) as response:
            assert response.status == expected
    except HTTPError as error:
        body = error.read()
        assert error.code == expected == 400
        assert (b'decoded length 33554433 exceeds limit 33554432' in body) if payload[0] == 129 else (b'snappy: corrupt input' in body)
assert get('/-/ready') == b'Prometheus Server is Ready.\n'
if sys.argv[1] == 'baseline':
    vector = wait_for(lambda: value(42))
    print(json.dumps({'historical_time': vector[0]['value'][0], 'baseline_value': 42, 'sample': vector[0], 'ui': ui()}))
else:
    assert sys.argv[1] == 'rebuilt'
    original = json.load(open('/fixture/previous.json'))
    with urlopen(Request('http://target:8000/control', data=b'{"value":43}', headers={'Content-Type': 'application/json'}), timeout=3) as response:
        assert response.status == 200
    wait_for(lambda: value(43))
    assert value(42, original['historical_time']) is not None, 'Historical data was not preserved across the binary replacement'
    try:
        get('/api/v1/query?' + urlencode({'query': 'owned_image_gauge{'}))
    except HTTPError as error:
        assert error.code == 400 and json.load(error)['errorType'] == 'bad_data'
    else:
        raise AssertionError('Malformed PromQL was accepted')
    observed_ui = ui()
    assert observed_ui == original['ui'], (observed_ui, original['ui'])
    def notified():
        with urlopen('http://target:8000/events', timeout=3) as response:
            return any(row['labels'].get('alertname') == 'OwnedImageHigh' and row['labels'].get('severity') == 'warning' for row in json.load(response))
    wait_for(notified)
    print(json.dumps({'new_scrape_value': 43, 'historical_sample_preserved': True, 'malformed_promql_refused': True, 'ui_asset_preserved': True, 'rule_notification_observed': True}))

PY
chmod 644 "$WORK/prometheus.yml" "$WORK/rules.yml" "$WORK/client.py"
network=$(docker network create --internal --label "$scope" "skillhub-prometheus-$RANDOM-$RANDOM-$RANDOM")
volume=$(docker volume create --label "$scope")
docker run -d --label "$scope" --network "$network" --network-alias target \
  --read-only --cap-drop ALL --security-opt no-new-privileges \
  "$PROBE_TARGET_IMAGE" python -B -u -c '
from http.server import BaseHTTPRequestHandler, HTTPServer
import json

state = {"value": 42, "alerts": []}

class Target(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == "/metrics":
            data = ("# TYPE owned_image_gauge gauge\nowned_image_gauge %d\n" % state["value"]).encode()
        elif self.path == "/events":
            data = json.dumps(state["alerts"]).encode()
        else:
            self.send_error(404)
            return
        self.send_response(200)
        self.send_header("Content-Type", "text/plain; version=0.0.4" if self.path == "/metrics" else "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_POST(self):
        data = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        if self.path == "/control":
            state["value"] = data["value"]
        elif self.path == "/api/v2/alerts":
            state["alerts"].extend(data)
        else:
            self.send_error(404)
            return
        self.send_response(200)
        self.end_headers()

HTTPServer(("0.0.0.0", 8000), Target).serve_forever()
' >/dev/null

start_server() {
  server=$(docker run -d --label "$scope" --network "$network" --network-alias prometheus \
    --read-only --cap-drop ALL --security-opt no-new-privileges --memory 512m \
    --mount "type=bind,src=$WORK_HOST,dst=/fixture,readonly" \
    --mount "type=volume,src=$volume,dst=/prometheus" \
    "$1" --config.file=/fixture/prometheus.yml --storage.tsdb.path=/prometheus --storage.tsdb.wal-segment-size=32MB)
}
probe() {
  docker run --rm --label "$scope" --network "$network" --read-only --cap-drop ALL \
    --mount "type=bind,src=$WORK_HOST,dst=/fixture,readonly" \
    "$PROBE_TARGET_IMAGE" python -B /fixture/client.py "$1"
}
start_server "$BASELINE"
probe baseline >"$WORK/previous.json"
chmod 644 "$WORK/previous.json"
docker stop "$server" >/dev/null
docker rm "$server" >/dev/null
start_server "$PROMETHEUS_IMAGE"
probe rebuilt
for command in check test; do
  file=alerts.yml
  if [ "$command" = test ]; then file=alerts_test.yml; fi
  result=$(docker run --rm --label "$scope" --network none --read-only --cap-drop ALL \
    --tmpfs /tmp:rw,noexec,nosuid,size=256m,mode=1777 \
    --mount "type=bind,src=$ROOT/infra/observability,dst=/rules,readonly" -w /rules \
    --entrypoint /bin/promtool "$PROMETHEUS_IMAGE" "$command" rules "/rules/$file" 2>&1)
  printf '%s\n' "$result"
  [[ "$result" = *SUCCESS* ]]
done
echo 'Prometheus smoke passed; old data preserved, new scrape/query, malformed PromQL, original UI asset, rule notification and remote-read boundaries observed; zero skips'
