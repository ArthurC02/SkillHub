from __future__ import annotations

import hashlib
import json
import subprocess
from pathlib import Path
from typing import Any, TypeGuard

from .common import ASSET_KEYS, find_record, registry_dir, shown_path
from .git import run_git, run_git_bytes
from .policy import existing_policy, policy_path
from .registry import as_stored
from .store import committed_documents, document_path, has_document, read_document

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
        result = run_git(repo_root, "rev-parse", "HEAD")
    except (OSError, subprocess.TimeoutExpired):
        return None
    if result.returncode != 0:
        return None
    commit = result.stdout.strip()
    return commit if _is_lowercase_hex(commit, COMMIT_LENGTH) else None


REGISTRY_FILES = ("manifest.json", *sorted(ASSET_KEYS))


def _in_identifier_order(name: str, document: Any) -> Any:
    records = document.get(ASSET_KEYS[name]) if name in ASSET_KEYS and isinstance(document, dict) else None
    if not isinstance(records, list):
        return document
    ordered = sorted(records, key=lambda record: str(record.get("id")) if isinstance(record, dict) else "")
    return {**document, ASSET_KEYS[name]: ordered}


def _digest_of(documents: dict[str, Any], policy_name: str, policy: Any) -> str:
    digest = hashlib.sha256()
    for name in REGISTRY_FILES:
        _feed(digest, name, _canonical_json(_in_identifier_order(name, documents[name])))
    _feed(digest, policy_name, b"absent" if policy is None else _canonical_json(policy))
    return f"{DIGEST_PREFIX}{digest.hexdigest()}"


def _current_documents(root: Path) -> tuple[dict[str, Any], Any]:
    documents = {}
    for name in REGISTRY_FILES:
        if not has_document(root, name):
            raise ValueError(
                f"registry asset is missing while calculating revision: {document_path(root, name)}"
            )
        documents[name] = read_document(root, name)
    return documents, existing_policy(root)


def registry_digest(root: Path) -> str:
    documents, policy = _current_documents(root)
    return _digest_of(documents, policy_path(root).name, policy)


def current_registry_revision(root: Path, repo_root: Path) -> dict[str, str | None]:
    return {
        "observed_commit": git_commit(repo_root.resolve()),
        "registry_digest": _digest_inside(root, repo_root),
    }


def base_registry_revision(root: Path, repo_root: Path) -> dict[str, str | None]:
    revision = current_registry_revision(root, repo_root)
    commit = revision["observed_commit"]
    committed = None if commit is None else _committed_registry(root, repo_root, commit)
    if committed is None:
        return revision
    documents, policy = committed
    return {**revision, "committed_digest": _digest_of(documents, policy_path(root).name, policy)}


def _digest_inside(root: Path, repo_root: Path) -> str:
    resolved_root = root.resolve()
    try:
        resolved_root.relative_to(repo_root.resolve())
    except ValueError as error:
        raise ValueError("registry root must be inside repo root") from error
    return registry_digest(resolved_root)


def valid_digest(value: Any) -> bool:
    return (
        isinstance(value, str)
        and value.startswith(DIGEST_PREFIX)
        and _is_lowercase_hex(value.removeprefix(DIGEST_PREFIX), DIGEST_HEX_LENGTH)
    )


def valid_commit(value: Any) -> bool:
    return isinstance(value, str) and _is_lowercase_hex(value, COMMIT_LENGTH)


def valid_registry_revision(value: Any) -> TypeGuard[dict[str, Any]]:
    if not isinstance(value, dict):
        return False
    commit = value.get("observed_commit")
    return (commit is None or valid_commit(commit)) and valid_digest(
        value.get("registry_digest")
    )


STALE_BASE = "proposal base revision is stale; rebase-proposal, then obtain fresh approval"


class StaleBaseError(ValueError):
    pass


