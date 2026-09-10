#!/usr/bin/env bash
# Smoke-checks that gVisor sandboxes inside a privileged Docker container.
# Not the full acceptance suite: nesting inside an already-privileged
# container tests that the procedure runs, not that a real boundary holds.
set -uo pipefail

IMAGE="${SMOKE_IMAGE:-ubuntu:24.04}"

# --privileged lets runsc create the namespaces and mounts its sentry needs;
# --cgroupns=private plus the leaf-cgroup move below avoids "device or resource
# busy" on cgroup.subtree_control without disabling resource limiting.
docker run --rm --privileged --cgroupns=private "$IMAGE" bash -lc '
set -e
apt-get update -qq >/dev/null 2>&1
apt-get install -y -qq curl ca-certificates >/dev/null 2>&1

ARCH=$(uname -m)
URL=https://storage.googleapis.com/gvisor/releases/release/latest/${ARCH}
curl -fsSL -o /usr/local/bin/runsc "${URL}/runsc"
curl -fsSL -o /tmp/runsc.sha512 "${URL}/runsc.sha512"
( cd /usr/local/bin && sha512sum -c /tmp/runsc.sha512 --ignore-missing )
chmod 755 /usr/local/bin/runsc

# Empties the root cgroup so subtree_control becomes writable: cgroup v2's
# no-internal-process rule blocks delegation while processes sit directly in
# it, which is where a container's root cgroup starts.
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

IFACES=$($R sh -c "ls /sys/class/net 2>/dev/null | tr \"\n\" \" \"" 2>&1 | tail -1)
echo "sandbox interfaces with --network=none: [${IFACES}]"

echo "SMOKE OK"
'
rc=$?
if [ $rc -ne 0 ]; then
  echo "gvisor smoke failed (exit $rc)" >&2
fi
exit $rc
