#!/usr/bin/env bash
# ADR-022 part three, T1 — the generic half of the container-escape attempts.
#
# ============================ WHAT THIS IS NOT ==============================
# This is NOT the SEC-009 acceptance, and a green run here says nothing about
# the production boundary.
#
# T1's pass criterion has two halves: every attempt fails, AND the node shows no
# observable side effect. On a Windows developer machine the "node" is a
# privileged container inside Docker Desktop's WSL2 VM, deliberately given every
# capability, on a kernel that is not the one production runs. What is measured
# here is the boundary between a sandbox and that container. ADR-022 defines
# Suite 2's subject as *the node that is about to join the pool*, so this can
# never be it.
#
# The CVE-specific PoCs T1 names (CVE-2019-5736, CVE-2022-0492, CVE-2024-21626,
# CVE-2022-0847) are NOT here. They need real exploit code against a real host,
# and running someone's PoC binary inside a nested developer container proves
# nothing about a node it never touched. The deployment batch runs those against
# the node; this script covers the generic attempts, whose expected result is
# checkable anywhere.
#
# What it IS for: the procedure runs. Until 2026-08-23 T1 had no executable form
# at all, so the deployment batch would have started by writing it under time
# pressure on a machine nobody had debugged. The first run of this script found
# two bugs in itself and none in gVisor — see PROBE NOTES below. That is the
# whole return on writing it early.
# ============================================================================
#
# PROBE NOTES — two probes graded gVisor's correct behaviour as an escape:
#
#   mount(2)  gVisor implements mount and hands back *its own* procfs, so the
#             syscall succeeding is not an escape. The probe now reads the mount
#             back and only fails if it shows a kernel that is not gVisor's.
#   sockets   `runsc do` — the convenience mode these scripts use — creates
#             /tmp/runsc-do*/runsc-*.sock for its own control channel.
#             Production never uses `do`; sandboxd goes through the Docker
#             runtime. That socket is the harness measuring itself, so it is
#             excluded, narrowly and by path.
#   marker    (2026-08-23) 'plant a file on the node' graded the touch(1) from
#             INSIDE the sandbox, so it read ESCAPED every time: the sandbox has
#             its own writable view and creating a file in it is not an escape.
#             The same run's node-side observation said the node was clean, so
#             the script was contradicting itself in adjacent lines — and the
#             half that was wrong was the one that cannot see the answer. T1's
#             criterion is "no new file on the NODE filesystem", so the attempt
#             is now made from inside and graded only from outside. Third
#             instance of the same mistake in this file: a probe that cannot
#             observe the thing it is grading will still confidently grade it.
#   taint     (2026-09-05) 'no kernel taint' did `dmesg | grep -qi taint`, and
#             the kernel's own oops/WARN preamble is "CPU: N PID: M Comm: X
#             Not tainted <version> #N" -- a CLEAN kernel's crash-trace header
#             contains the literal word "taint". This developer container
#             shares the WSL2 kernel's dmesg ring buffer across every run ever
#             made against it (T2's own comment says so), so the first dd-style
#             probe anywhere that ever provoked a kernel trace leaves a "Not
#             tainted" line sitting in the buffer for every later run to grep.
#             2026-08-26's run passed; this run's dmesg had eight such lines,
#             all "Not tainted", zero "Tainted:" -- the buffer had filled in
#             between. Fixed by reading /proc/sys/kernel/tainted (a single
#             integer, 0 = clean) instead of pattern-matching prose that talks
#             about tainting whether or not it happened. Fourth instance of the
#             same shape: a probe read a symptom instead of the fact.
#
# Usage: tools/sec009/t1-escape-attempts.sh
# Exit:  0 every attempt refused, no side effect observed
#        1 setup failed — nothing was measured, which is not a pass
#        2 an attempt succeeded, or a check could not be made
#
# Vocabulary is ADR-022's, including `unknown` counting as fail: "we could not
# look" must never read the same as "we looked and it was fine".
set -uo pipefail

IMAGE="${SMOKE_IMAGE:-ubuntu:24.04}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"


PREPARE="$(cat "$HERE/_prepare-runsc.sh")"

# Negative control. With SEC009_NO_SANDBOX=1 the identical probes run in the
# privileged container with no sandbox around them, and the script inverts its
# expectation: at least one attempt MUST succeed. A probe suite that passes
# whether or not anything is sandboxing is measuring nothing, and every green
# tick it ever produced was worthless. Run it after touching any probe.
#
# It earns its keep immediately: unsandboxed, core_pattern is writable,
# /dev/mem is readable, and the mounted proc reports the host kernel — the last
# of which is exactly what the corrected mount probe was rewritten to catch.
NO_SANDBOX="${SEC009_NO_SANDBOX:-0}"

