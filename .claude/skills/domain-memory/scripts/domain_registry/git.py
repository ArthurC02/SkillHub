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
OWN_PROCESS_GROUP: dict[str, Any] = (
    {"creationflags": subprocess.CREATE_NEW_PROCESS_GROUP} if os.name == "nt" else {"start_new_session": True}
)


def run_bounded(
    argv: list[str], *, stdin: str | bytes | None = None, errors: str | None = None
) -> subprocess.CompletedProcess[Any]:
    text = {"encoding": "utf-8", "errors": errors} if errors is not None else {}
    process = subprocess.Popen(
        argv, stdin=subprocess.PIPE if stdin is not None else None,
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, **text, **OWN_PROCESS_GROUP,
    )
    try:
        stdout, stderr = process.communicate(stdin, timeout=GIT_COMMAND_TIMEOUT_SECONDS)
    except subprocess.TimeoutExpired:
        _kill_process_tree(process)
        with contextlib.suppress(subprocess.TimeoutExpired):
            process.communicate(timeout=KILLED_TREE_DRAIN_SECONDS)
        raise
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
