#!/usr/bin/env python3
import contextlib
import hashlib
import io
import sys
import tarfile
import tempfile
from pathlib import Path
from types import SimpleNamespace
from unittest import mock

import backfill_artifacts as backfill

FRESH = "11111111-1111-4111-8111-111111111111"
DONE = "22222222-2222-4222-8222-222222222222"
ORPHAN = "99999999-9999-4999-8999-999999999999"
WORKSPACE = "33333333-3333-4333-8333-333333333333"
CREATED = "2026-08-01 10:00:00+00"
RUNS = [
    {"id": FRESH, "workspace_id": WORKSPACE, "created_at": CREATED, "existing": 0},
    {"id": DONE, "workspace_id": WORKSPACE, "created_at": CREATED, "existing": 2},
]


def write_archive(root, run_id, attempt_id, members):
    folder = Path(root) / run_id / attempt_id
    folder.mkdir(parents=True)
    with tarfile.open(folder / "artifacts.tar", "w") as tar:
        folder_entry = tarfile.TarInfo("out")
        folder_entry.type = tarfile.DIRTYPE
        tar.addfile(folder_entry)
        for name, body in members:
            info = tarfile.TarInfo(name)
            info.size = len(body)
            tar.addfile(info, io.BytesIO(body))


def mirror(root):
    write_archive(root, FRESH, "a1", [("report.md", b"# report"), ("data.csv", b"a,b\n")])
    write_archive(root, DONE, "a2", [("x.txt", b"x")])
    write_archive(root, ORPHAN, "a9", [("y.txt", b"y")])
    (Path(root) / "stray").mkdir()


def run_main(argv, runs=RUNS, apply_result=None):
    out, err = io.StringIO(), io.StringIO()
    applied = []

    def fake_run(cmd, **kwargs):
        applied.append((cmd, kwargs.get("input")))
        return apply_result
    with mock.patch.object(backfill, "psql", lambda sql: runs), \
            mock.patch.object(backfill.subprocess, "run", fake_run), \
            mock.patch.object(sys, "argv", ["backfill_artifacts.py", *argv]), \
            contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
        code = backfill.main()
    return code, out.getvalue(), err.getvalue(), applied


def expected_sql():
    key = "run-artifacts/" + FRESH + "/a1/artifacts.tar"
    return "\n".join([
        backfill.statement(FRESH, WORKSPACE, CREATED, backfill.ArtifactManifestEntry(
            key, "report.md", 8, hashlib.sha256(b"# report").hexdigest())),
        backfill.statement(FRESH, WORKSPACE, CREATED, backfill.ArtifactManifestEntry(
            key, "data.csv", 4, hashlib.sha256(b"a,b\n").hexdigest())),
    ])


SUMMARY = (
    "archives read:       3\n"
    "runs to backfill:    1\n"
    "manifest rows:       2\n"
    "already backfilled:  1\n"
    "archive with no run: 1\n"
    "  ! no run row for " + ORPHAN + "; archive left alone\n"
    "files per run:       min 2 max 2 median 2\n"
)


def test_a_dry_run_prints_the_statements_and_counts_every_archive():
    with tempfile.TemporaryDirectory() as root:
        mirror(root)
        code, out, err, applied = run_main(["--tar-dir", root])
    assert code == 0
    assert out == expected_sql() + "\n", out
    assert err == SUMMARY, err
    assert applied == []


def test_apply_sends_the_same_statements_in_one_transaction_and_reports_rows():
    with tempfile.TemporaryDirectory() as root:
        mirror(root)
        done = SimpleNamespace(returncode=0, stdout="INSERT\n", stderr="")
        code, out, err, applied = run_main(["--tar-dir", root, "--apply"], apply_result=done)
    assert code == 0
    assert out == ""
    assert err == SUMMARY + "INSERT\napplied 2 rows\n", err
    assert len(applied) == 1 and applied[0][1] == expected_sql(), applied
    assert applied[0][0][-1] == "-1", applied[0][0]


def test_a_failed_apply_returns_psql_exit_code_without_claiming_rows():
    with tempfile.TemporaryDirectory() as root:
        mirror(root)
        failed = SimpleNamespace(returncode=3, stdout="", stderr="ERROR: boom\n")
        code, _, err, _ = run_main(["--tar-dir", root, "--apply"], apply_result=failed)
    assert code == 3
    assert err == SUMMARY + "ERROR: boom\n", err


def test_nothing_left_to_backfill_applies_nothing():
    with tempfile.TemporaryDirectory() as root:
        write_archive(root, DONE, "a2", [("x.txt", b"x")])
        code, out, err, applied = run_main(["--tar-dir", root, "--apply"])
    assert code == 0 and out == "" and applied == []
    assert err == (
        "archives read:       1\n"
        "runs to backfill:    0\n"
        "manifest rows:       0\n"
        "already backfilled:  1\n"
        "archive with no run: 0\n"
        "nothing to apply\n"
    ), err


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
