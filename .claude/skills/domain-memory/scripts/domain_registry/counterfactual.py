from __future__ import annotations

import locale
import os
import re
import shlex
import subprocess
import time
from collections.abc import Callable
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from .evidence import digest
from .sources import EXCLUDED_DIRECTORIES

FORMAT = "domain-memory-counterfactual/v1"
SIDECAR_SUFFIX = ".counterfactual-original"
OUTPUT_TAIL = 1200
DEFAULT_TIMEOUT_SECONDS = 600
MTIME_STEP_SECONDS = 2
RESTORE_DELAYS_SECONDS = (0.1, 0.25, 0.5, 1, 2, 4)
OBLIGATION_PLACEHOLDER = "<obligation-id>"
BUILD_FAILURE = re.compile(
    r"declared and not used|imported and not used|\[build failed\]|syntax error|SyntaxError|"
    r"IndentationError|could not compile|error TS\d+|COMPILATION ERROR|error: cannot find symbol"
)
FAILURE_LINE = re.compile(r"^\s*(--- FAIL|FAIL:|ERROR:|FAILED|panic:)")
MAX_FAILURE_LINES = 20


def failure_evidence(output: str) -> str:
    tail = output[-OUTPUT_TAIL:]
    failures = [line for line in output[:-OUTPUT_TAIL].splitlines() if FAILURE_LINE.match(line)]
    return "\n".join([*failures[:MAX_FAILURE_LINES], "...", tail]) if failures else tail


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


@dataclass(frozen=True)
class Proof:
    test_command: str
    timeout: int = DEFAULT_TIMEOUT_SECONDS
    cwd: str | None = None
    expect: str | None = None
    obligation_id: str | None = None


def decoded(output: bytes) -> str:
    try:
        return output.decode("utf-8")
    except UnicodeDecodeError:
        return output.decode(locale.getpreferredencoding(do_setlocale=False), "replace")


def working_directory(repo_root: Path, cwd: str | None) -> Path:
    if cwd is None:
        return repo_root
    root = repo_root.resolve()
    directory = (root / cwd).resolve()
    if not directory.is_relative_to(root) or not directory.is_dir():
        raise ValueError(f"--cwd must name a directory inside the repository: {cwd}")
    return directory


def run_tests(directory: Path, command: str, timeout: int) -> TestRun:
    try:
        completed = subprocess.run(
            shlex.split(command), cwd=directory, capture_output=True, timeout=timeout, check=False
        )
    except subprocess.TimeoutExpired:
        return TestRun(None, f"no result within {timeout} seconds")
    except OSError as error:
        raise ValueError(
            f"{command} could not start: {error}. The command runs as written, with no shell, "
            "so give the full invocation and run a pipeline or several commands from a script"
        ) from error
    return TestRun(completed.returncode, decoded(completed.stdout + completed.stderr))


def target_inside(repo_root: Path, file: Path) -> Path:
    target = (repo_root / file).resolve()
    if not target.is_relative_to(repo_root):
        raise ValueError(f"file is outside the repository: {file}")
    if not target.is_file():
        raise ValueError(f"file does not exist: {file}")
    return target


def sidecar_of(target: Path) -> Path:
    return target.with_name(target.name + SIDECAR_SUFFIX)


def leftover_sidecars(repo_root: Path) -> list[str]:
    found = []
    for directory, subdirectories, files in os.walk(repo_root):
        subdirectories[:] = [name for name in subdirectories if name not in EXCLUDED_DIRECTORIES]
        found.extend(
            (Path(directory) / name).relative_to(repo_root).as_posix()
            for name in files
            if name.endswith(SIDECAR_SUFFIX)
        )
    return sorted(found)


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


def restore(target: Path, original: bytes, sidecar: Path) -> None:
    for delay in (*RESTORE_DELAYS_SECONDS, None):
        try:
            write_as_newer(target, original, MTIME_STEP_SECONDS)
            break
        except OSError as error:
            if delay is None:
                raise ValueError(
                    f"could not restore {target}: {error}. It still holds the mutation; copy "
                    f"{sidecar} over it, then delete {sidecar.name}"
                ) from error
            time.sleep(delay)
    sidecar.unlink()


def run_broken(
    target: Path, original: bytes, broken: bytes, run_tests_now: Callable[[], TestRun]
) -> TestRun:
    sidecar = sidecar_of(target)
    sidecar.write_bytes(original)
    try:
        write_as_newer(target, broken, MTIME_STEP_SECONDS)
        return run_tests_now()
    finally:
        restore(target, original, sidecar)


def verdict_of(broken_run: TestRun, expect: str | None = None) -> tuple[str, str]:
    if broken_run.timed_out:
        return "inconclusive", "the tests gave no result in time"
    if broken_run.passed:
        return "survived", (
            "the tests still pass with this change; either a test is missing, "
            "or the change does not break the protection (an equivalent mutation)"
        )
    if expect is not None:
        if re.search(expect, broken_run.output):
            return "killed", f"the tests failed with output matching --expect {expect}"
        return "inconclusive", f"the failure does not match --expect {expect}"
    if BUILD_FAILURE.search(broken_run.output):
        return "inconclusive", (
            "the broken code did not build, so no test judged it; break the protection so it still builds, "
            "or pass --expect with the assertion the failure must show"
        )
    return "killed", "the tests failed once the protection was broken"


def _one_line(text: str) -> str:
    return " ".join(text.split())


def _require_pattern(expect: str | None) -> None:
    if expect is None:
        return
    try:
        re.compile(expect)
    except re.error as error:
        raise ValueError(f"--expect is not a regular expression: {error}") from error


def counterfactual(repo_root: Path, mutation: Mutation, proof: Proof) -> dict[str, Any]:
    root = repo_root.resolve()
    target = target_inside(root, mutation.file)
    directory = working_directory(root, proof.cwd)
    _require_pattern(proof.expect)
    leftovers = leftover_sidecars(root)
    if leftovers:
        raise ValueError(
            f"an earlier run left {', '.join(leftovers)}; each holds the original content of the file "
            "beside it, so restore from it and remove it before proving anything"
        )
    original = target.read_bytes()
    broken = mutated(original, mutation)
    intact_run = run_tests(directory, proof.test_command, proof.timeout)
    if not intact_run.passed:
        raise ValueError(
            "the tests do not pass before anything is broken, "
            "so a failure afterwards would prove nothing:\n" + intact_run.output[-OUTPUT_TAIL:]
        )
    broken_run = run_broken(
        target, original, broken, lambda: run_tests(directory, proof.test_command, proof.timeout)
    )
    verdict, reason = verdict_of(broken_run, proof.expect)
    restored = target.read_bytes() == original
    shown = target.relative_to(root).as_posix()
    line = line_of(original, mutation.find)
    result = {
        "format": FORMAT,
        "file": shown,
        "line": line,
        "mutated_protection": {"from": mutation.find, "to": mutation.replace},
        "test_command": proof.test_command,
        "verdict": verdict,
        "reason": reason,
        "failure_evidence": failure_evidence(broken_run.output),
        "restoration_result": {"restored": restored, "content_sha256": digest(original)},
    }
    if verdict == "killed" and restored:
        result["counterfactual_check"] = {
            "status": "passed",
            "obligation_id": proof.obligation_id or OBLIGATION_PLACEHOLDER,
            "mutated_protection": f"{shown}:{line}: {_one_line(mutation.find)} -> {_one_line(mutation.replace)}",
            "failure_evidence": failure_evidence(broken_run.output),
            "restored": True,
        }
    return result
