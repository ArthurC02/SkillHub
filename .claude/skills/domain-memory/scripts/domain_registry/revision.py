from __future__ import annotations

import hashlib
import json
import subprocess
from pathlib import Path
from typing import Any, TypeGuard

from .common import (
    ASSET_KEYS,
    GIT_COMMAND_TIMEOUT_SECONDS,
    load_json,
    registry_dir,
)
from .policy import policy_path


def git_commit(repo_root: Path) -> str | None:
    try:
        result = subprocess.run(
            ["git", "-C", str(repo_root), "rev-parse", "HEAD"],
            capture_output=True,
            check=False,
            encoding="utf-8",
            timeout=GIT_COMMAND_TIMEOUT_SECONDS,
        )
    except (OSError, subprocess.TimeoutExpired):
        return None
    if result.returncode != 0:
        return None
    commit = result.stdout.strip()
    if len(commit) != 40 or any(
        character not in "0123456789abcdef" for character in commit
    ):
        return None
    return commit


def registry_digest(root: Path) -> str:
    digest = hashlib.sha256()
    for name in ("manifest.json", *sorted(ASSET_KEYS)):
        path = registry_dir(root) / name
        if not path.is_file():
            raise ValueError(
                f"registry asset is missing while calculating revision: {path}"
            )
        digest.update(name.encode("utf-8"))
        digest.update(b"\0")
        value = load_json(path)
        digest.update(
            json.dumps(
                value, ensure_ascii=False, separators=(",", ":"), sort_keys=True
            ).encode("utf-8")
        )
        digest.update(b"\0")
    policy = policy_path(root)
    digest.update(policy.name.encode("utf-8"))
    digest.update(b"\0")
    digest.update(
        json.dumps(
            load_json(policy), ensure_ascii=False, separators=(",", ":"), sort_keys=True
        ).encode("utf-8")
        if policy.is_file()
        else b"absent"
    )
    digest.update(b"\0")
    return f"sha256:{digest.hexdigest()}"


def current_registry_revision(root: Path, repo_root: Path) -> dict[str, str | None]:
    resolved_root = root.resolve()
    resolved_repo = repo_root.resolve()
    try:
        resolved_root.relative_to(resolved_repo)
    except ValueError as error:
        raise ValueError("registry root must be inside repo root") from error
    return {
        "observed_commit": git_commit(resolved_repo),
        "registry_digest": registry_digest(resolved_root),
    }


def valid_registry_revision(value: Any) -> TypeGuard[dict[str, Any]]:
    if not isinstance(value, dict):
        return False
    commit = value.get("observed_commit")
    digest = value.get("registry_digest")
    return (
        (
            commit is None
            or (
                isinstance(commit, str)
                and len(commit) == 40
                and all(character in "0123456789abcdef" for character in commit)
            )
        )
        and isinstance(digest, str)
        and digest.startswith("sha256:")
        and len(digest) == 71
        and all(
            character in "0123456789abcdef"
            for character in digest.removeprefix("sha256:")
        )
    )


def require_current_registry_revision(
    proposal: dict[str, Any], root: Path, repo_root: Path
) -> None:
    expected = proposal.get("base_registry_revision")
    if not valid_registry_revision(expected):
        raise ValueError(
            "proposal requires a Git commit and Registry digest base revision"
        )
    actual = current_registry_revision(root, repo_root)
    if expected["registry_digest"] != actual["registry_digest"]:
        raise ValueError(
            "proposal base revision is stale; rebase and obtain fresh approval"
        )
