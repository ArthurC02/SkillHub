from __future__ import annotations

import contextlib
import os
import signal
import subprocess
from collections.abc import Iterable
from pathlib import Path
from typing import Any

from .common import GIT_COMMAND_TIMEOUT_SECONDS

KILLED_TREE_DRAIN_SECONDS = 5
TIMEOUT_VARIABLE = "DOMAIN_MEMORY_GIT_TIMEOUT"
OWN_PROCESS_GROUP: dict[str, Any] = (
    {"creationflags": subprocess.CREATE_NEW_PROCESS_GROUP} if os.name == "nt" else {"start_new_session": True}
)


class GitDeadline(subprocess.TimeoutExpired):
    def __init__(self, cmd: list[str], timeout: float, *, rerun_is_safe: bool) -> None:
        super().__init__(cmd, timeout)
        self.rerun_is_safe = rerun_is_safe

    def __str__(self) -> str:
        safe = "; nothing was written, so it is safe to run again" if self.rerun_is_safe else ""
        return f"{super().__str__()}{safe}; set {TIMEOUT_VARIABLE} to a larger number of seconds to wait longer"


def deadline_seconds() -> int | float:
    given = os.environ.get(TIMEOUT_VARIABLE)
    if given is None:
        return GIT_COMMAND_TIMEOUT_SECONDS
    try:
        seconds = float(given)
    except ValueError:
        seconds = 0.0
    if not seconds > 0:
        raise ValueError(f"{TIMEOUT_VARIABLE} must be a positive number of seconds, not {given!r}")
    return int(seconds) if seconds.is_integer() else seconds


def rerun_is_safe(argv: list[str]) -> bool:
    return argv[0] == "git" and "config" not in argv


def run_bounded(
    argv: list[str], *, stdin: str | bytes | None = None, errors: str | None = None
) -> subprocess.CompletedProcess[Any]:
    seconds = deadline_seconds()
    text = {"encoding": "utf-8", "errors": errors} if errors is not None else {}
    process = subprocess.Popen(
        argv, stdin=subprocess.PIPE if stdin is not None else None,
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, **text, **OWN_PROCESS_GROUP,
    )
    try:
        stdout, stderr = process.communicate(stdin, timeout=seconds)
    except subprocess.TimeoutExpired as error:
        _kill_process_tree(process)
        with contextlib.suppress(subprocess.TimeoutExpired):
            process.communicate(timeout=KILLED_TREE_DRAIN_SECONDS)
        raise GitDeadline(argv, seconds, rerun_is_safe=rerun_is_safe(argv)) from error
    return subprocess.CompletedProcess(argv, process.returncode, stdout, stderr)


def _kill_process_tree(process: subprocess.Popen[Any]) -> None:
    if os.name == "nt":
        with contextlib.suppress(OSError, subprocess.SubprocessError):
            subprocess.run(
                ["taskkill", "/T", "/F", "/PID", str(process.pid)],
                capture_output=True, check=False, timeout=GIT_COMMAND_TIMEOUT_SECONDS,
            )
    else:
        with contextlib.suppress(ProcessLookupError):
            os.killpg(process.pid, signal.SIGKILL)
    process.kill()


def run_git(root: Path, *arguments: str, stdin: str | None = None) -> subprocess.CompletedProcess[str]:
    return run_bounded(["git", "-C", str(root), *arguments], stdin=stdin, errors="surrogateescape")


def run_git_bytes(root: Path, *arguments: str, stdin: bytes | None = None) -> subprocess.CompletedProcess[bytes]:
    return run_bounded(["git", "-C", str(root), *arguments], stdin=stdin)


def resolve_commit(repo_root: Path, ref: str) -> str | None:
    resolved = run_git(repo_root, "rev-parse", "--verify", "--quiet", f"{ref}^{{commit}}")
    return resolved.stdout.strip() if resolved.returncode == 0 else None


def toplevel(repo_root: Path) -> Path | None:
    result = run_git(repo_root, "rev-parse", "--show-toplevel")
    return Path(result.stdout.strip()).resolve() if result.returncode == 0 else None


def read_objects(repo_root: Path, names: Iterable[str]) -> dict[str, bytes]:
    asked = [name for name in dict.fromkeys(names) if "\n" not in name]
    if not asked:
        return {}
    request = "".join(f"{name}\n" for name in asked).encode("utf-8", "surrogateescape")
    result = run_git_bytes(repo_root, "cat-file", "--batch", stdin=request)
    if result.returncode != 0:
        raise subprocess.CalledProcessError(result.returncode, result.args, result.stdout, result.stderr)
    return _batch_objects(result.stdout, asked)


def _batch_objects(output: bytes, asked: list[str]) -> dict[str, bytes]:
    found, offset = {}, 0
    for name in asked:
        header_end = output.index(b"\n", offset)
        last_field = output[offset:header_end].rsplit(b" ", 1)[-1]
        offset = header_end + 1
        if last_field.isdigit():
            size = int(last_field)
            found[name] = output[offset:offset + size]
            offset += size + 1
    return found
