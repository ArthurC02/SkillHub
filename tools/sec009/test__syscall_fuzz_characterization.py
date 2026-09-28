#!/usr/bin/env python3
import json
import sys
from unittest import mock

import _syscall_fuzz as sf


def test_random_arg_is_deterministic_for_a_given_seed():
    import random
    rng = random.Random(42)
    got = [sf.random_arg(rng) for _ in range(5)]
    assert got == [15, 18446744073699065856, 0, 28, 18446744073699065856], got


def test_report_shapes_a_worker_progress_snapshot():
    progress = sf.WorkerProgress(
        worker=1, calls=10, errnos={5, 6}, slices=3, crashes=0, killed=1,
        uid_before=100, gid_before=100, uid_after=100, gid_after=100,
    )
    assert sf.report(progress) == {
        "worker": 1, "calls": 10, "distinct_errnos": 2, "errnos": [5, 6],
        "slices": 3, "child_crashes": 0, "child_killed": 1,
        "uid_before": 100, "gid_before": 100, "uid_after": 100, "gid_after": 100,
    }


def test_emit_writes_one_json_line_with_the_progress_flag():
    written = []
    with mock.patch.object(sf.os, "write", lambda fd, data: written.append((fd, data))):
        sf.emit({"a": 1}, progress=True)
    fd, data = written[0]
    assert fd == 1
    assert json.loads(data.decode()) == {"a": 1, "progress": True}
    assert data.endswith(b"\n")


def test_emit_defaults_progress_to_false():
    written = []
    with mock.patch.object(sf.os, "write", lambda fd, data: written.append(data)):
        sf.emit({"a": 1})
    assert json.loads(written[0].decode())["progress"] is False


def test_emit_refuses_a_positional_progress_argument():
    try:
        sf.emit({}, True)
    except TypeError:
        pass
    else:
        raise AssertionError("progress must be keyword-only")


def test_wait_until_returns_pid_and_status_once_the_child_is_reaped():
    replies = iter([(0, None), (42, 0x1234)])
    with mock.patch.object(sf.os, "WNOHANG", 1, create=True), \
            mock.patch.object(sf.os, "waitpid", lambda pid, flags: next(replies)), \
            mock.patch.object(sf.time, "sleep", lambda s: None):
        result = sf._wait_until(42, sf.time.time() + 10)
    assert result == (42, 0x1234)


def test_wait_until_gives_up_once_the_deadline_has_passed():
    with mock.patch.object(sf.os, "WNOHANG", 1, create=True), \
            mock.patch.object(sf.os, "waitpid", lambda pid, flags: (0, None)), \
            mock.patch.object(sf.time, "sleep", lambda s: None):
        result = sf._wait_until(42, sf.time.time() - 1)
    assert result is None


def test_reap_returns_the_exit_status_when_the_child_is_reaped_in_time():
    with mock.patch.object(sf, "_wait_until", lambda pid, deadline: (pid, 99)):
        assert sf.reap(123, 1.0) == 99


def test_reap_kills_and_returns_none_when_the_child_never_reaps():
    deadlines = []

    def fake_wait_until(pid, deadline):
        deadlines.append(deadline)
        return None

    with mock.patch.object(sf, "_wait_until", fake_wait_until), \
            mock.patch.object(sf.signal, "SIGKILL", 9, create=True), \
            mock.patch.object(sf.os, "kill") as kill_mock:
        assert sf.reap(123, 1.0) is None
    kill_mock.assert_called_once_with(123, 9)
    assert len(deadlines) == 2


def test_reap_suppresses_a_kill_on_an_already_dead_child():
    with mock.patch.object(sf, "_wait_until", lambda pid, deadline: None), \
            mock.patch.object(sf.signal, "SIGKILL", 9, create=True), \
            mock.patch.object(sf.os, "kill", side_effect=OSError("no such process")):
        assert sf.reap(123, 1.0) is None


if __name__ == "__main__":
    failed = 0
    for name, fn in sorted(globals().items()):
        if name.startswith("test_") and callable(fn):
            try:
                fn()
                print(f"ok   {name}")
            except AssertionError as exc:
                failed += 1
                print(f"FAIL {name}: {exc}")
    sys.exit(1 if failed else 0)
