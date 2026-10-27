from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
import tempfile
from pathlib import Path

ENGINE = Path(__file__).resolve().parents[1]
REPO = ENGINE.parents[1]
PLUGIN_ENTRY = REPO / ".claude" / "skills" / "domain-memory" / "scripts" / "registry_tools.py"
CASES = ENGINE / "parity" / "cases.json"
CONTRACT = ENGINE / "contract" / "cli.json"
BINARY = ENGINE / "target" / "release" / ("domain-memory.exe" if os.name == "nt" else "domain-memory")
PYTHON = "3.13"
UNKNOWN_OPTION = "--not-an-option-of-any-command"
RUN_SECONDS = 120


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
    def git(*args: str) -> None:
        subprocess.run(["git", "-C", str(root), "-c", "core.autocrlf=false", *args], check=True, capture_output=True)

    git("init", "-q")
    git("add", "-A")
    git("-c", "user.name=parity", "-c", "user.email=parity@example.invalid", "commit", "-q", "-m", "fixture")


def expanded(args: list[str], fixture: Path) -> list[str]:
    return [arg.replace("{fixture}", str(fixture)).replace("{repo}", str(REPO)) for arg in args]


def run(argv: list[str], extra: dict[str, str]) -> dict:
    environment = {**os.environ, "COLUMNS": "80", "PYTHONIOENCODING": "utf-8", **extra}
    try:
        done = subprocess.run(argv, cwd=REPO, capture_output=True, env=environment, check=False, timeout=RUN_SECONDS)
    except subprocess.TimeoutExpired as expired:
        return {"exit": f"killed after {RUN_SECONDS} s", "stdout": expired.stdout or b"", "stderr": expired.stderr or b""}
    return {"exit": done.returncode, "stdout": done.stdout, "stderr": done.stderr}


def first_difference(left: bytes, right: bytes) -> str:
    left_lines, right_lines = left.decode("utf-8", "replace").splitlines(), right.decode("utf-8", "replace").splitlines()
    for number, (one, other) in enumerate(zip(left_lines, right_lines, strict=False), start=1):
        if one != other:
            return f"line {number}: python {one!r} / rust {other!r}"
    return f"python has {len(left_lines)} lines, rust has {len(right_lines)}"


def compared(case: dict, binary: Path) -> list[str]:
    with tempfile.TemporaryDirectory() as scratch:
        fixture = Path(scratch)
        written_fixture(fixture, case.get("files", {}))
        if case.get("git"):
            committed(fixture)
        args = expanded(case["args"], fixture)
        python = run([sys.executable, str(PLUGIN_ENTRY), *args], case.get("env", {}))
        rust = run([str(binary), *args], case.get("env", {}))
    problems = [
        f"{stream}: {first_difference(python[stream], rust[stream])}"
        for stream in ("stdout", "stderr")
        if python[stream] != rust[stream]
    ]
    if python["exit"] != rust["exit"]:
        problems.append(f"exit: python {python['exit']} / rust {rust['exit']}")
    return problems


def main() -> int:
    parser = argparse.ArgumentParser(description="Run every parity case through the Python plugin and the Rust engine.")
    parser.add_argument("--binary", type=Path, default=BINARY)
    parser.add_argument("--only", help="run only the cases whose name contains this text")
    options = parser.parse_args()
    if f"{sys.version_info.major}.{sys.version_info.minor}" != PYTHON:
        print(f"run the parity cases with Python {PYTHON}; this is {sys.version.split()[0]}", file=sys.stderr)
        return 2
    cases = generated_cases() + json.loads(CASES.read_text(encoding="utf-8"))
    chosen = [case for case in cases if not options.only or options.only in case["name"]]
    failed = 0
    for case in chosen:
        problems = compared(case, options.binary)
        if problems:
            failed += 1
            print(f"DIFF {case['name']}")
            for problem in problems:
                print(f"     {problem}")
    print(f"{len(chosen) - failed} of {len(chosen)} cases match")
    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main())
