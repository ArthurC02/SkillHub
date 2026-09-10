#!/usr/bin/env bash
# Syscall smoke-level fuzz against a gVisor sandbox: checks that the sentry
# survives, the workload stays unprivileged, and the host kernel logs no oops.
# A short local run is a procedure check, not the full-duration acceptance run.
set -uo pipefail

IMAGE="${SMOKE_IMAGE:-ubuntu:24.04}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

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

# <<INNER is unquoted so $PREPARE expands into it, which also means backticks
# anywhere in this heredoc (comments included) run on the host — quote shell
# names with apostrophes, not backticks, inside it.
docker run --rm -i --privileged --cgroupns=private \
  -e SECS="$SECS" -e WORKERS="$WORKERS" -e FUZZER_B64="$FUZZER_B64" \
  "$IMAGE" bash -s <<INNER
$PREPARE

apt-get install -y -qq python3-minimal >/dev/null 2>&1 </dev/null || true
command -v python3 >/dev/null 2>&1 || { echo 'setup: no python3 in the image'; exit 1; }

echo "runsc:         \$(runsc --version | head -1)"
echo "host kernel:   \$(uname -sr)"
echo

echo "\$FUZZER_B64" | base64 -d > /tmp/fuzz.py || { echo 'setup: fuzzer did not decode'; exit 1; }
[ -s /tmp/fuzz.py ] || { echo 'setup: fuzzer arrived empty'; exit 1; }

# Setup needs 'set -e', but every check below is allowed to fail (that failure
# is the result), so it must not abort the script.
set +e

fail=0

# dmesg is compared before/after rather than just grepped after: this
# container shares its kernel's ring buffer with whatever ran before it.
if dmesg >/dev/null 2>&1; then
  dmesg | wc -l > /tmp/dmesg.before
  dmesg | grep -ci 'oops\|BUG:\|general protection\|taint' > /tmp/oops.before || true
else
  echo 'UNKNOWN' > /tmp/dmesg.before
fi

# One sandbox with N workers inside it, not N sandboxes: this measures whether
# a single sentry survives concurrent pressure, not N lightly-loaded ones.
echo "fuzzing (one sandbox, \$WORKERS worker(s) inside it)..."
start=\$(date +%s)
( peak=0
  while :; do
    cur=\$(awk '/^MemAvailable:/ {print \$2}' /proc/meminfo 2>/dev/null)
    used=\$(( \$(awk '/^MemTotal:/ {print \$2}' /proc/meminfo) - \$cur ))
    [ "\$used" -gt "\$peak" ] && { peak=\$used; echo "\$peak" > /tmp/mem.peak; }
    sleep 5
  done ) &
sampler=\$!
# Each worker is wrapped so its exit status reaches the transcript rather than
# leaving only an absence if it dies.
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
  # Returning much later than requested is a finding too, not only returning
  # early: something blocked. The margin allows only for the last slice and
  # the workers' final reports.
  printf '  %-38s FAIL    returned after %ss, expected ~%ss -- something blocked\n' \
    'the run kept its own clock' "\$elapsed" "\$SECS"
  fail=2
  fail=2
else
  printf '  %-38s PASS    survived %ss\n' 'sentry did not crash' "\$elapsed"
fi

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
# A dead supervisor leaves no final line; its last progress line is what
# distinguishes a finding from an unknown.
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
[ "\$seen" -eq 0 ] && privchange=x

# Floor catches "issued nothing", not throughput: a slow machine must not turn
# a run red, but a silent one must.
floor=\$(( 100 * \$SECS * \$WORKERS ))
if [ "\$total" -ge "\$floor" ] && [ "\$distinct" -ge 5 ]; then
  printf '  %-38s PASS    %s calls (floor %s), %s errnos\n' \
    'the fuzzer actually fuzzed' "\$total" "\$floor" "\$distinct"
  printf '  %-38s         %s slice(s), %s ended in a signal, %s killed for hanging\n' \
    '  (workload self-destruction)' "\$slices" "\$crashes" "\$killed"
else
  printf '  %-38s FAIL    %s calls (floor %s), %s errnos\n' \
    'the fuzzer actually fuzzed' "\$total" "\$floor" "\$distinct"
  echo '      --- worker output (first 5 lines) ---'
  head -5 /tmp/fuzz.out | sed 's/^/      /'
  echo '      --- worker stderr (first 5 lines) ---'
  head -5 /tmp/fuzz.err | sed 's/^/      /'
  fail=2
fi

if [ "\$seen" -eq "\$WORKERS" ]; then
  printf '  %-38s PASS    %s/%s reported to the end\n' 'every worker survived to report' "\$seen" "\$WORKERS"
else
  printf '  %-38s FAIL    %s/%s reported to the end\n' 'every worker survived to report' "\$seen" "\$WORKERS"
  echo "      last seen: \$lost"
  grep '^EXIT ' /tmp/fuzz.out | sed 's/^/      exit status: worker /'
  echo "      sandbox-side memory peak: \$(( \$(cat /tmp/mem.peak 2>/dev/null || echo 0) / 1024 )) MiB"
  fail=2
fi

if [ "\$privchange" = 0 ]; then
  printf '  %-38s PASS    uid/gid unchanged in every worker\n' 'no privileges gained (uid/gid)'
elif [ "\$privchange" = x ]; then
  printf '  %-38s UNKNOWN worker output unparseable\n' 'no privileges gained (uid/gid)'
  [ \$fail -eq 0 ] && fail=2
else
  printf '  %-38s FAIL    %s worker(s) changed identity\n' 'no privileges gained (uid/gid)' "\$privchange"
  fail=2
fi

# Re-asserts three of t1's escape-attempt invariants after sustained pressure.
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
