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
EXPECTED = ENGINE / "parity" / "expected"
CONTRACT = ENGINE / "contract" / "cli.json"
BINARY = ENGINE / "target" / "release" / ("domain-memory.exe" if os.name == "nt" else "domain-memory")
PLATFORM = "windows" if os.name == "nt" else "posix"
PYTHON = "3.13"
UNKNOWN_OPTION = "--not-an-option-of-any-command"
RUN_SECONDS = 120
FIXED_GIT_DATE = "2026-01-01T00:00:00Z"
CREDENTIAL_SHAPED = re.compile(
    r"AKIA[0-9A-Z]{16}|gh[pousr]_[A-Za-z0-9]{36,}|sk-[A-Za-z0-9_-]{32,}|xox[baprs]-[A-Za-z0-9-]{10,}|-----BEGIN [A-Z ]*PRIVATE KEY-----|(?i:aws_secret_access_key\s*=\s*\S{20})"
)
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


def git_in(root: Path, *args: str) -> None:
    dated = {**os.environ, "GIT_AUTHOR_DATE": FIXED_GIT_DATE, "GIT_COMMITTER_DATE": FIXED_GIT_DATE}
    done = subprocess.run(
        ["git", "-C", str(root), "-c", "core.autocrlf=false", "-c", "maintenance.auto=false", *args], capture_output=True, env=dated, check=False
    )
    if done.returncode != 0:
        raise RuntimeError(f"git {' '.join(args)} failed in {root}: {(done.stdout + done.stderr).decode('utf-8', 'replace')}")


def committed(root: Path) -> None:
    git_in(root, "init", "-q")
    git_in(root, "add", "-A")
    git_in(root, "-c", "user.name=parity", "-c", "user.email=parity@example.invalid", "commit", "-q", "-m", "fixture")


def snapshot_at(pin: str, root: Path) -> Path:
    git_in(REPO, "clone", "-q", "--shared", "--no-checkout", str(REPO), str(root))
    git_in(root, "checkout", "-q", "--detach", pin)
    return root


def expanded(args: list[str], fixture: Path, repo: Path) -> list[str]:
    return [arg.replace("{fixture}", str(fixture)).replace("{repo}", str(repo)) for arg in args]


def run(argv: list[str], extra: dict[str, str], stdin: bytes, cwd: Path) -> dict:
    environment = {**os.environ, "COLUMNS": "80", "PYTHONIOENCODING": "utf-8", **extra}
    try:
        done = subprocess.run(
            argv, cwd=cwd, input=stdin, capture_output=True, env=environment, check=False, timeout=RUN_SECONDS
        )
    except subprocess.TimeoutExpired as expired:
        return {"exit": f"killed after {RUN_SECONDS} s", "stdout": expired.stdout or b"", "stderr": expired.stderr or b""}
    return {"exit": done.returncode, "stdout": done.stdout, "stderr": done.stderr}


def first_difference(left: bytes, right: bytes, names: tuple[str, str] = ("python", "rust")) -> str:
    left_lines, right_lines = left.decode("utf-8", "replace").splitlines(), right.decode("utf-8", "replace").splitlines()
    for number, (one, other) in enumerate(zip(left_lines, right_lines, strict=False), start=1):
        if one != other:
            return f"line {number}: {names[0]} {one!r} / {names[1]} {other!r}"
    return f"{names[0]} has {len(left_lines)} lines, {names[1]} has {len(right_lines)}"


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


def audit_problems(root: Path, verifier: list[str]) -> list[str]:
    problems = []
    for events in sorted(root.rglob("audit/events.jsonl")):
        registry_root = events.parent.parent
        checked = run([*verifier, "verify-audit", "--registry-root", str(registry_root)], {}, b"", REPO)
        if checked["exit"] != 0:
            problems.append(f"audit written under {registry_root.relative_to(root).as_posix()} fails verify-audit: {checked['stdout'].decode('utf-8', 'replace').strip()}")
    return problems


def prepared(case: dict, fixture: Path) -> None:
    written_fixture(fixture, case.get("files", {}))
    if case.get("git"):
        committed(fixture)


