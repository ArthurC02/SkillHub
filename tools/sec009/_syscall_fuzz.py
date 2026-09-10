#!/usr/bin/env python3
"""Smoke-level syscall fuzzer that runs inside the gVisor sandbox: random
syscall numbers with random arguments through libc's syscall(2), supervised
by a parent that respawns short-lived children and sums their counters.

Usage: _syscall_fuzz.py <seconds> <worker-index>. Prints one JSON object."""

import ctypes
import json
import os
import random
import signal
import sys
import time

MAX_SYSCALL = 452

SLICE_SECONDS = 1.0

SLICE_GRACE_SECONDS = 5.0

DENY = {
    56, 57, 58, 435,
    59, 322,
    60, 231,
    62, 200, 234,
    15,
    101,
    161,
    169,

    105, 106, 113, 114, 116, 117, 119, 122, 123,
}


def random_arg(rng):
    """The values that actually find edges: 0, small, huge, pointer-shaped."""
    return rng.choice([
        0,
        rng.randint(1, 64),
        rng.randint(0, 2 ** 31),
        2 ** 64 - 1,
        rng.randint(0, 2 ** 48),
        0xFFFFFFFFFF600000,
    ])


def fuzz_slice(seconds, seed, result_path):
    """The expendable half. Runs in the child; nothing here is trusted to survive."""
    libc = ctypes.CDLL(None, use_errno=True)
    rng = random.Random(seed)

    devnull = os.open(os.devnull, os.O_RDWR)
    os.dup2(devnull, 1)
    os.dup2(devnull, 2)

    # A blocking syscall with random arguments would otherwise block forever;
    # SIGALRM turns that into EINTR, a result rather than a hang.
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
    # os.open/os.write rather than open(): the buffered io layer's state is not
    # trustworthy after a slice spent corrupting process state.
    fd = os.open(result_path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    os.write(fd, payload.encode())
    os.close(fd)


def reap(pid, budget):
    """Collect one slice, never waiting past its own budget: SIGKILLs the
    child if it hasn't been reaped by the deadline, since a fuzzed syscall
    can disable the SIGALRM that would otherwise bound a hung child."""
    if _wait_until(pid, time.time() + budget) is not None:
        return _last_status
    try:
        os.kill(pid, signal.SIGKILL)
    except OSError:
        pass
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
        "uid_after": uid_after,
        "gid_after": gid_after,
    }


def emit(payload, progress=False):
    """Write one record as a single line, in one os.write, so this worker's
    line can't interleave with another's on shared stdout."""
    payload = dict(payload, progress=progress)
    os.write(1, (json.dumps(payload) + chr(10)).encode())


def self_check():
    """Prove reap() kills a child that has disabled its own SIGALRM timeout,
    by manufacturing that hang directly rather than waiting for a random
    one to occur."""
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

    emit(report(worker, 0, set(), 0, 0, 0, uid_before, gid_before, uid_after, gid_after),
         progress=True)

    deadline = time.time() + seconds
    while time.time() < deadline:
        try:
            os.unlink(result_path)
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
            pass

        if slices % 30 == 0:
            emit(report(worker, calls, errnos, slices, crashes, killed,
                        uid_before, gid_before, uid_after, gid_after), progress=True)

    emit(report(worker, calls, errnos, slices, crashes, killed,
                uid_before, gid_before, uid_after, gid_after))


if __name__ == "__main__":
    if "--self-check" in sys.argv:
        sys.exit(self_check())
    main()
