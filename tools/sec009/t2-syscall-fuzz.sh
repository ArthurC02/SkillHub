#!/usr/bin/env bash
# ADR-022 part three, T2 — syscall smoke-level fuzz.
#
# ============================ WHAT THIS IS NOT ==============================
# Not the SEC-009 acceptance. Same nesting caveat as t1-escape-attempts.sh: on a
# developer machine the "node" is a privileged container inside Docker Desktop's
# WSL2 VM, on a kernel production does not run. ADR-022 defines Suite 2's subject
# as the node about to join the pool, so this can never be it.
#
# Not research-grade fuzzing either, and ADR-022 says so inside T2's own pass
# criterion. Research-grade Sentry fuzzing is gVisor upstream's responsibility
# and is not something this platform can buy. What this covers is the smoke
# question: under nonsense input, does the Sentry stay up and does the workload
# stay unprivileged.
# ============================================================================
#
# T2's criterion has three parts and this grades all three:
#   1. the Sentry does not crash        -> the sandbox survives the whole run
#   2. the host kernel shows no oops    -> node-side dmesg, before and after
#   3. no unexpected privileges gained  -> uid/gid unchanged, and the T1 escape
#                                          invariants still hold afterwards
#
# Part 3 re-runs three of t1's probes instead of restating them. A privilege that
# only appears after sustained syscall pressure is exactly what T2 is for, and a
# check that only compared uid would not see it.
#
# THE COUNT IS PART OF THE RESULT, not decoration. A fuzzer that issued no
# syscalls passes every check above, and so does one whose calls all failed the
# same way before reaching the Sentry. Both are green runs measuring nothing —
# the failure mode this directory exists to avoid producing. So the run fails
# unless it made a floor number of calls and saw a floor number of distinct
# errnos.
#
# Usage:
#   tools/sec009/t2-syscall-fuzz.sh                            # 60s x 4, procedure run
#   SEC009_FUZZ_SECONDS=1800 tools/sec009/t2-syscall-fuzz.sh   # ADR-022's 4 x 30min
#
# Exit: 0 pass   1 setup failed (nothing measured)   2 fail, or a check could not be made
#
# `unknown` counts as fail throughout: "we could not look" must never read the
# same as "we looked and it was fine".
set -uo pipefail

IMAGE="${SMOKE_IMAGE:-ubuntu:24.04}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# ADR-022 specifies 4 parallel x 30 minutes. The default is 60s so the procedure
# can be exercised without holding a machine for half an hour; the script says
# which one it ran and refuses to call a short run T2.
SECS="${SEC009_FUZZ_SECONDS:-60}"
WORKERS="${SEC009_FUZZ_WORKERS:-4}"
ADR_SECONDS=1800
ADR_WORKERS=4

PREPARE="$(cat "$HERE/_prepare-runsc.sh")"

# base64 rather than a nested heredoc: the fuzzer is Python full of quotes,
# backslashes and dollar signs, and every one of those is a way for it to arrive
# inside the container subtly different from what is on disk.
FUZZER_B64="$(base64 -w0 < "$HERE/_syscall_fuzz.py" 2>/dev/null || base64 < "$HERE/_syscall_fuzz.py" | tr -d '\n')"

echo "T2 syscall fuzz: ${WORKERS} worker(s) x ${SECS}s"
if [ "$SECS" -lt "$ADR_SECONDS" ] || [ "$WORKERS" -lt "$ADR_WORKERS" ]; then
  echo "  NOTE: below ADR-022's ${ADR_WORKERS} x ${ADR_SECONDS}s — a procedure run, NOT T2"
fi
echo

# <<INNER is unquoted so $PREPARE expands into it -- which also means BACKTICKS
# INSIDE THIS HEREDOC RUN ON THE HOST, comments included. Three explanatory
# comments below quoted shell names in backticks and the host dutifully ran
# `grep -c` and `runsc do` before the container ever started. Prose in here is
# executable; quote shell names with apostrophes.
# -i is not optional; see t1-escape-attempts.sh. `bash -s` reads the script from
# stdin, and without -i the container runs an empty one and exits 0.
docker run --rm -i --privileged --cgroupns=private \
  -e SECS="$SECS" -e WORKERS="$WORKERS" -e FUZZER_B64="$FUZZER_B64" \
  "$IMAGE" bash -s <<INNER
$PREPARE

