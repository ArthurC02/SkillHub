#!/usr/bin/env python3
"""Smoke-level syscall fuzzer for ADR-022 T2. Runs INSIDE the gVisor sandbox.

ADR-022 labels T2 "smoke-level, not research-level" inside its own pass
criterion, and that is a scope decision rather than an apology: research-grade
fuzzing of the Sentry is gVisor upstream's job and is not something this
platform can buy. What T2 buys is one narrow answer -- does the Sentry stay up,
and does the workload stay unprivileged, while a process throws garbage at it.

So this is deliberately dumb: random syscall numbers with random arguments
through libc's syscall(2). It is not trying to find a bug.

============================== THE SUPERVISOR ==============================
A process firing random syscalls destroys itself, and quickly. Measured here:
SIGSEGV within about five seconds, because sooner or later it hands munmap or
mprotect a pointer into its own address space. SIGKILL turns up too, from an
mmap large enough to be worth killing over.

That is not a finding about gVisor. It is the fuzzer being a fuzzer -- but it
means the naive shape of this script (one process, fuzz for N seconds) measures
about five seconds no matter what N says, and reports nothing at all, because
the process that died was also the one holding the counters. A 30-minute T2 run
would have quietly been a 5-second one.

So: a supervising parent that issues no random syscalls, and a stream of
short-lived children that do. A child fuzzes for one slice, writes its counters,
and exits; the parent adds them up and starts another until the wall clock says
stop. A child that dies mid-slice costs one slice of counts and is recorded as a
crash -- expected, reported as a number, and not by itself a T2 failure. T2 asks
whether the SENTRY survives, and the Sentry is what the parent is still talking
to when it starts the next child.
============================================================================

Usage: _syscall_fuzz.py <seconds> <worker-index>
Prints one JSON object, and nothing else, on stdout.
"""

import ctypes
import json
import os
import random
import signal
import sys
import time

MAX_SYSCALL = 452  # x86_64, roughly current upstream; overshooting is harmless.

# How long one child fuzzes before reporting back to its supervisor. Short enough that a segfault
# costs little (children die in seconds), long enough that fork/exec overhead
# does not dominate the run.
SLICE_SECONDS = 1.0

# How long past its own slice a child may take before the supervisor stops
# waiting. Generous against a one-second slice: this is the difference between
# "slow" and "never".
SLICE_GRACE_SECONDS = 5.0

# Calls that would end the fuzzer, wedge it, or destroy the thing it is
# measuring. Kept short on purpose: every number here is surface T2 does not
# cover, so each entry has to earn its place rather than be defensive.
DENY = {
    56, 57, 58, 435,        # clone/fork/vfork/clone3 -- fork bomb, and T3 owns that
    59, 322,                # execve/execveat -- replaces the process being measured
    60, 231,                # exit/exit_group
    62, 200, 234,           # kill/tkill/tgkill -- would shoot the supervisor
    15,                     # rt_sigreturn -- corrupts the stack, tells us nothing
    101,                    # ptrace -- T1 covers it as an escape attempt
    161,                    # chroot
    169,                    # reboot

    # The credential setters, and this exclusion is the interesting one. T2's
    # criterion is "no unexpected privileges gained", and a fuzzer that calls
    # setuid(2) with a random argument changes its own uid within the first
    # second -- an early run came back as uid 4086022094, which made the
    # criterion unmeasurable rather than failed. The surface given up is small:
    # a random setuid is a privilege *drop*, and everything after it would be a
    # weaker fuzz than the one we meant to run.
    105, 106, 113, 114, 116, 117, 119, 122, 123,
}


def random_arg(rng):
    """The values that actually find edges: 0, small, huge, pointer-shaped."""
    return rng.choice([
        0,
        rng.randint(1, 64),
        rng.randint(0, 2 ** 31),
        2 ** 64 - 1,
        rng.randint(0, 2 ** 48),          # plausible-looking userspace address
        0xFFFFFFFFFF600000,               # vsyscall page
    ])


