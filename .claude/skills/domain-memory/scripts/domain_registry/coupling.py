from __future__ import annotations

import re
import subprocess
from collections import Counter, defaultdict
from collections.abc import Callable
from dataclasses import dataclass, field
from functools import cache
from itertools import combinations
from pathlib import Path, PurePosixPath
from typing import Any

from .sources import run_git

COMMIT_MARKER = "\x1e"
SUBJECTS_SHOWN = 10
SKIPPED_SHOWN = 20


@dataclass(frozen=True)
class CouplingLimits:
    min_revisions: int = 5
    min_shared: int = 5
    min_degree: int = 30
    max_changeset: int = 30
    max_modules: int | None = None


@dataclass(frozen=True)
class CouplingReading:
    since: str | None = None
    exclude_subjects: tuple[str, ...] = ()
    context_paths: dict[str, str] = field(default_factory=dict)
    collaborations: dict[frozenset[str], list[str]] = field(default_factory=dict)


@dataclass(frozen=True)
class Commit:
    sha: str
    subject: str
    files: frozenset[str]


def change_coupling(
    repo_root: Path,
    roots: list[str],
    depth: int,
    limits: CouplingLimits,
    reading: CouplingReading | None = None,
) -> dict[str, Any]:
    reading = reading or CouplingReading()
    excluded = _subject_filter(reading.exclude_subjects)
    commits = _commits(repo_root, reading.since)
    wanted = [commit for commit in commits if not excluded(commit.subject)]
    kept, large = [], []
    for commit in wanted:
        (large if len(commit.files) > limits.max_changeset else kept).append(commit)
    module_for = _existing_module_finder(repo_root, roots, depth)
    revisions: Counter[str] = Counter()
    shared: defaultdict[tuple[str, str], list[str]] = defaultdict(list)
    sweeping = []
    for commit in kept:
        modules = sorted({module for path in commit.files if (module := module_for(path))})
        counted = _counted(modules, reading)
        if limits.max_modules is not None and len(counted) > limits.max_modules:
            sweeping.append(_sweep(commit, counted, reading))
            continue
        revisions.update(modules)
        for pair in combinations(modules, 2):
            shared[pair].append(commit.subject)
    pairs = [
        _pair(first, second, subjects, revisions, reading)
        for (first, second), subjects in shared.items()
        if len(subjects) >= limits.min_shared
        and min(revisions[first], revisions[second]) >= limits.min_revisions
    ]
    return {
        "commits": len(commits),
        "excluded_commits": len(commits) - len(wanted),
        "skipped_large_commits": len(large),
        "skipped_large": [
            {"sha": commit.sha, "subject": commit.subject, "files": len(commit.files)} for commit in large[:SKIPPED_SHOWN]
        ],
        "skipped_sweeping_commits": len(sweeping),
        "skipped_sweeping": sweeping[:SKIPPED_SHOWN],
        "modules": [{"module": module, "revisions": count} for module, count in sorted(revisions.items())],
        "pairs": sorted(
            (pair for pair in pairs if pair["degree"] >= limits.min_degree),
            key=lambda pair: (-pair["degree"], -pair["shared"], pair["modules"]),
        ),
    }


def _sweep(commit: Commit, counted: list[str], reading: CouplingReading) -> dict[str, Any]:
    sweep: dict[str, Any] = {"sha": commit.sha, "subject": commit.subject, "modules": counted}
    if reading.context_paths:
        sweep["contexts"] = sorted({context_of(module, reading.context_paths) for module in counted})
    return sweep


def _counted(modules: list[str], reading: CouplingReading) -> list[str]:
    if not reading.context_paths:
        return modules
    return [module for module in modules if context_of(module, reading.context_paths)]


def _pair(
    first: str, second: str, subjects: list[str], revisions: Counter[str], reading: CouplingReading
) -> dict[str, Any]:
    pair: dict[str, Any] = {
        "modules": [first, second],
        "shared": len(subjects),
        "degree": round(200 * len(subjects) / (revisions[first] + revisions[second])),
        "subjects": subjects[:SUBJECTS_SHOWN],
    }
    if reading.context_paths:
        contexts = [context_of(first, reading.context_paths), context_of(second, reading.context_paths)]
        pair["contexts"] = contexts
        unmapped = [module for module, context in zip((first, second), contexts, strict=True) if context is None]
        if unmapped:
            pair["explained_by"] = None
            pair["unmapped"] = unmapped
        else:
            pair["explained_by"] = _explanations(contexts, reading.collaborations)
    return pair


def _explanations(contexts: list[str], collaborations: dict[frozenset[str], list[str]]) -> list[str]:
    one, other = contexts
    if one == other:
        return [f"contexts/{one}"]
    return sorted(collaborations.get(frozenset((one, other)), []))


def _subject_filter(patterns: tuple[str, ...]) -> Callable[[str], bool]:
    try:
        compiled = [re.compile(pattern) for pattern in patterns]
    except re.error as error:
        raise ValueError(f"--exclude-subject is not a regular expression: {error}") from error
    return lambda subject: any(pattern.search(subject) for pattern in compiled)


def context_of(module: str, context_paths: dict[str, str]) -> str | None:
    parts = PurePosixPath(module).parts
    owners = [
        (len(owned), context)
        for context, path in context_paths.items()
        if (owned := PurePosixPath(path.strip("/")).parts)
        and any(parts[start : start + len(owned)] == owned for start in range(len(parts) - len(owned) + 1))
    ]
    return max(owners)[1] if owners else None


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


def _commits(repo_root: Path, since: str | None) -> list[Commit]:
    arguments = ["-c", "core.quotepath=false", "log", "--no-merges", "--no-renames", "--name-only",
                 "--format=%x1e%h %s"]
    if since:
        arguments.append(f"--since={since}")
    try:
        result = run_git(repo_root, *arguments)
    except (OSError, subprocess.TimeoutExpired) as error:
        raise ValueError(f"git could not read the history: {error}") from error
    if result.returncode != 0:
        raise ValueError(f"git could not read the history: {result.stderr.strip()}")
    commits = (_commit(block) for block in result.stdout.split(COMMIT_MARKER)[1:])
    return [commit for commit in commits if commit.files]


def _commit(block: str) -> Commit:
    header, _, names = block.partition("\n")
    sha, _, subject = header.partition(" ")
    return Commit(sha, subject, frozenset(line for line in names.splitlines() if line.strip()))