def ran(case: dict, fixture: Path, argv: list[str], repo: Path = REPO) -> dict:
    stdin = case.get("stdin", "").replace("{fixture}", json.dumps(str(fixture))[1:-1])
    return run([*argv, *expanded(case["args"], fixture, repo)], case.get("env", {}), stdin.encode("utf-8"), repo)


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
            problems += audit_problems(fixture, [sys.executable, str(PLUGIN_ENTRY)])
    problems = [
        f"{stream}: {first_difference(python[stream], rust[stream])}"
        for stream in ("stdout", "stderr")
        if python[stream] != rust[stream]
    ] + problems
    if python["exit"] != rust["exit"]:
        problems.append(f"exit: python {python['exit']} / rust {rust['exit']}")
    return problems


def spellings(path: Path) -> list[bytes]:
    found = set()
    for native in {str(path), str(path.resolve())}:
        found |= {native, native.replace("\\", "/"), native.replace("\\", "\\\\"), native.replace("\\", "\\\\\\\\")}
    return sorted((text.encode("utf-8") for text in found), key=len, reverse=True)


OS_ERROR = re.compile(rb"\[(?:Errno|WinError) \d+\] ")
OS_WORDING_ENDS = (b": ", b". ", b"\n", b"\\n", b'"', b"'")


def without_os_wording(data: bytes) -> bytes:
    kept, at = bytearray(), 0
    while found := OS_ERROR.search(data, at):
        kept += data[at : found.start()] + b"[OS error] ~"
        ends = [end for end in (data.find(mark, found.end()) for mark in OS_WORDING_ENDS) if end >= 0]
        at = min(ends) if ends else len(data)
    return bytes(kept + data[at:])


def tool_output_cut(data: bytes, prefixes: list[str]) -> bytes:
    for prefix in prefixes:
        found = data.find(prefix.encode("utf-8"))
        if found >= 0:
            data = data[: found + len(prefix.encode("utf-8"))] + b"~"
    return data


def server_info(version: str) -> bytes:
    return f'"name": "domain-memory", "version": "{version}"'.encode()


def placed(data: bytes, places: list[tuple[bytes, Path]]) -> bytes:
    for token, path in places:
        for spelling in spellings(path):
            data = data.replace(spelling, token)
    version = json.loads((PLUGIN / ".claude-plugin" / "plugin.json").read_text(encoding="utf-8"))["version"]
    return without_os_wording(data.replace(server_info(version), server_info("{version}")))


def stored(data: bytes) -> str | dict[str, str]:
    try:
        text = data.decode("utf-8")
    except UnicodeDecodeError:
        return {"hex": data.hex()}
    return {"hex": data.hex()} if CREDENTIAL_SHAPED.search(text) else text


def loaded(value: str | dict[str, str]) -> bytes:
    return bytes.fromhex(value["hex"]) if isinstance(value, dict) else value.encode("utf-8")


def observed(case: dict, argv: list[str], snapshot: Path) -> tuple[dict, list[str]]:
    with tempfile.TemporaryDirectory(ignore_cleanup_errors=True) as scratch:
        fixture = Path(scratch) / "f"
        fixture.mkdir()
        prepared(case, fixture)
        result = ran(case, fixture, argv, snapshot)
        places = [(b"{fixture}", fixture), (b"{scratch}", fixture.parent), (b"{repo}", snapshot), (b"{plugin}", PLUGIN)]
        streams = {stream: tool_output_cut(placed(result[stream], places), case.get("tool_output_after", [])) for stream in ("stdout", "stderr")}
        outcome: dict = {"exit": result["exit"]}
        problems: list[str] = []
        if case.get("writes"):
            streams = {stream: masked(data) for stream, data in streams.items()}
            outcome["files"] = {name: stored(placed(data, places)) for name, data in tree_of(fixture).items()}
            problems = audit_problems(fixture, argv)
        outcome |= {stream: stored(data) for stream, data in streams.items()}
        return outcome, problems


def expected_cases() -> tuple[str, dict[str, dict]]:
    pin, cases = "", {}
    for platform in ("posix", PLATFORM):
        recorded_file = EXPECTED / f"{platform}.json"
        if recorded_file.exists():
            recording = json.loads(recorded_file.read_text(encoding="utf-8"))
            pin, cases = recording["pin"], {**cases, **recording["cases"]}
    return pin, cases