def fuzz_slice(seconds, seed, result_path):
    """The expendable half. Runs in the child; nothing here is trusted to survive."""
    libc = ctypes.CDLL(None, use_errno=True)
    # Seeded, and the seed varies per slice: a fuzz run nobody can repeat is an
    # anecdote, and one that repeats the same slice forever is a single test.
    rng = random.Random(seed)

    # Random writes to fd 1 and 2 do land sometimes -- an early run produced a
    # 1.5 GB stderr file and lost a worker's result in the noise. Point them at
    # /dev/null; the counters travel by file.
    devnull = os.open(os.devnull, os.O_RDWR)
    os.dup2(devnull, 1)
    os.dup2(devnull, 2)

    # A blocking syscall with random arguments blocks forever. SIGALRM turns that
    # into EINTR, which is a result rather than a hang; without it one unlucky
    # poll() ends the slice and the counts read as "quiet".
    signal.signal(signal.SIGALRM, lambda *_: None)

    calls = 0
    errnos = set()
    deadline = time.time() + seconds
    while time.time() < deadline:
        nr = rng.randint(0, MAX_SYSCALL)
        if nr in DENY:
            continue
        args = [random_arg(rng) for _ in range(6)]
        ctypes.set_errno(0)
        signal.setitimer(signal.ITIMER_REAL, 0.25)
        try:
            libc.syscall(ctypes.c_long(nr), *[ctypes.c_long(a & (2 ** 64 - 1)) for a in args])
        except Exception:  # noqa: BLE001 -- ctypes can raise on wild arguments
            pass
        finally:
            signal.setitimer(signal.ITIMER_REAL, 0)
        errnos.add(ctypes.get_errno())
        calls += 1

    payload = json.dumps({
        "calls": calls,
        "errnos": sorted(errnos),
        "uid": os.getuid(),
        "gid": os.getgid(),
    })
    # os.open/os.write rather than open(): the io layer buffers, and this process
    # has spent the slice corrupting the state a flush would rely on.
    fd = os.open(result_path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    os.write(fd, payload.encode())
    os.close(fd)


def reap(pid, budget):
    """Collect one slice, and never wait longer than that slice's own budget.

    os.waitpid(pid, 0) is the obvious call, and it is the one that hung a
    4 x 1800s run at 48 minutes: every sentry thread asleep, runsc accumulating
    zero CPU ticks over ten seconds, and no worker having reported anything.

    A slice bounds itself with a 0.25s SIGALRM, and the fuzzer can reach the
    syscalls that arm it -- one random rt_sigprocmask(SIG_BLOCK, ...) or
    rt_sigaction(SIGALRM, SIG_IGN) and the timer stops arriving, so the next
    blocking syscall never returns and the supervisor waits on it forever. That
    is the same shape as the setuid case in DENY above: the fuzzer disabling the
    instrument that measures it.

    It is deliberately NOT fixed by adding those two syscalls to DENY. A
    blocklist only covers the hangs somebody predicted, and each entry is fuzz
    surface given up for good. SIGKILL cannot be blocked, ignored or handled, so
    the supervisor holds the deadline instead and the fuzzer keeps every syscall.

    Returns the wait status, or None if the child had to be killed -- a number
    the caller reports rather than hides, for the same reason child_crashes is
    reported: if it ever reaches the slice count, the workers are not fuzzing.
    """
    if _wait_until(pid, time.time() + budget) is not None:
        return _last_status
    try:
        os.kill(pid, signal.SIGKILL)
    except OSError:
        pass
    # SIGKILL is delivered by the sentry, so this normally returns at once. Bound
    # it anyway: a supervisor that hangs while cleaning up a hang has not fixed
    # anything. A zombie left behind costs one process slot until the run ends.
    _wait_until(pid, time.time() + SLICE_GRACE_SECONDS)
    return None


_last_status = 0


def _wait_until(pid, deadline):
    """Poll for pid until deadline. Returns pid on reap, None on timeout."""
    global _last_status
    while True:
        done, status = os.waitpid(pid, os.WNOHANG)
        if done == pid:
            _last_status = status
            return pid
        if time.time() >= deadline:
            return None
        time.sleep(0.01)


def report(worker, calls, errnos, slices, crashes, killed,
           uid_before, gid_before, uid_after, gid_after):
    """The one shape a worker reports in, mid-run or at the end."""
    return {
        "worker": worker,
        "calls": calls,
        "distinct_errnos": len(errnos),
        "errnos": sorted(errnos),
        "slices": slices,
        "child_crashes": crashes,
        "child_killed": killed,
        "uid_before": uid_before,
        "gid_before": gid_before,
        # From the last child that got far enough to report. The supervisor's own
        # uid is not the interesting one -- it never fuzzed.
        "uid_after": uid_after,
        "gid_after": gid_after,
    }


def emit(payload, progress=False):
    """Write one record as a single line, in one write.

    os.write of one pre-joined buffer rather than print(): four supervisors share
    this stdout, and print() issues the text and the newline separately, which is
    two chances for another worker's line to land in the middle of this one.
    """
    payload = dict(payload, progress=progress)
    os.write(1, (json.dumps(payload) + chr(10)).encode())


def self_check():
    """Prove reap() ends a child that has disabled its own timeout.

    The bug this closes is invisible in a passing run: reap is only exercised by
    a child that hangs, and a child hangs only when the fuzzer happens to roll
    the syscall that blocks SIGALRM. A 4 x 1800s run reached 48 minutes before
    anyone noticed, and the next regression would be just as quiet -- the code
    would simply wait forever again, and the run would look busy.

    So the hang is manufactured here rather than waited for: the child blocks
    SIGALRM exactly the way a random rt_sigprocmask does, then sleeps far past
    any budget. A reap that returns None inside the budget is the fix working;
    one that returns a status, or does not return, is not.
    """
    budget = 1.0
    pid = os.fork()
    if pid == 0:
        try:
            signal.pthread_sigmask(signal.SIG_BLOCK, {signal.SIGALRM})
            signal.setitimer(signal.ITIMER_REAL, 0.05)
            time.sleep(600)
        finally:
            os._exit(0)
    began = time.time()
    status = reap(pid, budget)
    took = time.time() - began
    ok = status is None and took < budget + SLICE_GRACE_SECONDS + 2
    print("  %-44s %s (returned %r after %.2fs, budget %.1fs)"
          % ("a child that blocked SIGALRM is killed", "ok" if ok else "FAIL",
             status, took, budget))

    # The other direction, so "reap kills everything" cannot pass as a fix.
    pid = os.fork()
    if pid == 0:
        os._exit(7)
    status = reap(pid, budget)
    exited7 = status is not None and os.WIFEXITED(status) and os.WEXITSTATUS(status) == 7
    print("  %-44s %s (status %r)"
          % ("a child that exits normally is reaped, not killed",
             "ok" if exited7 else "FAIL", status))
    return 0 if (ok and exited7) else 1


def main():
    seconds = float(sys.argv[1])
    worker = int(sys.argv[2])
    result_path = "/tmp/sec009-t2-worker-%d.json" % worker

    uid_before, gid_before = os.getuid(), os.getgid()
    calls = 0
    errnos = set()
    slices = crashes = killed = 0
    uid_after, gid_after = uid_before, gid_before

    # A supervisor that is killed leaves no final line, and the run then reports
    # "3/4 workers" with no way to say what happened to the fourth -- which is
    # `unknown`, and ADR-022 §3 counts unknown as fail.
    #
    # The progress line goes to STDOUT and not to a file, and that is the whole
    # point: `runsc do` gives the sandbox an overlay, so nothing written inside
    # /tmp is visible to the caller afterwards. A file-based checkpoint was
    # tried first and reported "no checkpoint" for every worker including the
    # three that finished -- an artifact that read exactly like a finding. The
    # one channel out of this sandbox is the stdout it was handed.
    emit(report(worker, 0, set(), 0, 0, 0, uid_before, gid_before, uid_after, gid_after),
         progress=True)

    deadline = time.time() + seconds
    while time.time() < deadline:
        try:
            os.unlink(result_path)  # a leftover would be counted twice
        except OSError:
            pass

        remaining = min(SLICE_SECONDS, deadline - time.time())
        if remaining <= 0:
            break

        pid = os.fork()
        if pid == 0:
            try:
                fuzz_slice(remaining, 100000 * worker + slices, result_path)
            finally:
                os._exit(0)

        status = reap(pid, remaining + SLICE_GRACE_SECONDS)
        slices += 1
        if status is None:
            killed += 1
        elif status != 0:
            crashes += 1

        try:
            with open(result_path) as fh:
                d = json.load(fh)
            calls += d["calls"]
            errnos.update(d["errnos"])
            uid_after, gid_after = d["uid"], d["gid"]
        except (OSError, ValueError, KeyError):
            pass  # the child died mid-slice; already counted as a crash

        # Roughly every 30 slices, so about twice a minute at a one-second slice.
        if slices % 30 == 0:
            emit(report(worker, calls, errnos, slices, crashes, killed,
                        uid_before, gid_before, uid_after, gid_after), progress=True)

    emit(report(worker, calls, errnos, slices, crashes, killed,
                uid_before, gid_before, uid_after, gid_after))


if __name__ == "__main__":
    if "--self-check" in sys.argv:
        sys.exit(self_check())
    main()