# -i, and it is not optional: `bash -s` reads the script from stdin, and without
# -i docker attaches nothing there. The container then runs an empty script and
# exits 0 — a green tick for a test that never ran, which is the exact failure
# mode this directory exists to avoid producing. It happened on the first try.
docker run --rm -i --privileged --cgroupns=private \
  -e NO_SANDBOX="$NO_SANDBOX" "$IMAGE" \
  bash -s <<INNER
$PREPARE

# Fixed and not random: a leftover from a previous run is itself a finding.
# Defined HERE rather than passed in with -e on purpose: a Git Bash host
# rewrites /tmp/... into C:/Users/.../Temp/... on its way through docker run,
# the plant below then fails, and the node-side row reports PASS having
# measured nothing. The negative control dies with it -- unsandboxed the same
# touch fails too, so that row had no input anywhere that could turn it red.
MARKER=/tmp/skillhub-sec009-t1-escaped

echo "runsc:         \$(runsc --version | head -1)"
echo "host kernel:   \$(uname -sr)"
echo

set +e
fail=0
escaped=0

# Each probe runs INSIDE the sandbox and prints one word we grade on. The probe
# reports what happened rather than exiting non-zero, so "the probe itself
# broke" stays distinguishable from "the attempt was refused".
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

# C-02/C-03: the classic host-command-execution primitive.
probe 'write /proc/sys/kernel/core_pattern' \
  'echo "|/bin/sh -c id" > /proc/sys/kernel/core_pattern 2>/dev/null && echo ESCAPED wrote core_pattern || echo REFUSED'

# C-04: see PROBE NOTES. Grades what got mounted, not whether mount returned 0.
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

# C-05: a module runs in the host kernel by definition.
probe 'load a kernel module' \
  'modprobe dummy 2>/dev/null && echo ESCAPED modprobe || { insmod /dev/null 2>/dev/null && echo ESCAPED insmod || echo REFUSED; }'

# C-06: physical memory is the whole host.
probe 'read /dev/mem' \
  'dd if=/dev/mem of=/dev/null bs=1 count=1 2>/dev/null && echo ESCAPED read /dev/mem || echo REFUSED'

# C-07: PID 1 inside the sandbox is the workload's own init, so seeing anything
# that belongs to the node is the finding.
probe 'see a host process' \
  'grep -qs docker /proc/1/cgroup 2>/dev/null && echo ESCAPED host cgroup visible || { [ "\$(cat /proc/1/comm 2>/dev/null)" = systemd ] && echo ESCAPED host init visible || echo REFUSED; }'

# C-16: a unix socket is a door that does not need the network. See PROBE NOTES
# for why exactly one path is excluded.
probe 'find any unix socket' \
  's=\$(find / -xdev -type s 2>/dev/null | grep -v "^/tmp/runsc-do" | head -3); [ -n "\$s" ] && echo ESCAPED sockets: \$s || echo REFUSED'

# P-01: the docker socket is the shortest escape there is.
probe 'reach the docker socket' \
  '[ -S /var/run/docker.sock ] && echo ESCAPED docker.sock present || echo REFUSED'

# The side-effect half. The attempt is made from inside and graded from OUTSIDE,
# and the split is the whole point: see PROBE NOTES entry 4.
#
# It honours NO_SANDBOX like the probes do, and that is not tidiness: run
# unconditionally through the sandbox, the node-side observation below would
# read PASS even in the negative control, i.e. it would be a check with no
# failing input anywhere in the suite. Unsandboxed it plants the file for real
# and that observation goes red, which is the only evidence it works.
if [ "\$NO_SANDBOX" = 1 ]; then
  touch "\$MARKER" >/dev/null 2>&1
else
  \$RUNSC sh -c "touch \$MARKER" >/dev/null 2>&1
fi

echo
echo 'Node-side observations (T1 requires these too):'

# The half a probe inside the sandbox cannot answer about itself.
if [ -e "\$MARKER" ]; then
  printf '  %-36s FAIL    %s exists on the node\n' 'no file planted on the node' "\$MARKER"; fail=2
else
  printf '  %-36s PASS\n' 'no file planted on the node'
fi

# A nonzero /proc/sys/kernel/tainted means the running kernel is tainted. See
# PROBE NOTES 'taint': dmesg text-matching "taint" also matches a clean
# kernel's own "Not tainted" crash-trace header, which sits in the shared
# ring buffer left over from any past run's dd(1)-style probes.
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
  # Inverted on purpose: with nothing sandboxing, the probes are supposed to
  # get through. If they do not, they were never measuring the sandbox.
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
