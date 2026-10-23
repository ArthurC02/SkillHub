from __future__ import annotations

import locale
import os
import re
import shlex
import subprocess
import tempfile
import time
from collections import Counter
from collections.abc import Callable
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from .common import inside, sha256_of, whole_number
from .sources import EXCLUDED_DIRECTORIES

FORMAT = "domain-memory-counterfactual/v1"
SUITE_FORMAT = "domain-memory-counterfactual-suite/v1"
VERDICTS = ("killed", "survived", "inconclusive", "error")
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
MAX_FAILURE_LINES = 40
MAX_DETAIL_LINES = 3


GO_FAILED_TEST = re.compile(r"^\s*--- FAIL: (\S+)", re.MULTILINE)
GO_JSON_FAILED_TEST = re.compile(r'"Action":"fail"[^\n]*?"Test":"([^"]+)"')
PYTEST_FAILED_TEST = re.compile(r"^(?:FAILED|ERROR) (\S+::\S+)|^(\S+::\S+) (?:FAILED|ERROR)", re.MULTILINE)
UNITTEST_FAILED_TEST = re.compile(r"^(?:FAIL|ERROR): (\w+) \(([\w.]+)\)", re.MULTILINE)


GO_TEST_PASSED = re.compile(r'^\s*--- PASS: |"Action":"pass"[^\n]*"Test":"', re.MULTILINE)
GO_SKIPPED_TEST = re.compile(r'^\s*--- SKIP: |"Action":"skip"[^\n]*"Test":"', re.MULTILINE)
GO_SKIP_REASON = re.compile(r"^\s+\S+\.go:\d+: (.+)\n\s*--- SKIP: ", re.MULTILINE)
GO_NOTHING_RAN = re.compile(r"\[no tests to run\]|\[no test files\]")
UNITTEST_RAN = re.compile(r"^Ran (\d+) tests? in ", re.MULTILINE)
UNITTEST_SKIPPED = re.compile(r"skipped=(\d+)")
PYTEST_SUMMARY = re.compile(
    r"^=*\s*(\d+ (?:passed|failed|skipped|deselected|xfailed|xpassed|errors?|warnings?)(?:, \d+ \w+)*) in [\d.]+s",
    re.MULTILINE,
)


def is_go_test(argv: list[str]) -> bool:
    return bool(argv) and Path(argv[0]).stem == "go" and "test" in argv[1:]


def reports_tests(argv: list[str]) -> bool:
    return any(argument in ("-v", "-json") or argument.startswith(("-v=", "-test.v")) for argument in argv)


def no_test_passed(argv: list[str], output: str) -> str | None:
    if is_go_test(argv):
        if not reports_tests(argv):
            return "go test without -v or -json does not say which tests passed, so a skipped test reads as a pass; add -v"
        return None if GO_TEST_PASSED.search(output) else _no_go_test_passed(output)
    if GO_NOTHING_RAN.search(output) and not GO_TEST_PASSED.search(output):
        return "go test found no test to run"
    ran = UNITTEST_RAN.search(output)
    if ran:
        skipped = UNITTEST_SKIPPED.search(output)
        if int(ran[1]) == 0 or (skipped and int(skipped[1]) >= int(ran[1])):
            return "unittest ran no test that was not skipped"
    summaries = PYTEST_SUMMARY.findall(output)
    if summaries and "passed" not in summaries[-1]:
        return f"pytest passed no test: {summaries[-1]}"
    return None


def _no_go_test_passed(output: str) -> str:
    skipped = len(GO_SKIPPED_TEST.findall(output))
    if not skipped:
        return "no test passed: no test matched"
    reason = GO_SKIP_REASON.search(output)
    said = f", the first because {reason[1].strip()}" if reason else ""
    return f"no test passed: every test was skipped ({skipped}){said}"


def failing_tests(output: str) -> list[str]:
    names = [
        *(name.split("/", 1)[0] for name in GO_FAILED_TEST.findall(output)),
        *(name.split("/", 1)[0] for name in GO_JSON_FAILED_TEST.findall(output)),
        *(summary or verbose for summary, verbose in PYTEST_FAILED_TEST.findall(output)),
        *(where if where.endswith(f".{name}") else f"{where}.{name}" for name, where in UNITTEST_FAILED_TEST.findall(output)),
    ]
    return list(dict.fromkeys(names))


def failure_evidence(output: str) -> str:
    tail = output[-OUTPUT_TAIL:]
    failures = _failure_lines(output[:-OUTPUT_TAIL].splitlines())
    return "\n".join([*failures[:MAX_FAILURE_LINES], "...", tail]) if failures else tail


