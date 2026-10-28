from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
import tempfile
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

ENGINE = Path(__file__).resolve().parents[1]
PLUGIN = ENGINE.parent
REPO = ENGINE.parents[3]
PLUGIN_ENTRY = PLUGIN / "scripts" / "registry_tools.py"
CASES = ENGINE / "parity" / "cases.json"
CONTRACT = ENGINE / "contract" / "cli.json"
BINARY = ENGINE / "target" / "release" / ("domain-memory.exe" if os.name == "nt" else "domain-memory")
PYTHON = "3.13"
UNKNOWN_OPTION = "--not-an-option-of-any-command"
RUN_SECONDS = 120
FIXED_GIT_DATE = "2026-01-01T00:00:00Z"
VOLATILE_FIELD = re.compile(rb'"(event_sha256|previous_event_sha256|head_sha256|heads|operation_id)": ("[^"]*"|null|\[[^\]]*\])')
VOLATILE_VALUE = re.compile(rb"(sha256:)?[0-9a-f]{64}|[0-9a-f]{32}")
VOLATILE_NOW = re.compile(rb"\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{6}Z|(?<=domain-registry-stage-)[0-9a-f]{32}|(?<=domain-registry-backup-)[0-9a-f]{32}")


def generated_cases() -> list[dict]:
    commands = json.loads(CONTRACT.read_text(encoding="utf-8"))["commands"]
    cases = [{"name": "no command", "args": []}, {"name": "root help", "args": ["--help"]}]
    for name in commands:
        cases.append({"name": f"{name} help", "args": [name, "-h"]})
        cases.append({"name": f"{name} unknown option", "args": [name, UNKNOWN_OPTION]})
    return cases


def written_fixture(root: Path, files: dict[str, str | dict[str, str] | None]) -> None:
    for relative, content in files.items():
        path = root / relative
        if content is None:
            path.mkdir(parents=True, exist_ok=True)
            continue
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(bytes.fromhex(content["hex"]) if isinstance(content, dict) else content.encode("utf-8"))


def committed(root: Path) -> None:
    dated = {**os.environ, "GIT_AUTHOR_DATE": FIXED_GIT_DATE, "GIT_COMMITTER_DATE": FIXED_GIT_DATE}

    def git(*args: str) -> None:
        done = subprocess.run(
            ["git", "-C", str(root), "-c", "core.autocrlf=false", "-c", "maintenance.auto=false", *args], capture_output=True, env=dated, check=False
        )
        if done.returncode != 0:
            raise RuntimeError(f"fixture git {' '.join(args)} failed in {root}: {(done.stdout + done.stderr).decode('utf-8', 'replace')}")

    git("init", "-q")
    git("add", "-A")
    git("-c", "user.name=parity", "-c", "user.email=parity@example.invalid", "commit", "-q", "-m", "fixture")


def expanded(args: list[str], fixture: Path) -> list[str]:
    return [arg.replace("{fixture}", str(fixture)).replace("{repo}", str(REPO)) for arg in args]


def run(argv: list[str], extra: dict[str, str], stdin: bytes = b"") -> dict:
    environment = {**os.environ, "COLUMNS": "80", "PYTHONIOENCODING": "utf-8", **extra}
    try:
        done = subprocess.run(
            argv, cwd=REPO, input=stdin, capture_output=True, env=environment, check=False, timeout=RUN_SECONDS
        )
    except subprocess.TimeoutExpired as expired:
        return {"exit": f"killed after {RUN_SECONDS} s", "stdout": expired.stdout or b"", "stderr": expired.stderr or b""}
    return {"exit": done.returncode, "stdout": done.stdout, "stderr": done.stderr}


def first_difference(left: bytes, right: bytes) -> str:
    left_lines, right_lines = left.decode("utf-8", "replace").splitlines(), right.decode("utf-8", "replace").splitlines()
    for number, (one, other) in enumerate(zip(left_lines, right_lines, strict=False), start=1):
        if one != other:
            return f"line {number}: python {one!r} / rust {other!r}"
    return f"python has {len(left_lines)} lines, rust has {len(right_lines)}"