def against_expected(case: dict, expected: dict | None, binary: Path, snapshot: Path) -> list[str]:
    if expected is None:
        return ["no expected output is recorded for this case"]
    actual, problems = observed(case, [str(binary)], snapshot)
    names = ("expected", "actual")
    for stream in ("stdout", "stderr"):
        if actual[stream] != expected[stream]:
            problems.insert(0, f"{stream}: {first_difference(loaded(expected[stream]), loaded(actual[stream]), names)}")
    wanted, written = expected.get("files", {}), actual.get("files", {})
    problems += [
        f"file {name}: {first_difference(loaded(wanted.get(name, '')), loaded(written.get(name, '')), names)}"
        for name in sorted(set(wanted) | set(written))
        if wanted.get(name) != written.get(name)
    ]
    if actual["exit"] != expected["exit"]:
        problems.append(f"exit: expected {expected['exit']} / actual {actual['exit']}")
    return problems


def recorded(chosen: list[dict], pin: str, jobs: int) -> int:
    with tempfile.TemporaryDirectory(ignore_cleanup_errors=True) as scratch:
        snapshot = snapshot_at(pin, Path(scratch) / "repo")
        with ThreadPoolExecutor(max_workers=jobs) as pool:
            results = list(pool.map(lambda case: observed(case, [sys.executable, str(PLUGIN_ENTRY)], snapshot), chosen))
    failed = [(case["name"], problems) for case, (_, problems) in zip(chosen, results, strict=True) if problems]
    for name, problems in failed:
        print(f"AUDIT {name}: {problems}")
    if failed:
        return 1
    cases = {case["name"]: outcome for case, (outcome, _) in zip(chosen, results, strict=True)}
    if PLATFORM == "windows" and (EXPECTED / "posix.json").exists():
        posix = json.loads((EXPECTED / "posix.json").read_text(encoding="utf-8"))["cases"]
        cases = {name: outcome for name, outcome in cases.items() if posix.get(name) != outcome}
    EXPECTED.mkdir(exist_ok=True)
    target = EXPECTED / f"{PLATFORM}.json"
    target.write_text(json.dumps({"pin": pin, "cases": cases}, ensure_ascii=False, indent=1, sort_keys=True) + "\n", encoding="utf-8", newline="\n")
    print(f"recorded {len(cases)} cases to {target}")
    return 0


def checked(chosen: list[dict], binary: Path, jobs: int) -> list[list[str]]:
    pin, expected = expected_cases()
    with tempfile.TemporaryDirectory(ignore_cleanup_errors=True) as scratch:
        snapshot = snapshot_at(pin, Path(scratch) / "repo")
        with ThreadPoolExecutor(max_workers=jobs) as pool:
            return list(pool.map(lambda case: against_expected(case, expected.get(case["name"]), binary, snapshot), chosen))


def main() -> int:
    parser = argparse.ArgumentParser(description="Run every parity case through the Python plugin and the Rust engine.")
    parser.add_argument("--binary", type=Path, default=BINARY)
    parser.add_argument("--only", help="run only the cases whose name contains this text")
    parser.add_argument("--jobs", type=int, default=os.cpu_count() or 1)
    parser.add_argument("--record", metavar="PIN", help="record the Python plugin's output against the repository at this commit")
    parser.add_argument("--expected", action="store_true", help="compare the binary with the recorded output instead of the Python plugin")
    options = parser.parse_args()
    if f"{sys.version_info.major}.{sys.version_info.minor}" != PYTHON:
        print(f"run the parity cases with Python {PYTHON}; this is {sys.version.split()[0]}", file=sys.stderr)
        return 2
    cases = generated_cases() + json.loads(CASES.read_text(encoding="utf-8"))
    chosen = [case for case in cases if not options.only or options.only in case["name"]]
    if options.record:
        return recorded(chosen, options.record, options.jobs)
    binary = options.binary.resolve()
    with ThreadPoolExecutor(max_workers=options.jobs) as pool:
        results = checked(chosen, binary, options.jobs) if options.expected else list(pool.map(lambda case: compared(case, binary), chosen))
    failed = 0
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