def _failure_lines(lines: list[str]) -> list[str]:
    kept: list[str] = []
    depth, details = None, 0
    for line in lines:
        indent = len(line) - len(line.lstrip())
        if FAILURE_LINE.match(line):
            kept.append(line)
            depth, details = indent, 0
        elif depth is not None and indent > depth and details < MAX_DETAIL_LINES:
            kept.append(line)
            details += 1
        else:
            depth = None
    return kept


class FailingBaselineError(ValueError):
    def __init__(self, output: str) -> None:
        with tempfile.NamedTemporaryFile(
            "w", encoding="utf-8", newline="", prefix="counterfactual-baseline-", suffix=".log", delete=False
        ) as log:
            log.write(output)
        super().__init__(
            "the tests do not pass before anything is broken, so a failure afterwards would prove nothing; "
            f"the whole output is in {log.name}"
        )
        self.details = {
            "failure_evidence": failure_evidence(output),
            "failing_tests": failing_tests(output),
            "log_path": log.name,
        }


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
    directory = inside(repo_root, cwd)
    if directory is None or not directory.is_dir():
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
    target = inside(repo_root, file)
    if target is None:
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


def _lines_holding(original: bytes, find: bytes) -> list[int]:
    lines = []
    start = original.find(find)
    while start != -1:
        lines.append(original.count(b"\n", 0, start) + 1)
        start = original.find(find, start + len(find))
    return list(dict.fromkeys(lines))


LISTED_LINES = 10


def _listed(lines: list[int]) -> str:
    shown = ", ".join(map(str, lines[:LISTED_LINES]))
    return f"{shown} and {len(lines) - LISTED_LINES} more" if len(lines) > LISTED_LINES else shown


