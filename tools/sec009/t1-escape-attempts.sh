#!/usr/bin/env bash
# Runs generic container-escape attempts against a gVisor sandbox. Each probe
# reports REFUSED, ESCAPED, or UNKNOWN; node-side checks run after, separately,
# since a probe inside the sandbox cannot see the node's own state.
set -uo pipefail

IMAGE="${SMOKE_IMAGE:-ubuntu:24.04}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"


PREPARE="$(cat "$HERE/_prepare-runsc.sh")"

# With SEC009_NO_SANDBOX=1 the identical probes run with no sandbox, and the
# script inverts its expectation: at least one attempt must then succeed, or
# the suite is not measuring anything.
NO_SANDBOX="${SEC009_NO_SANDBOX:-0}"

# -i is required: `bash -s` reads this script from stdin, and without -i
# docker attaches nothing there, so the container silently runs an empty
# script and exits 0.
docker run --rm -i --privileged --cgroupns=private \
  -e NO_SANDBOX="$NO_SANDBOX" "$IMAGE" \
  bash -s <<INNER
$PREPARE

# Defined here rather than passed in with -e: a Git Bash host rewrites
# /tmp/... paths on its way through docker run's -e, which would silently
# break the plant/check pair below.
MARKER=/tmp/skillhub-sec009-t1-escaped

echo "runsc:         \$(runsc --version | head -1)"
echo "host kernel:   \$(uname -sr)"
echo

set +e
fail=0
escaped=0

# Each probe prints one word rather than relying on its exit code, so a
# broken probe stays distinguishable from a refused attempt.
probe() {
  local name="\$1" script="\$2" out
  if [ "\$NO_SANDBOX" = 1 ]; then
    out=\$(sh -c "\$script" 2>&1 | tail -1)
  else
    out=\$(\$RUNSC sh -c "\$script" 2>&1 | tail -1)
  fi
  case "\$out" in
    REFUSED*) printf '  %-36s PASS    %s\n' "\$name" "\$out" ;;
    ESCAPED*) printf '  %-36s FAIL    %s\n' "\$name" "\$out"; fail=2; escaped=\$((escaped + 1)) ;;
    *)        printf '  %-36s UNKNOWN %s\n' "\$name" "\$out"; [ \$fail -eq 0 ] && fail=2 ;;
  esac
}

if [ "\$NO_SANDBOX" = 1 ]; then
  echo 'T1 generic attempts (NEGATIVE CONTROL: no sandbox, failures are expected):'
else
  echo 'T1 generic attempts (inside the sandbox):'
fi

probe 'write /proc/sys/kernel/core_pattern' \
  'echo "|/bin/sh -c id" > /proc/sys/kernel/core_pattern 2>/dev/null && echo ESCAPED wrote core_pattern || echo REFUSED'

# gVisor implements mount and hands back its own procfs, so a successful
# mount(2) is not itself an escape; this grades what got mounted instead.
probe 'mount(2) cannot reach the host' \
  'mkdir -p /mnt/x
   if mount -t proc none /mnt/x 2>/dev/null; then
     v=\$(cat /mnt/x/version 2>/dev/null)
     case "\$v" in
       *gvisor*) echo REFUSED mounted the sandbox own proc ;;
       "")       echo UNREADABLE mounted but version unreadable ;;
       *)        echo ESCAPED host proc: \$v ;;
     esac
   else
     echo REFUSED mount denied
   fi'

probe 'load a kernel module' \
  'modprobe dummy 2>/dev/null && echo ESCAPED modprobe || { insmod /dev/null 2>/dev/null && echo ESCAPED insmod || echo REFUSED; }'

probe 'read /dev/mem' \
  'dd if=/dev/mem of=/dev/null bs=1 count=1 2>/dev/null && echo ESCAPED read /dev/mem || echo REFUSED'

probe 'see a host process' \
  'grep -qs docker /proc/1/cgroup 2>/dev/null && echo ESCAPED host cgroup visible || { [ "\$(cat /proc/1/comm 2>/dev/null)" = systemd ] && echo ESCAPED host init visible || echo REFUSED; }'

# /tmp/runsc-do* is excluded: it's runsc's own control socket for the `do`
# convenience mode these scripts use, not a host-provided door.
probe 'find any unix socket' \
  's=\$(find / -xdev -type s 2>/dev/null | grep -v "^/tmp/runsc-do" | head -3); [ -n "\$s" ] && echo ESCAPED sockets: \$s || echo REFUSED'

probe 'reach the docker socket' \
  '[ -S /var/run/docker.sock ] && echo ESCAPED docker.sock present || echo REFUSED'

if [ "\$NO_SANDBOX" = 1 ]; then
  touch "\$MARKER" >/dev/null 2>&1
else
  \$RUNSC sh -c "touch \$MARKER" >/dev/null 2>&1
fi

echo
echo 'Node-side observations (T1 requires these too):'

if [ -e "\$MARKER" ]; then
  printf '  %-36s FAIL    %s exists on the node\n' 'no file planted on the node' "\$MARKER"; fail=2
else
  printf '  %-36s PASS\n' 'no file planted on the node'
fi

# Reads the integer flag directly rather than grepping dmesg text: a clean
# kernel's own crash-trace header contains the literal word "taint".
TAINTED="\$(cat /proc/sys/kernel/tainted 2>/dev/null)"
if [ -z "\$TAINTED" ]; then
  printf '  %-36s UNKNOWN /proc/sys/kernel/tainted not readable here\n' 'no kernel taint'; [ \$fail -eq 0 ] && fail=2
elif [ "\$TAINTED" != 0 ]; then
  printf '  %-36s FAIL    tainted=%s\n' 'no kernel taint' "\$TAINTED"; fail=2
else
  printf '  %-36s PASS\n' 'no kernel taint'
fi

echo
if [ "\$NO_SANDBOX" = 1 ]; then
  if [ \$escaped -gt 0 ]; then
    echo "NEGATIVE CONTROL OK: \$escaped attempt(s) succeeded unsandboxed, so these probes can see a failure"
    exit 0
  fi
  echo 'NEGATIVE CONTROL FAILED: nothing succeeded even unsandboxed — these probes measure nothing'
  exit 2
fi
if [ \$fail -eq 0 ]; then
  echo 'T1 GENERIC: every attempt refused, no side effect observed'
else
  echo 'T1 GENERIC: something was not refused, or could not be checked'
fi
exit \$fail
INNER
rc=$?
echo
if [ "$NO_SANDBOX" = 1 ]; then
  case $rc in
    0) echo "t1 negative control: the probes can see a failure" ;;
    *) echo "t1 negative control: the probes could not see a failure — they measure nothing" >&2 ;;
  esac
  exit $rc
fi
case $rc in
  0) echo "t1 generic: procedure executed, all refused — NOT the SEC-009 acceptance" ;;
  1) echo "t1 generic: setup failed, nothing was measured" >&2 ;;
  *) echo "t1 generic: an attempt succeeded or a check could not be made" >&2 ;;
esac
exit $rc
