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

step() { printf '\n== %s\n' "$1"; }

cat >"$WORK/settings" <<'EOF'
SKILLHUB_DOMAIN=skillhub.example
SKILLHUB_ACME_EMAIL=owner@skillhub.example
SKILLHUB_PRIVATE_IP=10.0.0.2
SKILLHUB_ALERT_EMAIL=owner@skillhub.example
SKILLHUB_SMTP_SMARTHOST=smtp.example:587
SKILLHUB_SMTP_FROM=alerts@skillhub.example
SKILLHUB_SMTP_USERNAME=alerts@skillhub.example
EOF

step "user-data renders, and the renderer's own tests pass"
docker run --rm -v "$ROOT:/repo:ro" -v "$WORK_HOST:/work" -w /repo/tools/deploy "$UBUNTU" bash -euc '
  apt-get update -qq >/dev/null
  DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends cloud-init systemd >/dev/null
  python3 test_render.py
  python3 -c "
import render
env = render.release_env(\"control-plane\", \"0\" * 40, render.read_settings(open(\"/work/settings\").read()),
                         resolve=lambda repository, tag: \"sha256:\" + \"0\" * 64)
open(\"/work/release.env\", \"w\").write(env)
open(\"/work/user-data.yaml\", \"w\").write(render.cloud_init(env))
"
  cloud-init schema --config-file /work/user-data.yaml

  mkdir -p /opt/skillhub/infra/deploy/control-plane/bin
  for script in skillhub-preflight skillhub-alert; do
    printf "#!/bin/sh\n" >"/opt/skillhub/infra/deploy/control-plane/bin/$script"
    chmod +x "/opt/skillhub/infra/deploy/control-plane/bin/$script"
  done
  printf "#!/bin/sh\n" >/usr/bin/docker && chmod +x /usr/bin/docker
  install -m 0644 /repo/infra/deploy/control-plane/systemd/* /etc/systemd/system/
  cd /etc/systemd/system
  schedule=$(sed -n "s/^\([a-z]*\) \([a-z-]*\)$/skillhub-\1@\2.timer/p" /repo/infra/deploy/control-plane/maintenance-schedule)
  report=$(systemd-analyze verify skillhub.service skillhub-backup.timer skillhub-restore-drill.timer \
    skillhub-alert@skillhub-backup.service $schedule 2>&1 | grep -v "docker.service" || true)
  if [ -n "$report" ]; then printf "%s\n" "$report"; exit 1; fi
  echo "systemd units verify clean"
'

step "compose file resolves with a rendered release"
mkdir -p "$WORK/secrets"
for file in platform.env llm.env postgres.env postgres-exporter.env smtp-password; do echo "X=1" >"$WORK/secrets/$file"; done
{
  echo "SKILLHUB_SECRETS_DIR=$WORK_HOST/secrets"
  echo "SKILLHUB_CONFIG_DIR=$WORK_HOST"
  echo "SKILLHUB_DEPLOY_DIR=$ROOT"
} >>"$WORK/release.env"
docker compose --env-file "$WORK_HOST/release.env" -f "$ROOT/infra/compose/control-plane.yml" --profile jobs config -q

step "Caddyfile"
docker run --rm -e SKILLHUB_DOMAIN=skillhub.example -e SKILLHUB_ACME_EMAIL=owner@skillhub.example \
  -v "$CP/Caddyfile:/etc/caddy/Caddyfile:ro" "$CADDY" caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile

step "Alertmanager configuration rendered the way bootstrap renders it"
docker run --rm --env-file "$WORK_HOST/release.env" -v "$CP:/cp:ro" -v "$WORK_HOST:/work" "$UBUNTU" bash -euc '
  apt-get update -qq >/dev/null && apt-get install -y -qq --no-install-recommends gettext-base >/dev/null
  vars=$(sed -n "s/^envsubst \x27\(.*\)\x27 \\\\$/\1/p" /cp/bin/skillhub-bootstrap)
  [ -n "$vars" ] || { echo "could not read the envsubst variable list from skillhub-bootstrap"; exit 1; }
  envsubst "$vars" </cp/alertmanager.yml.tmpl >/work/alertmanager.yml
  if grep -n "\${" /work/alertmanager.yml; then echo "placeholders left unrendered"; exit 1; fi
'
docker run --rm --entrypoint amtool -v "$WORK_HOST/alertmanager.yml:/etc/alertmanager/alertmanager.yml:ro" \
  "$ALERTMANAGER" check-config /etc/alertmanager/alertmanager.yml

step "Prometheus configuration and alert rules"
docker run --rm --entrypoint promtool \
  -v "$CP/prometheus.yml:/etc/prometheus/prometheus.yml:ro" \
  -v "$ROOT/infra/observability/alerts.yml:/etc/prometheus/alerts.yml:ro" \
  "$PROMETHEUS" check config /etc/prometheus/prometheus.yml

step "node scripts are POSIX sh"
docker run --rm -v "$ROOT:/repo:ro" -w /repo "$SHELLCHECK" -s sh -S warning \
  infra/deploy/common/install-docker infra/deploy/control-plane/bin/* \
  infra/images/postgres/skillhub-backup infra/images/postgres/skillhub-restore-drill

echo
echo "deploy configuration: ok"