def mutated(original: bytes, mutation: Mutation) -> bytes:
    find, replace = mutation.find.encode("utf-8"), mutation.replace.encode("utf-8")
    occurrences = original.count(find)
    if occurrences == 0:
        raise ValueError(f"the text to break must occur once in {mutation.file}, and it occurs 0 times")
    if occurrences > 1:
        raise ValueError(
            f"the text to break must occur once in {mutation.file}, and it occurs {occurrences} times, "
            f"at lines {_listed(_lines_holding(original, find))}; widen --find to text that occurs once"
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


def verdict_of(broken_run: TestRun, expect: str | None = None, nothing_ran: str | None = None) -> tuple[str, str]:
    if broken_run.timed_out:
        return "inconclusive", "the tests gave no result in time"
    if broken_run.passed:
        return ("inconclusive", f"the tests passed on the broken file, but {nothing_ran}; a run that judges nothing "
                "kills nothing") if nothing_ran else ("survived", (
            "the tests still pass with this change; either a test is missing, "
            "or the change does not break the protection (an equivalent mutation); if the test command "
            "selects only some tests, run it over the whole suite, where a kill names the protecting test "
            "in failing_tests"
        ))
    if BUILD_FAILURE.search(broken_run.output):
        return "inconclusive", (
            "the broken code did not build, so no test judged it, whatever --expect matches; "
            "break the protection so it still builds"
        )
    if expect is not None:
        if re.search(expect, broken_run.output):
            return "killed", f"the tests failed with output matching --expect {expect}"
        return "inconclusive", f"the failure does not match --expect {expect}"
    return "killed", "the tests failed once the protection was broken"


def _one_line(text: str) -> str:
    return " ".join(text.split())


VERDICT_WORDS = {"fail", "failed", "failure", "pass", "passed", "killed", "survived", "error"}


def _require_pattern(expect: str | None) -> None:
    if expect is None:
        return
    if expect.strip().casefold() in VERDICT_WORDS:
        raise ValueError(
            f"--expect is a regular expression the failing output must match, such as the assertion message, "
            f"not a verdict: {expect!r} would match almost any failure"
        )
    try:
        re.compile(expect)
    except re.error as error:
        raise ValueError(f"--expect is not a regular expression: {error}") from error


def _refuse_leftovers(root: Path) -> None:
    leftovers = leftover_sidecars(root)
    if leftovers:
        raise ValueError(
            f"an earlier run left {', '.join(leftovers)}; each holds the original content of the file "
            "beside it, so restore from it and remove it before proving anything"
        )


def _intact_run(directory: Path, proof: Proof, intact_runs: dict[tuple[str, Path, int], TestRun]) -> TestRun:
    key = (proof.test_command, directory, proof.timeout)
    if key not in intact_runs:
        intact_runs[key] = run_tests(directory, proof.test_command, proof.timeout)
    return intact_runs[key]


def counterfactual(repo_root: Path, mutation: Mutation, proof: Proof) -> dict[str, Any]:
    root = repo_root.resolve()
    _refuse_leftovers(root)
    return _prove(root, mutation, proof, {})


def counterfactual_suite(repo_root: Path, suite: Any) -> dict[str, Any]:
    root = repo_root.resolve()
    mutations = suite.get("mutations") if isinstance(suite, dict) else None
    if not isinstance(mutations, list) or not mutations:
        raise ValueError("--suite must be a JSON object whose mutations is a non-empty list")
    _refuse_leftovers(root)
    intact_runs: dict[tuple[str, Path, int], TestRun] = {}
    results = [_suite_result(root, entry, suite, index, intact_runs) for index, entry in enumerate(mutations, start=1)]
    counted = Counter(result["verdict"] for result in results)
    return {
        "format": SUITE_FORMAT,
        "results": results,
        "summary": {verdict: counted.get(verdict, 0) for verdict in VERDICTS},
    }


def _suite_result(
    root: Path, entry: Any, suite: dict[str, Any], index: int, intact_runs: dict[tuple[str, Path, int], TestRun]
) -> dict[str, Any]:
    identity = {
        key: entry.get(key) for key in ("obligation_id", "file", "test_command")
        if isinstance(entry, dict) and (isinstance(entry.get(key), str) or (key == "test_command" and isinstance(suite.get(key), str)))
    }
    if "test_command" in identity and identity["test_command"] is None:
        identity["test_command"] = suite["test_command"]
    try:
        mutation, proof = _suite_entry(entry, suite, index)
        result = _prove(root, mutation, proof, intact_runs)
        return {**result, "entry": index, **({"obligation_id": proof.obligation_id} if proof.obligation_id else {})}
    except (ValueError, OSError) as error:
        details = error.details if isinstance(error, FailingBaselineError) else {}
        return {"format": FORMAT, "entry": index, **identity, "verdict": "error", "reason": str(error), **details}


def _suite_entry(entry: Any, suite: dict[str, Any], index: int) -> tuple[Mutation, Proof]:
    if not isinstance(entry, dict):
        raise ValueError(f"mutations[{index}] must be an object with file, find and replace")
    values = {**{key: suite.get(key) for key in ("test_command", "cwd", "timeout", "expect")}, **entry}
    missing = [key for key in ("file", "find", "replace", "test_command") if not isinstance(values.get(key), str)]
    if missing:
        raise ValueError(f"mutations[{index}] needs {', '.join(missing)}; test_command may be set once for the suite")
    timeout = DEFAULT_TIMEOUT_SECONDS if values.get("timeout") is None else values["timeout"]
    if not whole_number(timeout) or timeout < 1:
        raise ValueError(f"mutations[{index}] timeout must be a positive number of seconds")
    for key in ("cwd", "expect", "obligation_id"):
        if values.get(key) is not None and not isinstance(values[key], str):
            raise ValueError(f"mutations[{index}] {key} must be text")
    return (
        Mutation(Path(values["file"]), values["find"], values["replace"]),
        Proof(values["test_command"], timeout, values.get("cwd"), values.get("expect"), values.get("obligation_id")),
    )


def _prove(
    root: Path, mutation: Mutation, proof: Proof, intact_runs: dict[tuple[str, Path, int], TestRun]
) -> dict[str, Any]:
    target = target_inside(root, mutation.file)
    directory = working_directory(root, proof.cwd)
    _require_pattern(proof.expect)
    original = target.read_bytes()
    broken = mutated(original, mutation)
    argv = shlex.split(proof.test_command)
    intact_run = _intact_run(directory, proof, intact_runs)
    if not intact_run.passed:
        raise FailingBaselineError(intact_run.output)
    nothing_ran = no_test_passed(argv, intact_run.output)
    if nothing_ran:
        raise ValueError(
            f"the tests pass before anything is broken, but {nothing_ran}; a run that judges nothing can kill nothing"
        )
    broken_run = run_broken(
        target, original, broken, lambda: run_tests(directory, proof.test_command, proof.timeout)
    )
    verdict, reason = verdict_of(broken_run, proof.expect, no_test_passed(argv, broken_run.output))
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
        "failing_tests": failing_tests(broken_run.output),
        "restoration_result": {"restored": restored, "content_sha256": sha256_of(original)},
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