# </dev/null: see _prepare-runsc.sh. Without it apt eats the rest of this script.
apt-get install -y -qq python3-minimal >/dev/null 2>&1 </dev/null || true
command -v python3 >/dev/null 2>&1 || { echo 'setup: no python3 in the image'; exit 1; }

echo "runsc:         \$(runsc --version | head -1)"
echo "host kernel:   \$(uname -sr)"
echo

echo "\$FUZZER_B64" | base64 -d > /tmp/fuzz.py || { echo 'setup: fuzzer did not decode'; exit 1; }
[ -s /tmp/fuzz.py ] || { echo 'setup: fuzzer arrived empty'; exit 1; }

# _prepare-runsc.sh leaves 'set -e' on, which is right for setup and wrong from
# here on: every line below is a check that is ALLOWED to fail, and its failure
# is the result. Left on, the first FAIL kills the script and the remaining
# checks are silently never made -- reported as one exit code that looks like a
# crash rather than as findings. 'grep -c' returning 1 on a zero count did
# exactly that on the first run.
set +e

fail=0

# ---- node-side baseline, taken before anything runs -----------------------
# dmesg is compared before/after rather than just grepped after: this developer
# container shares the WSL2 kernel ring buffer, which already has plenty in it
# that has nothing to do with us.
if dmesg >/dev/null 2>&1; then
  dmesg | wc -l > /tmp/dmesg.before
  dmesg | grep -ci 'oops\|BUG:\|general protection\|taint' > /tmp/oops.before || true
else
  echo 'UNKNOWN' > /tmp/dmesg.before
fi

# ---- the fuzz: one sandbox, N workers inside it ---------------------------
# One sandbox rather than N: T2 asks whether *a* Sentry survives concurrent
# pressure. N sandboxes would be N lightly-loaded Sentries, which is a different
# and easier question.
echo "fuzzing (one sandbox, \$WORKERS worker(s) inside it)..."
start=\$(date +%s)
# Peak memory, sampled from outside the sandbox. The first full-scale run lost a
# worker with empty stderr and no final line -- killed by a signal, and the only
# candidate anyone could name was memory. A number beats a shrug.
( peak=0
  while :; do
    cur=\$(awk '/^MemAvailable:/ {print \$2}' /proc/meminfo 2>/dev/null)
    used=\$(( \$(awk '/^MemTotal:/ {print \$2}' /proc/meminfo) - \$cur ))
    [ "\$used" -gt "\$peak" ] && { peak=\$used; echo "\$peak" > /tmp/mem.peak; }
    sleep 5
  done ) &
sampler=\$!
# Each worker is wrapped so its exit status reaches the transcript. A supervisor
# that dies otherwise leaves only an absence, and the classes call for opposite
# investigations: 137 is the sandbox running out of something, 139 is the
# supervisor corrupted by one of its own children.
\$RUNSC sh -c '
  for i in \$(seq 1 '"\$WORKERS"'); do
    ( python3 /tmp/fuzz.py '"\$SECS"' \$i; echo "EXIT \$i \$?" ) &
  done
  wait
' > /tmp/fuzz.out 2>/tmp/fuzz.err
rc=\$?
kill \$sampler 2>/dev/null
elapsed=\$(( \$(date +%s) - start ))
echo "  sandbox exited \$rc after \${elapsed}s"
echo

echo 'T2 checks:'

# 1. Sentry survived. A sandbox that died early is the finding, and "exited 0
#    immediately" is the shape that would otherwise read as a clean pass.
if [ \$rc -ne 0 ]; then
  printf '  %-38s FAIL    sandbox exited %s\n' 'sentry did not crash' "\$rc"
  head -3 /tmp/fuzz.err | sed 's/^/      /'
  fail=2
elif [ \$elapsed -lt \$(( \$SECS - 5 )) ]; then
  printf '  %-38s FAIL    returned after %ss, expected ~%ss\n' 'sentry did not crash' "\$elapsed" "\$SECS"
elif [ \$elapsed -gt \$(( \$SECS + 120 )) ]; then
  # Late is a finding too, and it did not used to be one. A 4x1800s run hung at
  # 48 minutes with every sentry thread asleep; had it eventually returned, this
  # block only looked for an EARLY return and would have printed PASS for a run
  # that took two and a half times as long. The margin covers the last slice and
  # the workers' final reports, which cost seconds, not minutes.
  printf '  %-38s FAIL    returned after %ss, expected ~%ss -- something blocked\n' \
    'the run kept its own clock' "\$elapsed" "\$SECS"
  fail=2
  fail=2
