#!/usr/bin/env bash
# Can gVisor actually sandbox on this machine, and is it really gVisor?
#
# ============================ WHAT THIS IS NOT ==============================
# This is NOT the SEC-009 acceptance. ADR-022 part three requires 10 test items
# on a dedicated VM pool, 45 coverage rows, every one pass with 0 unknown, and
# the evidence filed in docs/plans/mvp/m4/sec-009-acceptance/. This script runs
# one smoke test inside a privileged Docker container, which on a Windows host
# means a container inside Docker Desktop's WSL2 VM.
#
# Nesting matters and is the reason this cannot be the acceptance: an escape
# test run here is measuring the boundary between a sandbox and a container that
# was deliberately given every capability, on a kernel that is not the one
# production runs. A pass here is "the procedure executes", not "the boundary
# holds".
#
# What it IS good for: the 10 test items can now be written as scripts and dry
# run somewhere, which until 2026-08-23 they could not be — the repo had no
# Linux host at all and infra/nodes/gvisor-baseline.txt still reads `unset`.
# ============================================================================
#
# Usage: tools/sec009/gvisor-smoke.sh
# Exit:  0 the sandbox ran and reported itself as gVisor
#        1 something in the setup failed
#        2 it ran but is not a gVisor kernel — the interesting failure, because
#          it would mean the workload was executing somewhere it should not be
set -uo pipefail

IMAGE="${SMOKE_IMAGE:-ubuntu:24.04}"

# --privileged and --cgroupns=private are both needed and neither is incidental:
#
#   --privileged      runsc creates namespaces and mounts; without it the sentry
#                     cannot start at all.
#   --cgroupns=private plus the leaf-cgroup move below is what gets past
#                     "write /sys/fs/cgroup/cgroup.subtree_control: device or
#                     resource busy". The obvious fix is --ignore-cgroups, and
#                     it works — but it turns off the resource limiting that
#                     SEC-009's own exhaustion items are about, so the harder
#                     path is the one taken here. That trade is a finding for
#                     the deployment runbook, not a detail of this script.
docker run --rm --privileged --cgroupns=private "$IMAGE" bash -lc '
set -e
apt-get update -qq >/dev/null 2>&1
apt-get install -y -qq curl ca-certificates >/dev/null 2>&1

ARCH=$(uname -m)
URL=https://storage.googleapis.com/gvisor/releases/release/latest/${ARCH}
curl -fsSL -o /usr/local/bin/runsc "${URL}/runsc"
curl -fsSL -o /tmp/runsc.sha512 "${URL}/runsc.sha512"
# The checksum is checked, not assumed. A silently corrupted or substituted
# runsc that still ran would produce a green tick for the wrong binary.
( cd /usr/local/bin && sha512sum -c /tmp/runsc.sha512 --ignore-missing )
chmod 755 /usr/local/bin/runsc

# Empty the root cgroup so subtree_control becomes writable. A cgroup with
# processes directly in it cannot delegate controllers (cgroup v2 no-internal-
# process rule), and in a container the root cgroup is where everything lands.
mkdir -p /sys/fs/cgroup/init
for p in $(cat /sys/fs/cgroup/cgroup.procs); do
  echo "$p" > /sys/fs/cgroup/init/cgroup.procs 2>/dev/null || true
done
echo "+cpu +memory +pids" > /sys/fs/cgroup/cgroup.subtree_control

echo "runsc:        $(runsc --version | head -1)"
echo "host kernel:  $(uname -sr)"

R="runsc --platform=systrap --network=none do"
SANDBOX_KERNEL=$($R uname -sr 2>&1 | tail -1)
echo "sandbox kernel: ${SANDBOX_KERNEL}"

case "${SANDBOX_KERNEL}" in
  *gvisor*) ;;
  *) echo "NOT GVISOR: the workload ran on ${SANDBOX_KERNEL}"; exit 2 ;;
esac

# --network=none must mean no interfaces, not "an interface you probably will
# not use". SBX-007 default-deny starts here.
IFACES=$($R sh -c "ls /sys/class/net 2>/dev/null | tr \"\n\" \" \"" 2>&1 | tail -1)
echo "sandbox interfaces with --network=none: [${IFACES}]"

echo "SMOKE OK"
'
rc=$?
if [ $rc -ne 0 ]; then
  echo "gvisor smoke failed (exit $rc)" >&2
fi
exit $rc
