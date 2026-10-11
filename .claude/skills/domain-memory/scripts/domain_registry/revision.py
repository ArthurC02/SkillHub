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

COMMIT_LENGTH = 40
DIGEST_PREFIX = "sha256:"
DIGEST_HEX_LENGTH = 64


def _is_lowercase_hex(text: str, length: int) -> bool:
    return len(text) == length and all(
        character in "0123456789abcdef" for character in text
    )


def _canonical_json(value: Any) -> bytes:
    return json.dumps(
        value, ensure_ascii=False, separators=(",", ":"), sort_keys=True
    ).encode("utf-8")


def _feed(digest: Any, name: str, content: bytes) -> None:
    digest.update(name.encode("utf-8") + b"\0" + content + b"\0")


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
    return commit if _is_lowercase_hex(commit, COMMIT_LENGTH) else None


REGISTRY_FILES = ("manifest.json", *sorted(ASSET_KEYS))


def _digest_of(documents: dict[str, Any], policy_name: str, policy: Any) -> str:
    digest = hashlib.sha256()
    for name in REGISTRY_FILES:
        _feed(digest, name, _canonical_json(documents[name]))
    _feed(digest, policy_name, b"absent" if policy is None else _canonical_json(policy))
    return f"{DIGEST_PREFIX}{digest.hexdigest()}"


def _current_documents(root: Path) -> tuple[dict[str, Any], Any]:
    documents = {}
    for name in REGISTRY_FILES:
        path = registry_dir(root) / name
        if not path.is_file():
            raise ValueError(
                f"registry asset is missing while calculating revision: {path}"
            )
        documents[name] = load_json(path)
    policy = policy_path(root)
    return documents, load_json(policy) if policy.is_file() else None


def registry_digest(root: Path) -> str:
    documents, policy = _current_documents(root)
    return _digest_of(documents, policy_path(root).name, policy)


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
            or (isinstance(commit, str) and _is_lowercase_hex(commit, COMMIT_LENGTH))
        )
        and isinstance(digest, str)
        and digest.startswith(DIGEST_PREFIX)
        and _is_lowercase_hex(digest.removeprefix(DIGEST_PREFIX), DIGEST_HEX_LENGTH)
    )


STALE_BASE = "proposal base revision is stale; rebase and obtain fresh approval"


def _committed_json(repo_root: Path, commit: str, path: Path) -> tuple[bool, Any]:
    relative = path.resolve().relative_to(repo_root.resolve()).as_posix()
    shown = subprocess.run(
        ["git", "-C", str(repo_root), "show", f"{commit}:{relative}"],
        capture_output=True, check=False, timeout=GIT_COMMAND_TIMEOUT_SECONDS,
    )
    if shown.returncode != 0:
        return False, None
    return True, json.loads(shown.stdout.decode("utf-8"))


def _base_documents(
    root: Path, repo_root: Path, base: dict[str, Any]
) -> tuple[dict[str, Any], Any] | None:
    commit = base.get("observed_commit")
    if commit is None:
        return None
    documents = {}
    for name in REGISTRY_FILES:
        found, document = _committed_json(repo_root, commit, registry_dir(root) / name)
        if not found:
            return None
        documents[name] = document
    _, policy = _committed_json(repo_root, commit, policy_path(root))
    if _digest_of(documents, policy_path(root).name, policy) != base["registry_digest"]:
        return None
    return documents, policy


def _record(documents: dict[str, Any], name: str, record_id: str) -> Any:
    records = documents[name].get(ASSET_KEYS[name], [])
    return next(
        (r for r in records if isinstance(r, dict) and r.get("id") == record_id), None
    )


def _without_records(name: str, document: Any) -> Any:
    if name not in ASSET_KEYS or not isinstance(document, dict):
        return document
    return {key: value for key, value in document.items() if key != ASSET_KEYS[name]}


def _strings_in(value: Any) -> set[str]:
    if isinstance(value, str):
        return {value}
    if isinstance(value, dict):
        return set().union(*map(_strings_in, value.values()))
    if isinstance(value, list):
        return set().union(*map(_strings_in, value))
    return set()


def _touched_records(
    proposal: dict[str, Any], documents: dict[str, Any]
) -> set[tuple[str, str]]:
    touched: set[tuple[str, str]] = set()
    mentioned: set[str] = set()
    for update in proposal.get("registry_updates") or []:
        if not isinstance(update, dict):
            continue
        record = update.get("record")
        record_id = update.get("id") or (record or {}).get("id")
        touched.add((f"{update.get('asset')}.json", str(record_id)))
        mentioned |= _strings_in(record)
    for name in ASSET_KEYS:
        for record in documents[name].get(ASSET_KEYS[name], []):
            if isinstance(record, dict) and record.get("id") in mentioned:
                touched.add((name, record["id"]))
    return touched


def _changed_since_base(
    proposal: dict[str, Any], root: Path, repo_root: Path, base: dict[str, Any]
) -> str | None:
    rebuilt = _base_documents(root, repo_root, base)
    if rebuilt is None:
        return (
            "the base Registry cannot be rebuilt from its observed commit, so the records "
            "this proposal touches cannot be compared"
        )
    then, then_policy = rebuilt
    now, now_policy = _current_documents(root)
    if then_policy != now_policy:
        return "the Domain Memory policy changed"
    for name in REGISTRY_FILES:
        if _without_records(name, then[name]) != _without_records(name, now[name]):
            return f"{name} changed outside its records"
    for name, record_id in sorted(_touched_records(proposal, then) | _touched_records(proposal, now)):
        if name not in ASSET_KEYS or _record(then, name, record_id) != _record(now, name, record_id):
            return f"{name.removesuffix('.json')}/{record_id}, which this proposal touches, changed"
    return None


def require_current_registry_revision(
    proposal: dict[str, Any], root: Path, repo_root: Path
) -> str:
    expected = proposal.get("base_registry_revision")
    if not valid_registry_revision(expected):
        raise ValueError(
            "proposal requires a Git commit and Registry digest base revision"
        )
    actual = current_registry_revision(root, repo_root)["registry_digest"]
    if expected["registry_digest"] == actual:
        return actual
    changed = _changed_since_base(proposal, root, repo_root, expected)
    if changed:
        raise ValueError(f"{STALE_BASE}: {changed}")
    return actual
