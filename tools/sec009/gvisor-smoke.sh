#!/usr/bin/env bash
# Smoke-checks that gVisor sandboxes inside a privileged Docker container.
# Not the full acceptance suite: nesting inside an already-privileged
# container tests that the procedure runs, not that a real boundary holds.
set -uo pipefail

IMAGE="${SMOKE_IMAGE:-ubuntu:24.04}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

PREPARE="$(cat "$HERE/_prepare-runsc.sh")"

# The same file node admission (gate A) compares a node against, so the probes
# exercise the build the fleet actually admits.
BASELINE="${SEC009_RUNSC_VERSION:-$(awk '!/^#/ && NF {print $1; exit}' "$HERE/../../infra/nodes/gvisor-baseline.txt")}"
if [ -z "$BASELINE" ] || [ "$BASELINE" = unset ]; then
  echo "infra/nodes/gvisor-baseline.txt is unset; there is no version to test" >&2
  exit 1
fi

# --privileged lets runsc create the namespaces and mounts its sentry needs;
# --cgroupns=private plus the leaf-cgroup move in the prepared body avoids
# "device or resource busy" without disabling resource limiting. -i: stdin.
docker run --rm -i --privileged --cgroupns=private \
  -e SEC009_RUNSC_VERSION="$BASELINE" "$IMAGE" \
  bash -s <<INNER
$PREPARE

echo "runsc:        \$(runsc --version | head -1)"
echo "host kernel:  \$(uname -sr)"

SANDBOX_KERNEL=\$(\$RUNSC uname -sr 2>&1 | tail -1)
echo "sandbox kernel: \${SANDBOX_KERNEL}"

case "\${SANDBOX_KERNEL}" in
  *gvisor*) ;;
  *) echo "NOT GVISOR: the workload ran on \${SANDBOX_KERNEL}"; exit 2 ;;
esac

IFACES=\$(\$RUNSC sh -c 'ls /sys/class/net 2>/dev/null | tr "\n" " "' 2>&1 | tail -1)
echo "sandbox interfaces with --network=none: [\${IFACES}]"

echo "SMOKE OK"
INNER
rc=$?
if [ $rc -ne 0 ]; then
  echo "gvisor smoke failed (exit $rc)" >&2
fi
exit $rc
