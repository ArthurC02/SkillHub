#!/usr/bin/env bash
set -euo pipefail

CADDY=caddy:2.11.4@sha256:13ba145cba2f3e28fa801994876e4c086d1b95d5aa2a520a734765ffb6b12017
ALERTMANAGER=prom/alertmanager:v0.34.1@sha256:e9733bafb1bdef9b00e25a21f8f99dc26a22224bf16641ad754d1649f4c3357a
PROMETHEUS=prom/prometheus:v3.14.0@sha256:5ce7540c3c00ef4ab0c9d2c995c6a5b9c421f44b4a115d97a2c7af3b1c21cbb0
SHELLCHECK=koalaman/shellcheck:v0.11.0@sha256:61862eba1fcf09a484ebcc6feea46f1782532571a34ed51fedf90dd25f925a8d
UBUNTU=ubuntu:24.04@sha256:69cecf4bbf72d2d44a9eef1b71fb98c7fb973d78af11399deccef19beb008ad9

ROOT="$(cd "$(dirname "$0")/../.." && (pwd -W 2>/dev/null || pwd))"
CP="$ROOT/infra/deploy/control-plane"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
WORK_HOST="$(cd "$WORK" && (pwd -W 2>/dev/null || pwd))"
export MSYS_NO_PATHCONV=1
OWNER="$(id -u):$(id -g)"

step() { printf '\n== %s\n' "$1"; }

cat >"$WORK/settings" <<'EOF'
SKILLHUB_DOMAIN=skillhub.example
SKILLHUB_ACME_EMAIL=owner@skillhub.example
SKILLHUB_PRIVATE_IP=10.0.0.2
SKILLHUB_ALERT_EMAIL=owner@skillhub.example
SKILLHUB_SMTP_SMARTHOST=smtp.example:587
SKILLHUB_SMTP_FROM=alerts@skillhub.example
SKILLHUB_SMTP_USERNAME=alerts@skillhub.example
SKILLHUB_GATEWAY_URL=http://10.0.0.3:4000
EOF

step "user-data renders, every node checkout carries what it runs, and the renderers' own tests pass"
docker run --rm -e OWNER="$OWNER" -v "$ROOT:/repo:ro" -v "$WORK_HOST:/work" -w /repo/tools/deploy "$UBUNTU" bash -euc '
  apt-get update -qq >/dev/null
  DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends \
    cloud-init systemd systemd-journal-remote conntrack git python3-yaml >/dev/null
  git config --global --add safe.directory /repo
  python3 test_render.py
  python3 test_checkout.py
  python3 test_sandbox_node.py
  python3 - <<"PY"
import render
digest = lambda repository, tag: "sha256:" + "0" * 64
release = "0" * 40
settings = {
    "control-plane": render.read_settings(open("/work/settings").read()),
    "gateway": {"SKILLHUB_PRIVATE_IP": "10.0.0.3"},
    "sandbox": {"SKILLHUB_PRIVATE_IP": "10.0.0.4", "SKILLHUB_CONTROL_PLANE_IP": "10.0.0.2", "SKILLHUB_SANDBOX_SLOTS": "2"},
}
for role, values in settings.items():
    env = render.release_env(role, release, values, resolve=digest,
                             read_file=lambda release, path: open("/repo/" + path).read())
    open("/work/%s-release.env" % role, "w").write(env)
    open("/work/%s-user-data.yaml" % role, "w").write(render.cloud_init(env))
PY
  for role in control-plane gateway sandbox; do
    cloud-init schema --config-file "/work/$role-user-data.yaml"
    python3 checkout.py "$role" --stage "/work/checkout-$role"
  done

  sandbox=/work/checkout-sandbox
  python3 "$sandbox/tools/egress/render.py" --out /work/sandbox-egress --sandbox-iface docker0 --control-plane 10.0.0.2
  python3 -m json.tool "$sandbox/infra/deploy/sandbox/daemon.json" >/dev/null
  python3 - "$sandbox" <<"PY"
