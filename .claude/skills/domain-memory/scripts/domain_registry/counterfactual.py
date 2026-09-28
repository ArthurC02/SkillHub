from __future__ import annotations

import hashlib
import os
import subprocess
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Any

FORMAT = "domain-memory-counterfactual/v1"
SIDECAR_SUFFIX = ".counterfactual-original"
OUTPUT_TAIL = 1200
DEFAULT_TIMEOUT_SECONDS = 600
MTIME_STEP_SECONDS = 2


@dataclass(frozen=True)
class Mutation:
    file: Path
    find: str
    replace: str


@dataclass(frozen=True)
class TestRun:
    exit_code: int | None
    output: str

    @property
    def passed(self) -> bool:
        return self.exit_code == 0

    @property
    def timed_out(self) -> bool:
        return self.exit_code is None


def digest(content: bytes) -> str:
    return "sha256:" + hashlib.sha256(content).hexdigest()


def run_tests(repo_root: Path, command: str, timeout: int) -> TestRun:
    try:
        completed = subprocess.run(
            command,
            cwd=repo_root,
            shell=True,
            capture_output=True,
            text=True,
            encoding="utf-8",
            errors="replace",
            timeout=timeout,
            check=False,
        )
    except subprocess.TimeoutExpired:
        return TestRun(None, f"no result within {timeout} seconds")
    return TestRun(
        completed.returncode, (completed.stdout + completed.stderr)[-OUTPUT_TAIL:]
    )


def target_inside(repo_root: Path, file: Path) -> Path:
    target = (repo_root / file).resolve()
    if not target.is_relative_to(repo_root):
        raise ValueError(f"file is outside the repository: {file}")
    if not target.is_file():
        raise ValueError(f"file does not exist: {file}")
    return target


def sidecar_of(target: Path) -> Path:
    return target.with_name(target.name + SIDECAR_SUFFIX)


def mutated(original: bytes, mutation: Mutation) -> bytes:
    find, replace = mutation.find.encode("utf-8"), mutation.replace.encode("utf-8")
    occurrences = original.count(find)
    if occurrences != 1:
        raise ValueError(
            f"the text to break must occur once in {mutation.file}, "
            f"and it occurs {occurrences} times"
        )
    if find == replace:
        raise ValueError("the replacement is the text it replaces")
    return original.replace(find, replace)


def line_of(original: bytes, find: str) -> int:
    return original[: original.index(find.encode("utf-8"))].count(b"\n") + 1


def write_as_newer(target: Path, content: bytes, seconds_ahead: int) -> None:
    # Build tools and bytecode caches compare whole-second timestamps and sizes,
    # so a same-size edit inside one second would run the previous content.
    newest = max(target.stat().st_mtime, time.time()) + seconds_ahead
    target.write_bytes(content)
    os.utime(target, (newest, newest))


def run_broken(
    target: Path, original: bytes, broken: bytes, run: tuple[Path, str, int]
) -> TestRun:
    sidecar = sidecar_of(target)
    sidecar.write_bytes(original)
    try:
        write_as_newer(target, broken, MTIME_STEP_SECONDS)
        return run_tests(*run)
    finally:
        write_as_newer(target, original, MTIME_STEP_SECONDS)
        sidecar.unlink()


def verdict_of(broken_run: TestRun) -> str:
    if broken_run.timed_out:
        return "inconclusive"
    return "survived" if broken_run.passed else "killed"


def counterfactual(
    repo_root: Path,
    mutation: Mutation,
    test_command: str,
    timeout: int = DEFAULT_TIMEOUT_SECONDS,
) -> dict[str, Any]:
    root = repo_root.resolve()
    target = target_inside(root, mutation.file)
    if sidecar_of(target).exists():
        raise ValueError(
            f"an earlier run left {sidecar_of(target).name} beside the file; "
            "it holds the original content, so restore from it and remove it"
        )
    original = target.read_bytes()
    broken = mutated(original, mutation)
    intact_run = run_tests(root, test_command, timeout)
    if not intact_run.passed:
        raise ValueError(
            "the tests do not pass before anything is broken, "
            "so a failure afterwards would prove nothing:\n" + intact_run.output
        )
    broken_run = run_broken(target, original, broken, (root, test_command, timeout))
    return {
        "format": FORMAT,
        "file": target.relative_to(root).as_posix(),
        "line": line_of(original, mutation.find),
        "mutated_protection": {"from": mutation.find, "to": mutation.replace},
        "test_command": test_command,
        "verdict": verdict_of(broken_run),
        "failing_evidence": broken_run.output,
        "restoration_result": {
            "restored": digest(target.read_bytes()) == digest(original),
            "content_sha256": digest(original),
        },
    }
