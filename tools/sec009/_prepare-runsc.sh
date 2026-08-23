# Shared in-container preparation for the SEC-009 scripts. Not executable on its
# own: the callers cat it in front of their own body and hand the whole thing to
# bash inside a privileged container.
#
# One copy because the checksum check below is the security-relevant half. A
# second copy of it is a second place for it to quietly stop being run.
set -e

if ! command -v runsc >/dev/null 2>&1; then
  apt-get update -qq >/dev/null 2>&1
  apt-get install -y -qq curl ca-certificates >/dev/null 2>&1

  ARCH=$(uname -m)
  URL=https://storage.googleapis.com/gvisor/releases/release/latest/${ARCH}
  curl -fsSL -o /usr/local/bin/runsc "${URL}/runsc"
  curl -fsSL -o /tmp/runsc.sha512 "${URL}/runsc.sha512"
  # Checked, not assumed. A substituted runsc that still ran would produce a
  # green tick for the wrong binary, which is the failure mode this whole
  # directory exists to avoid producing.
  ( cd /usr/local/bin && sha512sum -c /tmp/runsc.sha512 --ignore-missing )
  chmod 755 /usr/local/bin/runsc
fi

# Empty the root cgroup so subtree_control becomes writable. A cgroup with
# processes directly in it cannot delegate controllers (cgroup v2's
# no-internal-process rule), and in a container the root cgroup is where
# everything lands.
#
# The obvious alternative is `runsc --ignore-cgroups`, and it works — but it
# turns off exactly the resource limiting SEC-009's T3 exhaustion items measure.
# A green run with the limits disabled is worse than a red one. This is a
# finding for the deployment runbook, not a detail of these scripts.
if [ ! -d /sys/fs/cgroup/init ]; then
  mkdir -p /sys/fs/cgroup/init
  for p in $(cat /sys/fs/cgroup/cgroup.procs); do
    echo "$p" > /sys/fs/cgroup/init/cgroup.procs 2>/dev/null || true
  done
  echo "+cpu +memory +pids" > /sys/fs/cgroup/cgroup.subtree_control
fi

RUNSC="runsc --platform=systrap --network=none do"
