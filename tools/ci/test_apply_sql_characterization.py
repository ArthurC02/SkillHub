#!/usr/bin/env python3
import json
import os
import subprocess
import sys
import tempfile

SCRIPT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "apply-sql.sh")

FAKE_PSQL = r"""
import json, os, sys
script = sys.stdin.read()
with open(os.environ["FAKE_PSQL_RECORD"], "w") as record:
    json.dump({"args": sys.argv[1:], "stdin": script}, record)
if os.environ.get("FAKE_PSQL_UNREACHABLE"):
    sys.exit(2)
for line in script.splitlines():
    if line.startswith("SELECT 'apply-sql: "):
        print(" " + line[len("SELECT '"):-len("';")] + " ")
    if "BREAKS" in line:
        sys.exit(3)
"""


def run(args, files=None, env=None):
    with tempfile.TemporaryDirectory() as work:
        paths = []
        for name, body in (files or {}).items():
            path = os.path.join(work, name).replace(os.sep, "/")
            with open(path, "w", newline="\n") as handle:
                handle.write(body)
            paths.append(path)
        record = os.path.join(work, "record.json")
        command = [arg for item in args for arg in (paths if item == "FILES" else [item])]
        command = [sys.executable if arg == "PYTHON" else arg for arg in command]
        result = subprocess.run(
            ["bash", SCRIPT, *command], capture_output=True, text=True,
            env={**os.environ, "FAKE_PSQL_RECORD": record, **(env or {})},
        )
        received = None
        if os.path.exists(record):
            with open(record) as handle:
                received = json.load(handle)
        return result, received, paths


def test_each_file_runs_in_its_own_transaction_in_order_through_one_session():
    result, received, paths = run(
        ["FILES", "--", "PYTHON", "-c", FAKE_PSQL, "postgres://db"],
        {"0001_a.sql": "CREATE TABLE a();\n", "0002_b.sql": "CREATE TABLE b();\n"},
    )
    assert result.returncode == 0, result.stderr
    assert received["args"] == ["postgres://db", "-q", "-v", "ON_ERROR_STOP=1"], received["args"]
    assert received["stdin"] == (
        "SELECT 'apply-sql: 0';\nBEGIN;\nCREATE TABLE a();\n\nCOMMIT;\n"
        "SELECT 'apply-sql: 1';\nBEGIN;\nCREATE TABLE b();\n\nCOMMIT;\n"
    ), received["stdin"]


def test_file_paths_never_reach_psql_where_a_backslash_could_read_as_a_command():
    result, received, paths = run(
        ["FILES", "--", "PYTHON", "-c", FAKE_PSQL], {"0001_a.sql": "SELECT 1;\n"},
    )
    assert result.returncode == 0, result.stderr
    assert paths[0] not in received["stdin"], received["stdin"]


def test_a_file_without_a_final_newline_still_ends_before_its_commit():
    result, received, paths = run(
        ["FILES", "--", "PYTHON", "-c", FAKE_PSQL], {"0001_a.sql": "SELECT 1;"},
    )
    assert result.returncode == 0, result.stderr
    assert received["stdin"].endswith("SELECT 1;\nCOMMIT;\n"), received["stdin"]


def test_a_failure_in_a_later_file_names_that_file_not_an_earlier_one():
    result, _, paths = run(
        ["FILES", "--", "PYTHON", "-c", FAKE_PSQL],
        {"0001_a.sql": "SELECT 1;\n", "0002_b.sql": "BREAKS;\n", "0003_c.sql": "SELECT 3;\n"},
    )
    assert result.returncode == 1, result.returncode
    assert result.stderr == f"::error file={paths[1]}::0002_b.sql failed to apply\n", result.stderr


def test_a_failure_in_the_first_file_names_the_first_file():
    result, _, paths = run(
        ["FILES", "--", "PYTHON", "-c", FAKE_PSQL],
        {"0001_a.sql": "BREAKS;\n", "0002_b.sql": "SELECT 2;\n"},
    )
    assert result.returncode == 1, result.returncode
    assert result.stderr == f"::error file={paths[0]}::0001_a.sql failed to apply\n", result.stderr


def test_a_session_that_fails_before_any_file_blames_no_file():
    result, _, _ = run(
        ["FILES", "--", "PYTHON", "-c", FAKE_PSQL],
        {"0001_a.sql": "SELECT 1;\n"}, env={"FAKE_PSQL_UNREACHABLE": "1"},
    )
    assert result.returncode == 1, result.returncode
    assert result.stderr == "::error file=unknown::unknown failed to apply\n", result.stderr


def test_missing_files_separator_or_command_is_a_usage_error_that_runs_nothing():
    cases = {
        "no files": ["--", "PYTHON", "-c", FAKE_PSQL],
        "no separator": ["FILES", "PYTHON", "-c", FAKE_PSQL],
        "no command": ["FILES", "--"],
    }
    for name, args in cases.items():
        result, received, _ = run(args, {"0001_a.sql": "SELECT 1;\n"})
        assert result.returncode == 2, (name, result.returncode)
        assert result.stderr.startswith("usage: apply-sql.sh"), (name, result.stderr)
        assert received is None, (name, received)


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
