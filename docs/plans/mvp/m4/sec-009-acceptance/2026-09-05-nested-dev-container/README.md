# SEC-009 Suite 1 partial re-run — nested developer container, 2026-09-05

## ⛔ Same caveat as 2026-08-26, unchanged

This is not the SEC-009 acceptance and cannot be. ADR-022 defines Suite 2's
subject as *the node about to join the pool*; this is Windows → Docker
Desktop's WSL2 VM → a privileged container → a gVisor sandbox. What is
measured is the boundary between a sandbox and a container deliberately
given every capability, not the boundary a production node enforces. See
[`tools/sec009/README.md`](../../../../../../tools/sec009/README.md) and the
[2026-08-26 evidence](../2026-08-26-nested-dev-container/README.md) for the
full reasoning; this directory does not repeat it.

## Why this run happened

Prompted by [`05` R-44](../../../../../plans/05-pending-rulings.md): before proposing a
base-image swap for the runtime-agent-sdk image (`node:22-bookworm-slim` →
`node:22-trixie-slim`, to get Python 3.13 natively instead of a hand-spliced
interpreter), check whether this exact gVisor build still holds its boundary
on this host — the T1/T2 procedure already existed and had not been re-run
since 2026-08-26.

**What this run does NOT answer**: T1 and T2 default `SMOKE_IMAGE` to a bare
`ubuntu:24.04` used only as the outer privileged container; every probe runs
via `runsc do sh -c ...`, never `docker run <skillhub image>`. So this is
real evidence that gVisor's boundary (this pinned release, this host kernel)
is intact — it is not evidence about whether the candidate trixie-based
image behaves differently inside that boundary. That narrower question needs
the actual candidate image run under a registered `runsc` runtime, which
this session did not do.

## T1: re-run, and it found a real bug in the probe itself

Both directions pass — [`sandboxed.txt`](T1/sandboxed.txt),
[`negative-control.txt`](T1/negative-control.txt) — but the first attempt at
the sandboxed direction did not:

```
no kernel taint                      FAIL    dmesg reports a taint
```

**The check was reading a symptom that also describes its absence.** The
kernel's own oops/WARN preamble is `CPU: N PID: M Comm: X Not tainted
<version> #N` — a *clean* kernel's own crash-trace header contains the
literal word "taint". This developer container shares the WSL2 kernel's
`dmesg` ring buffer across every run ever made against it (T2's own header
comment already says so, for a different reason), so the first `dd`-style
probe anywhere that ever provoked a kernel trace leaves a "Not tainted" line
sitting in the buffer for every later run to grep. Confirmed directly:

```
$ docker run --rm --privileged ubuntu:24.04 sh -c "dmesg | grep -i taint" | sort -u
[...] CPU: 1 PID: 68626 Comm: dd Not tainted 6.6.87.2-microsoft-standard-WSL2 #1
[...] (eight lines total, all "Not tainted", zero "Tainted:")
$ docker run --rm --privileged ubuntu:24.04 sh -c "cat /proc/sys/kernel/tainted"
0
```

2026-08-26's run of the same script passed this line — the buffer had not
yet accumulated one of these headers. It is not a fluke that only shows up
sometimes; it is a check that was always going to fail once *any* prior probe
anywhere on this host had ever provoked a kernel trace, which is exactly the
kind of latent, load-bearing-until-it-isn't bug this directory exists to
catch. Fourth instance of the same shape in this file's history (see the
PROBE NOTES in `t1-escape-attempts.sh`): a probe that reads prose about the
condition instead of the condition's own authoritative value.

**Fix**: read `/proc/sys/kernel/tainted` (a single integer, 0 = clean)
instead of grepping `dmesg` text. Landed in
[`tools/sec009/t1-escape-attempts.sh`](../../../../../../tools/sec009/t1-escape-attempts.sh).
Re-run after the fix: PASS. Negative control re-run after the fix: still
correctly reports 3 escapes and the marker on the node — the fix only
touches the taint line, and that line still reads PASS unsandboxed too
(taint is a property of the host kernel, not of whether this particular
process was sandboxed).

## T2: full ADR-022 spec (4×1800s), and the three official criteria all pass

