from __future__ import annotations

import subprocess
from collections import Counter
from collections.abc import Callable
from dataclasses import dataclass
from functools import cache
from itertools import combinations
from pathlib import Path, PurePosixPath
from typing import Any

from .sources import run_git

COMMIT_MARKER = "\x1e"


@dataclass(frozen=True)
class CouplingLimits:
    min_revisions: int = 5
    min_shared: int = 5
    min_degree: int = 30
    max_changeset: int = 30


def change_coupling(
    repo_root: Path, roots: list[str], depth: int, since: str | None, limits: CouplingLimits
) -> dict[str, Any]:
    commits = _commits(repo_root, since)
    kept = [files for files in commits if len(files) <= limits.max_changeset]
    module_for = _existing_module_finder(repo_root, roots, depth)
    revisions: Counter[str] = Counter()
    shared: Counter[tuple[str, str]] = Counter()
    for files in kept:
        modules = sorted({module for path in files if (module := module_for(path))})
        revisions.update(modules)
        shared.update(combinations(modules, 2))
    pairs = [
        {"modules": [first, second], "shared": count,
         "degree": round(200 * count / (revisions[first] + revisions[second]))}
        for (first, second), count in shared.items()
        if count >= limits.min_shared
        and min(revisions[first], revisions[second]) >= limits.min_revisions
    ]
    return {
        "commits": len(commits),
        "skipped_large_commits": len(commits) - len(kept),
        "modules": [{"module": module, "revisions": count} for module, count in sorted(revisions.items())],
        "pairs": sorted(
            (pair for pair in pairs if pair["degree"] >= limits.min_degree),
            key=lambda pair: (-pair["degree"], -pair["shared"], pair["modules"]),
        ),
    }


def _existing_module_finder(repo_root: Path, roots: list[str], depth: int) -> Callable[[str], str | None]:
    exists = cache(lambda module: (repo_root / module).is_dir())

    @cache
    def module_for(path: str) -> str | None:
        module = module_of(path, roots, depth)
        return module if module and exists(module) else None

    return module_for


def module_of(path: str, roots: list[str], depth: int) -> str | None:
    parts = PurePosixPath(path).parts
    for root in roots:
        owned = PurePosixPath(root.strip("/")).parts
        if parts[: len(owned)] == owned and len(parts) > len(owned) + 1:
            directories = parts[len(owned) : -1]
            return PurePosixPath(*owned, *directories[:depth]).as_posix()
    return None


def _commits(repo_root: Path, since: str | None) -> list[set[str]]:
    arguments = ["-c", "core.quotepath=false", "log", "--no-merges", "--no-renames", "--name-only",
                 "--format=%x1e"]
    if since:
        arguments.append(f"--since={since}")
    try:
        result = run_git(repo_root, *arguments)
    except (OSError, subprocess.TimeoutExpired) as error:
        raise ValueError(f"git could not read the history: {error}") from error
    if result.returncode != 0:
        raise ValueError(f"git could not read the history: {result.stderr.strip()}")
    return [
        {line for line in block.splitlines() if line.strip()}
        for block in result.stdout.split(COMMIT_MARKER)
        if block.strip()
    ]