def masked(data: bytes) -> bytes:
    def field(match: re.Match[bytes]) -> bytes:
        return match.group(0).replace(match.group(2), VOLATILE_VALUE.sub(b"~", match.group(2)))

    return VOLATILE_NOW.sub(b"~", VOLATILE_FIELD.sub(field, data))


def tree_of(root: Path) -> dict[str, bytes]:
    return {
        path.relative_to(root).as_posix(): masked(path.read_bytes())
        for path in sorted(root.rglob("*"))
        if path.is_file() and ".git" not in path.relative_to(root).parts
    }


def audit_problems(root: Path) -> list[str]:
    problems = []
    for events in sorted(root.rglob("audit/events.jsonl")):
        registry_root = events.parent.parent
        checked = run([sys.executable, str(PLUGIN_ENTRY), "verify-audit", "--registry-root", str(registry_root)], {})
        if checked["exit"] != 0:
            problems.append(f"audit written by rust under {registry_root.relative_to(root).as_posix()} fails verify-audit: {checked['stdout'].decode('utf-8', 'replace').strip()}")
    return problems


def prepared(case: dict, fixture: Path) -> None:
    written_fixture(fixture, case.get("files", {}))
    if case.get("git"):
        committed(fixture)


def ran(case: dict, fixture: Path, argv: list[str]) -> dict:
    stdin = case.get("stdin", "").replace("{fixture}", json.dumps(str(fixture))[1:-1])
    return run([*argv, *expanded(case["args"], fixture)], case.get("env", {}), stdin.encode("utf-8"))


def compared(case: dict, binary: Path) -> list[str]:
    with tempfile.TemporaryDirectory(ignore_cleanup_errors=True) as scratch:
        fixture = Path(scratch) / "f"
        fixture.mkdir()
        prepared(case, fixture)
        python = ran(case, fixture, [sys.executable, str(PLUGIN_ENTRY)])
        problems: list[str] = []
        if case.get("writes"):
            python_tree = tree_of(fixture)
            fixture.rename(Path(scratch) / "python")
            fixture.mkdir()
            prepared(case, fixture)
        rust = ran(case, fixture, [str(binary)])
        if case.get("writes"):
            for stream in ("stdout", "stderr"):
                python[stream], rust[stream] = masked(python[stream]), masked(rust[stream])
            rust_tree = tree_of(fixture)
            problems += [
                f"file {name}: {first_difference(python_tree.get(name, b''), rust_tree.get(name, b''))}"
                for name in sorted(set(python_tree) | set(rust_tree))
                if python_tree.get(name) != rust_tree.get(name)
            ]
            problems += audit_problems(fixture)
    problems = [
        f"{stream}: {first_difference(python[stream], rust[stream])}"
        for stream in ("stdout", "stderr")
        if python[stream] != rust[stream]
    ] + problems
    if python["exit"] != rust["exit"]:
        problems.append(f"exit: python {python['exit']} / rust {rust['exit']}")
    return problems


def main() -> int:
    parser = argparse.ArgumentParser(description="Run every parity case through the Python plugin and the Rust engine.")
    parser.add_argument("--binary", type=Path, default=BINARY)
    parser.add_argument("--only", help="run only the cases whose name contains this text")
    parser.add_argument("--jobs", type=int, default=os.cpu_count() or 1)
    options = parser.parse_args()
    if f"{sys.version_info.major}.{sys.version_info.minor}" != PYTHON:
        print(f"run the parity cases with Python {PYTHON}; this is {sys.version.split()[0]}", file=sys.stderr)
        return 2
    cases = generated_cases() + json.loads(CASES.read_text(encoding="utf-8"))
    chosen = [case for case in cases if not options.only or options.only in case["name"]]
    binary = options.binary.resolve()
    failed = 0
    with ThreadPoolExecutor(max_workers=options.jobs) as pool:
        results = pool.map(lambda case: compared(case, binary), chosen)
    for case, problems in zip(chosen, results, strict=True):
        if problems:
            failed += 1
            print(f"DIFF {case['name']}")
            for problem in problems:
                print(f"     {problem}")
    print(f"{len(chosen) - failed} of {len(chosen)} cases match")
    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main())