def committed_json(repo_root: Path, commit: str, path: Path) -> tuple[bool, Any]:
    relative = path.resolve().relative_to(repo_root.resolve()).as_posix()
    shown = run_git_bytes(repo_root, "show", f"{commit}:{relative}")
    if shown.returncode != 0:
        return False, None
    try:
        return True, json.loads(shown.stdout.decode("utf-8"))
    except (ValueError, UnicodeDecodeError) as error:
        raise ValueError(f"{commit}:{relative} is not valid JSON") from error


def _committed_registry(root: Path, repo_root: Path, commit: str) -> tuple[dict[str, Any], Any] | None:
    documents = committed_documents(repo_root, commit, root, REGISTRY_FILES)
    if documents is None:
        return None
    _, policy = committed_json(repo_root, commit, policy_path(root))
    return documents, policy


CANNOT_COMPARE = (
    "so the records this proposal touches cannot be compared; rebase-proposal restamps the proposal on the "
    "current Registry"
)


def _base_documents(
    root: Path, repo_root: Path, base: dict[str, Any]
) -> tuple[dict[str, Any], Any] | str:
    commit = base.get("observed_commit")
    if commit is None:
        return f"the base revision names no commit, {CANNOT_COMPARE}"
    rebuilt = _committed_registry(root, repo_root, commit)
    if rebuilt is None:
        return (
            f"commit {commit[:12]} does not hold the Registry at {shown_path(registry_dir(root), repo_root)}, "
            f"{CANNOT_COMPARE}"
        )
    documents, policy = rebuilt
    if _digest_of(documents, policy_path(root).name, policy) != base.get("committed_digest", base["registry_digest"]):
        return f"the Registry committed at {commit[:12]} does not reproduce the recorded base digest, {CANNOT_COMPARE}"
    return rebuilt


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
    if isinstance(rebuilt, str):
        return rebuilt
    then, then_policy = rebuilt
    now, now_policy = _current_documents(root)
    if then_policy != now_policy:
        return "the Domain Memory policy changed"
    for name in REGISTRY_FILES:
        if _without_records(name, then[name]) != _without_records(name, now[name]):
            return f"{name} changed outside its records"
    written = _written_records(proposal)
    for name, record_id in sorted(_touched_records(proposal, then) | _touched_records(proposal, now)):
        changed = _changed_record(name, record_id, (then, now), written)
        if changed:
            return changed
    return None


def _written_records(proposal: dict[str, Any]) -> dict[tuple[str, str], dict[str, Any]]:
    return {
        (f"{update.get('asset')}.json", str(update["record"].get("id"))): update["record"]
        for update in proposal.get("registry_updates") or []
        if isinstance(update, dict) and update.get("operation") == "upsert" and isinstance(update.get("record"), dict)
    }


def _changed_record(
    name: str, record_id: str, documents: tuple[dict[str, Any], dict[str, Any]],
    written: dict[tuple[str, str], dict[str, Any]],
) -> str | None:
    shown = f"{name.removesuffix('.json')}/{record_id}"
    if name not in ASSET_KEYS:
        return f"{shown}, which this proposal touches, changed"
    then, now = (find_record(document[name].get(ASSET_KEYS[name], []), record_id) for document in documents)
    if then == now:
        return None
    if (name, record_id) not in written:
        return f"{shown}, which this proposal touches, changed"
    asset = name.removesuffix(".json")
    if isinstance(now, dict) and as_stored(asset, now) == as_stored(asset, written[(name, record_id)]):
        return None
    return f"{shown}, which this proposal writes, changed since it was reviewed"


def require_current_registry_revision(
    proposal: dict[str, Any], root: Path, repo_root: Path
) -> str:
    expected = proposal.get("base_registry_revision")
    if not valid_registry_revision(expected):
        raise ValueError(
            "proposal requires a Git commit and Registry digest base revision"
        )
    actual = _digest_inside(root, repo_root)
    if expected["registry_digest"] == actual:
        return actual
    changed = _changed_since_base(proposal, root, repo_root, expected)
    if changed:
        raise StaleBaseError(f"{STALE_BASE}: {changed}")
    return actual