[`fuzz-4x1800s.txt`](T2/fuzz-4x1800s.txt) — **6.7 million syscalls is wrong,
it's 4,924,693 calls** (floor 720,000), 31 distinct errnos, sandbox exited 0
after exactly 1800s.

| ADR-022's three T2 criteria | Result |
| --- | --- |
| Sentry does not crash | **PASS** — survived the full 1800s |
| Host kernel shows no oops | **PASS** — 0 new entries vs the pre-run baseline |
| No unexpected privileges gained | **PASS** — uid/gid unchanged in every worker that reported, and the three re-checked T1 invariants (`core_pattern`, `/dev/mem`, `docker.sock`) still held after 30 minutes of pressure |

**But the script's own stricter bookkeeping check failed**, and per ADR-022
§3 ("`unknown` counts as fail, no partial credit for the rest looking fine")
the run's overall exit code is 2, not 0:

```
every worker survived to report        FAIL    3/4 reported to the end
    last seen: 4:last-seen-at-294907calls/330slices
    exit status: worker EXIT 4 137
```

Worker 4's supervisor was itself killed (exit 137 = SIGKILL) partway through
— at 294,907 calls, well short of the ~1.2M/worker the other three logged.
Its `EXIT` line survived (unlike 2026-08-26's worker 2, whose final line was
lost to stdout collision), so this is a different failure mode from last
time, not a repeat.

### What killed worker 4 — investigated, and the leading hypothesis from last time does not hold this time

2026-08-26 left "a guest task collected under memory pressure" as one of two
open explanations for that run's lost worker. Checked whether the same
applies here, because if it did this would not be a new finding:

- **Memory was not the constraint.** The sampler's own peak reading was 8067
  MiB; `docker info` reports this VM has 25,164,980,224 bytes (≈23.4 GiB)
  total, and `free -h` immediately after the run showed 21 GiB still
  available. Nowhere close to exhaustion.
- **No cgroup OOM kill fired.** A real memcg OOM kill on this host prints a
  distinctive `dmesg` block (`invoked oom-killer` / `Memory cgroup out of
  memory: Killed process ...`) — confirmed present in this same ring buffer
  for unrelated `dd` processes from three days earlier (2026-09-02/03,
  killed at ~5 KB RSS inside a tightly memcg-constrained probe container,
  which is also what left the "Not tainted" lines T1's fix above deals
  with). **No such entry exists for today's run, and no entry names a
  `python3` process at all.** The script's own before/after `dmesg` oops
  comparison agrees (0 new entries).
- The `DENY` list in `_syscall_fuzz.py` already excludes `kill`/`tkill`/
  `tgkill` (62/200/234) from what a fuzzing child may call, specifically so
  a child cannot shoot its own supervisor — so the supervisor's death did
  not come from its own fuzzed children on the documented threat model.

**So this is genuinely unattributed, not "probably memory" restated with
more confidence.** The remaining candidate this session did not chase down
(it would need a `runsc do` run with sentry debug logging enabled, i.e.
another full 30-minute run) is gVisor's own sentry deciding to kill the task
in response to whatever the fuzzer's random `mmap`/`munmap`/`mprotect`
arguments did to its internal memory-mapping bookkeeping — which, if true,
would be the sentry defending itself correctly (kill the misbehaving task,
keep running), not a boundary failure: the "sentry did not crash" and "no
privileges gained" criteria both independently held for the *whole* 1800s,
including the window worker 4 died in.

**Recorded as `unknown` per ADR-022's own vocabulary, not waved through and
not asserted as a security finding.** The three official T2 criteria pass
on their own evidence; the fourth, stricter bookkeeping check is the reason
this run's script-level verdict stays FAIL.

## Net effect on R-44

This refreshes Suite 1's general gVisor-boundary evidence (T1 clean after a
real self-bug fix; T2's three official criteria clean; one unattributed
bookkeeping gap recorded honestly) on the exact `runsc` release
(`release-20260817.0`) that is also `infra/nodes/gvisor-baseline.txt`'s
value. It does **not** by itself clear R-44's `node:22-trixie-slim` swap for
push, because — as stated above — neither script touches the candidate
image. The SEC-009-full-suite blocker on 甲-5/R-43 (a real node, the
production nftables layer, the resident P-02 probe, etc.) is unchanged.