import importlib.util, sys
spec = importlib.util.spec_from_file_location("probe", sys.argv[1] + "/tools/sec009/t8-node-probe.py")
probe = importlib.util.module_from_spec(spec)
spec.loader.exec_module(probe)
probe.CRED_PATHS = (sys.argv[1],)
report = probe.Report()
probe.check_p05(report)
row = report.rows[0]
print("P-05 on the sandbox checkout: %s -- %s" % (row["status"], row["detail"]))
sys.exit(0 if row["status"] == probe.PASS else 1)
PY

  for script in control-plane/bin/skillhub-preflight control-plane/bin/skillhub-alert control-plane/bin/skillhub-egress-retention gateway/bin/skillhub-preflight \
                sandbox/bin/skillhub-preflight sandbox/bin/skillhub-mark-serving; do
    mkdir -p "$(dirname "/opt/skillhub/infra/deploy/$script")"
    printf "#!/bin/sh\n" >"/opt/skillhub/infra/deploy/$script"
    chmod +x "/opt/skillhub/infra/deploy/$script"
  done
  for binary in /usr/bin/docker /usr/local/bin/sandboxd; do printf "#!/bin/sh\n" >"$binary" && chmod +x "$binary"; done
  install -m 0644 /repo/infra/deploy/control-plane/systemd/* /etc/systemd/system/
  install -D -m 0644 /repo/infra/deploy/control-plane/journal-remote/service.conf /etc/systemd/system/systemd-journal-remote.service.d/skillhub.conf
  cd /etc/systemd/system
  schedule=$(sed -n "s/^\([a-z]*\) \([a-z-]*\)$/skillhub-\1@\2.timer/p" /repo/infra/deploy/control-plane/maintenance-schedule)
  report=$(systemd-analyze verify skillhub.service skillhub-backup.timer skillhub-restore-drill.timer \
    skillhub-egress-retention.timer skillhub-alert@skillhub-egress-retention.service $schedule \
    systemd-journal-remote.service 2>&1 | grep -v "docker.service" || true)
  if [ -n "$report" ]; then printf "%s\n" "$report"; exit 1; fi
  for role in gateway sandbox; do
    mkdir -p "/$role" && install -m 0644 /repo/infra/deploy/$role/systemd/* "/$role/"
    report=$(cd "/$role" && systemd-analyze verify ./*.service 2>&1 | grep -v "docker.service\|nftables.service" || true)
    if [ -n "$report" ]; then printf "%s\n" "$report"; exit 1; fi
  done
  echo "systemd units verify clean"

  journals=$(mktemp -d)
  touch -d "91 days ago" "$journals/old.journal"
  touch -d "89 days ago" "$journals/recent.journal"
  SKILLHUB_REMOTE_JOURNAL=$journals sh /repo/infra/deploy/control-plane/bin/skillhub-egress-retention
  if [ -e "$journals/old.journal" ] || [ ! -e "$journals/recent.journal" ]; then
    echo "retention must delete records past 90 days and keep younger ones: $(ls "$journals")"; exit 1
  fi
  truncate -s 4G "$journals/large.journal"
  if SKILLHUB_REMOTE_JOURNAL=$journals sh /repo/infra/deploy/control-plane/bin/skillhub-egress-retention; then
    echo "retention did not alarm on a remote journal near its cap"; exit 1
  fi
  echo "egress record retention keeps 90 days and alarms near the cap"
  chown -R "$OWNER" /work
'

step "the sandbox ruleset, rendered the way bootstrap renders it, parses"
docker run --rm --cap-add NET_ADMIN -v "$WORK_HOST/sandbox-egress:/egress:ro" "$UBUNTU" bash -euc '
  apt-get update -qq >/dev/null && apt-get install -y -qq --no-install-recommends nftables >/dev/null
  nft -c -f /egress/nftables.conf
  grep -q "define SANDBOX_IFACE = \"docker0\"" /egress/nftables.conf
  echo "nftables ruleset parses"
'

step "compose files resolve with a rendered release"
mkdir -p "$WORK/secrets"
for file in platform.env llm.env postgres.env postgres-exporter.env smtp-password; do echo "X=1" >"$WORK/secrets/$file"; done
{
  echo "SKILLHUB_SECRETS_DIR=$WORK_HOST/secrets"
  echo "SKILLHUB_CONFIG_DIR=$WORK_HOST"
  echo "SKILLHUB_DEPLOY_DIR=$ROOT"
} >>"$WORK/control-plane-release.env"
docker compose --env-file "$WORK_HOST/control-plane-release.env" -f "$ROOT/infra/compose/control-plane.yml" --profile jobs config -q
echo "SKILLHUB_SECRETS_DIR=$WORK_HOST/secrets" >>"$WORK/gateway-release.env"
echo "X=1" >"$WORK/secrets/litellm.env"
docker compose --env-file "$WORK_HOST/gateway-release.env" -f "$ROOT/infra/compose/gateway.yml" config -q

step "Caddyfile"
docker run --rm -e SKILLHUB_DOMAIN=skillhub.example -e SKILLHUB_ACME_EMAIL=owner@skillhub.example \
  -v "$CP/Caddyfile:/etc/caddy/Caddyfile:ro" "$CADDY" caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile

step "Alertmanager configuration and probe targets rendered the way bootstrap renders them"
docker run --rm -e OWNER="$OWNER" --env-file "$WORK_HOST/control-plane-release.env" -v "$CP:/cp:ro" -v "$WORK_HOST:/work" "$UBUNTU" bash -euc '
  apt-get update -qq >/dev/null && apt-get install -y -qq --no-install-recommends gettext-base >/dev/null
  vars=$(sed -n "s/^envsubst \x27\(.*\)\x27 \\\\$/\1/p" /cp/bin/skillhub-bootstrap)
  [ -n "$vars" ] || { echo "could not read the envsubst variable list from skillhub-bootstrap"; exit 1; }
  envsubst "$vars" </cp/alertmanager.yml.tmpl >/work/alertmanager.yml
  if grep -n "\${" /work/alertmanager.yml; then echo "placeholders left unrendered"; exit 1; fi
  sh -euc "$(grep prometheus-targets /cp/bin/skillhub-bootstrap | sed "s#/etc/skillhub/#/work/#g")"
  cat /work/prometheus-targets/gateway.yml
  chown -R "$OWNER" /work
'
docker run --rm --entrypoint amtool -v "$WORK_HOST/alertmanager.yml:/etc/alertmanager/alertmanager.yml:ro" \
  "$ALERTMANAGER" check-config /etc/alertmanager/alertmanager.yml

step "Prometheus configuration and alert rules"
docker run --rm --entrypoint promtool \
  -v "$CP/prometheus.yml:/etc/prometheus/prometheus.yml:ro" \
  -v "$ROOT/infra/observability/alerts.yml:/etc/prometheus/alerts.yml:ro" \
  -v "$WORK_HOST/prometheus-targets:/etc/prometheus/targets:ro" \
  "$PROMETHEUS" check config /etc/prometheus/prometheus.yml

step "node scripts are POSIX sh"
docker run --rm -v "$ROOT:/repo:ro" -w /repo "$SHELLCHECK" -s sh -S warning \
  infra/deploy/common/install-docker infra/deploy/control-plane/bin/* infra/deploy/gateway/bin/* \
  infra/deploy/sandbox/bin/* infra/images/postgres/skillhub-backup infra/images/postgres/skillhub-restore-drill

echo
echo "deploy configuration: ok"