else
  printf '  %-38s PASS    survived %ss\n' 'sentry did not crash' "\$elapsed"
fi

# 2. The counts. This is the negative-control half: without it every check here
#    is satisfied by a fuzzer that never called anything.
calls=\$(python3 -c "
import json
n=e=bad=seen=sl=cr=kl=0
final={}; last={}
for line in open('/tmp/fuzz.out', errors='replace'):
    line=line.strip()
    if not line.startswith('{'): continue
    try: d=json.loads(line)
    except ValueError: continue        # a torn line, itself worth knowing about
    if 'calls' not in d or 'worker' not in d: continue
    (last if d.get('progress') else final)[d['worker']]=d
for d in final.values():
    seen+=1
    n+=d['calls']; e=max(e,d['distinct_errnos'])
    sl+=d.get('slices',0); cr+=d.get('child_crashes',0); kl+=d.get('child_killed',0)
    if d['uid_after']!=d['uid_before'] or d['gid_after']!=d['gid_before']: bad+=1
# A supervisor that dies leaves no final line. Its last progress line says how far
# it got, which is the difference between a finding and an 'unknown'. It has to be
# a line and not a file: 'runsc do' overlays the sandbox's filesystem, so nothing
# written to /tmp inside survives -- a file-based version reported 'no checkpoint'
# for every worker including the three that finished.
lost=[]
for w in sorted(set(last) - set(final)):
    d=last[w]
    lost.append('%d:last-seen-at-%dcalls/%dslices' % (w, d.get('calls',0), d.get('slices',0)))
    n+=d.get('calls',0); e=max(e,d.get('distinct_errnos',0))
    sl+=d.get('slices',0); cr+=d.get('child_crashes',0); kl+=d.get('child_killed',0)
for w in range(1, $WORKERS + 1):
    if w not in final and w not in last: lost.append('%d:never-reported' % w)
print(n,e,bad,seen,sl,cr,kl,','.join(sorted(lost)) or '-')
" 2>/dev/null)
set -- \$calls
total=\${1:-0}; distinct=\${2:-0}; privchange=\${3:-x}; seen=\${4:-0}
slices=\${5:-0}; crashes=\${6:-0}; killed=\${7:-0}; lost=\${8:--}
# No worker records is not "nobody changed identity"; it is not knowing.
[ "\$seen" -eq 0 ] && privchange=x

# 100 calls/second/worker, against a measured 600-900 in this sandbox. The floor
# catches "issued nothing" rather than characterising throughput: a slow machine
# must not turn into a red run, but a silent one must. Every worker has to have
# reported, too -- one worker's output going missing is how the first run of
# this script looked, and averaging over the survivors would have hidden it.
floor=\$(( 100 * \$SECS * \$WORKERS ))
# Two facts, two lines, and they used to share one. A run where the fuzzer
# issued eight times its floor but lost a worker printed
# "the fuzzer actually fuzzed FAIL 971822 calls (floor 120000)" -- an accurate
# verdict attached to a false statement. Both still set fail, so nothing is
# waved through; the transcript just stops asserting the wrong thing.
if [ "\$total" -ge "\$floor" ] && [ "\$distinct" -ge 5 ]; then
  printf '  %-38s PASS    %s calls (floor %s), %s errnos\n' \
    'the fuzzer actually fuzzed' "\$total" "\$floor" "\$distinct"
  # Not a verdict. The workload killing itself is what a syscall fuzzer does, and
  # it is the reason the supervisor exists at all. Printed so that a future run
  # reporting zero crashes cannot pass unnoticed: that would mean the children
  # had stopped fuzzing, not that they had become robust.
  printf '  %-38s         %s slice(s), %s ended in a signal, %s killed for hanging\n' \
    '  (workload self-destruction)' "\$slices" "\$crashes" "\$killed"
else
  printf '  %-38s FAIL    %s calls (floor %s), %s errnos\n' \
    'the fuzzer actually fuzzed' "\$total" "\$floor" "\$distinct"
  # What the workers actually said. Without this the failure is a number with no
  # explanation, and every debugging round costs another full run.
  echo '      --- worker output (first 5 lines) ---'
  head -5 /tmp/fuzz.out | sed 's/^/      /'
  echo '      --- worker stderr (first 5 lines) ---'
  head -5 /tmp/fuzz.err | sed 's/^/      /'
  fail=2
fi

# Whether every worker lived to file a final record. Separate from the line
# above because a run can fuzz hard and still lose a supervisor -- and because
# a supervisor never fuzzes, so its death is not the workload self-destruction
# that child_crashes counts. It is something ending a process that was only
# forking and waiting.
if [ "\$seen" -eq "\$WORKERS" ]; then
  printf '  %-38s PASS    %s/%s reported to the end\n' 'every worker survived to report' "\$seen" "\$WORKERS"
else
  printf '  %-38s FAIL    %s/%s reported to the end\n' 'every worker survived to report' "\$seen" "\$WORKERS"
  echo "      last seen: \$lost"
  grep '^EXIT ' /tmp/fuzz.out | sed 's/^/      exit status: worker /'
  echo "      sandbox-side memory peak: \$(( \$(cat /tmp/mem.peak 2>/dev/null || echo 0) / 1024 )) MiB"
  fail=2
fi

# 3a. No privileges gained, each worker comparing its own uid/gid before and
#     after. Comparing against 0 was wrong twice over: 'runsc do' does not run
#     the workload as root, and the fuzzer itself used to call setuid(2) with a
#     random argument -- the first run came back uid 4086022094, which made the
#     criterion unmeasurable rather than failed.
if [ "\$privchange" = 0 ]; then
  printf '  %-38s PASS    uid/gid unchanged in every worker\n' 'no privileges gained (uid/gid)'
elif [ "\$privchange" = x ]; then
  printf '  %-38s UNKNOWN worker output unparseable\n' 'no privileges gained (uid/gid)'
  [ \$fail -eq 0 ] && fail=2
else
  printf '  %-38s FAIL    %s worker(s) changed identity\n' 'no privileges gained (uid/gid)' "\$privchange"
  fail=2
fi

# 3b. The invariants T1 established, re-asserted after the pressure. Three of
#     t1's eight, chosen because each is a distinct escape primitive rather than
#     a variation: host command execution, physical memory, the docker socket.
post=0
\$RUNSC sh -c 'echo "|/bin/sh -c id" > /proc/sys/kernel/core_pattern 2>/dev/null && echo BROKEN || echo held' 2>/dev/null | tail -1 | grep -q held || post=1
\$RUNSC sh -c 'dd if=/dev/mem of=/dev/null bs=1 count=1 2>/dev/null && echo BROKEN || echo held' 2>/dev/null | tail -1 | grep -q held || post=1
\$RUNSC sh -c '[ -S /var/run/docker.sock ] && echo BROKEN || echo held' 2>/dev/null | tail -1 | grep -q held || post=1
if [ \$post -eq 0 ]; then
  printf '  %-38s PASS    core_pattern, /dev/mem, docker.sock\n' 'T1 invariants still hold after'
else
  printf '  %-38s FAIL    an invariant that held before does not now\n' 'T1 invariants still hold after'
  fail=2
fi

# 4. Node side.
if [ "\$(cat /tmp/dmesg.before)" = UNKNOWN ]; then
  printf '  %-38s UNKNOWN dmesg not readable here\n' 'no host kernel oops'
  [ \$fail -eq 0 ] && fail=2
else
  after=\$(dmesg | grep -ci 'oops\|BUG:\|general protection\|taint' || true)
  before=\$(cat /tmp/oops.before)
  if [ "\$after" -gt "\$before" ]; then
    printf '  %-38s FAIL    %s new kernel complaint(s)\n' 'no host kernel oops' "\$(( after - before ))"
    fail=2
  else
    printf '  %-38s PASS    no new entries (baseline %s)\n' 'no host kernel oops' "\$before"
  fi
fi

echo
if [ \$fail -eq 0 ]; then
  echo 'T2: sentry survived, workload stayed unprivileged, node unmarked'
else
  echo 'T2: something failed or could not be checked'
fi
exit \$fail
INNER
rc=$?

echo
case $rc in
  0) if [ "$SECS" -ge "$ADR_SECONDS" ] && [ "$WORKERS" -ge "$ADR_WORKERS" ]; then
       echo "t2: passed at ADR-022's duration — still NOT the SEC-009 acceptance (nested, wrong kernel)"
     else
       echo "t2: procedure executed and passed at ${WORKERS}x${SECS}s — NOT T2, which is ${ADR_WORKERS}x${ADR_SECONDS}s"
     fi ;;
  1) echo "t2: setup failed, nothing was measured" >&2 ;;
  *) echo "t2: a check failed or could not be made" >&2 ;;
esac
exit $rc
