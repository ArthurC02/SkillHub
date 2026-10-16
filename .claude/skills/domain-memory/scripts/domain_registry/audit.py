from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path
from typing import Any

from .common import (
    MAX_JSON_BYTES,
    PROPOSAL_FILE,
    load_json,
    reject_duplicate_keys,
    utc_now,
    write_json,
)
from .sources import run_git
from .store import has_registry

MANIFEST_FORMAT = "domain-memory-audit/v1"


def canonical(value: dict[str, Any]) -> bytes:
    return json.dumps(
        value, ensure_ascii=False, sort_keys=True, separators=(",", ":")
    ).encode("utf-8")


def event_digest(value: dict[str, Any]) -> str:
    return "sha256:" + hashlib.sha256(canonical(value)).hexdigest()


def events_path(root: Path) -> Path:
    return root / "audit" / "events.jsonl"


def audit_manifest_path(root: Path) -> Path:
    return root / "audit" / "manifest.json"


def read_events(root: Path) -> list[dict[str, Any]]:
    path = events_path(root)
    if not path.is_file():
        return []
    if path.stat().st_size > MAX_JSON_BYTES:
        raise ValueError(f"audit log exceeds {MAX_JSON_BYTES} bytes: {path}")
    events = []
    for index, line in enumerate(
        path.read_text(encoding="utf-8").splitlines(), start=1
    ):
        try:
            value = json.loads(line, object_pairs_hook=reject_duplicate_keys)
        except (json.JSONDecodeError, ValueError) as error:
            raise ValueError(f"audit event {index} is invalid JSON") from error
        if not isinstance(value, dict):
            raise ValueError(f"audit event {index} must be an object")
        events.append(value)
    return events


def reviewing_proposals(root: Path) -> dict[str, str]:
    reviewed_by: dict[str, str] = {}
    for event in read_events(root):
        if event.get("operation") == "apply-approved-updates":
            reviewed_by.update(dict.fromkeys(event.get("records", []), event.get("proposal_id")))
        elif event.get("operation") == "migrate-registry":
            reviewed_by.update(event.get("reviewed_by", {}))
    return reviewed_by


def append_locked(root: Path, event: dict[str, Any]) -> None:
    reserved = {
        "sequence",
        "previous_event_sha256",
        "recorded_at",
        "event_sha256",
    }
    if reserved.intersection(event):
        raise ValueError("audit event contains reserved fields")
    directory = root / "audit"
    directory.mkdir(exist_ok=True)
    existing = verify(root)
    if existing["status"] != "valid":
        raise ValueError(f"audit log is invalid: {existing['reason']}")
    value = {
        "sequence": existing["events"] + 1,
        "previous_event_sha256": existing["head_sha256"],
        "recorded_at": utc_now(),
        **event,
    }
    value["event_sha256"] = event_digest(value)
    with events_path(root).open("a", encoding="utf-8", newline="\n") as output:
        output.write(json.dumps(value, ensure_ascii=False, sort_keys=True) + "\n")
        output.flush()
        os.fsync(output.fileno())
    manifest = {
        "format": MANIFEST_FORMAT,
        "events": value["sequence"],
        "head_sha256": value["event_sha256"],
    }
    write_json(audit_manifest_path(root), manifest)


def _invalid(reason: str) -> dict[str, Any]:
    return {"status": "invalid", "reason": reason}


def _event_error(event: dict[str, Any], sequence: int, previous: str | None) -> str | None:
    unsigned = {key: value for key, value in event.items() if key != "event_sha256"}
    if event.get("sequence") != sequence:
        return f"audit sequence is invalid at event {sequence}"
    if event.get("previous_event_sha256") != previous:
        return f"audit chain is broken at event {sequence}"
    if event.get("event_sha256") != event_digest(unsigned):
        return f"audit digest is invalid at event {sequence}"
    return None


def _chain_head(events: list[dict[str, Any]]) -> str | None:
    previous = None
    for sequence, event in enumerate(events, start=1):
        error = _event_error(event, sequence, previous)
        if error:
            raise ValueError(error)
        previous = event["event_sha256"]
    return previous


def _manifest_error(root: Path, events: int, head: str | None) -> str | None:
    manifest_path = audit_manifest_path(root)
    if not manifest_path.is_file():
        return None
    manifest = load_json(manifest_path)
    if (
        manifest.get("format") != MANIFEST_FORMAT
        or manifest.get("events") != events
        or manifest.get("head_sha256") != head
    ):
        return "audit manifest does not match the event log"
    return None


def verify(root: Path) -> dict[str, Any]:
    if not has_registry(root):
        return _invalid(
            f"no Domain Memory Registry at {root}; point --registry-root at the directory "
            "that holds registry/manifest.json"
        )
    try:
        events = read_events(root)
        head = _chain_head(events)
        error = _manifest_error(root, len(events), head)
    except ValueError as failure:
        return _invalid(str(failure))
    if error:
        return _invalid(error)
    return {"status": "valid", "events": len(events), "head_sha256": head}


def anchor_error(root: Path, changes_root: Path | None, against: str | None) -> str | None:
    if changes_root is not None:
        missing = _applied_without_event(root, changes_root)
        if missing:
            return "applied proposals have no apply-approved-updates event in the audit log: " + ", ".join(missing)
    return _committed_prefix_error(root, against) if against is not None else None


def _applied_without_event(root: Path, changes_root: Path) -> list[str]:
    applied = {
        event.get("proposal_id") for event in read_events(root) if event.get("operation") == "apply-approved-updates"
    }
    missing = []
    for package in sorted(path for path in changes_root.iterdir() if (path / PROPOSAL_FILE).is_file()):
        proposal = load_json(package / PROPOSAL_FILE)
        was_applied = "applied" in (proposal.get("status"), proposal.get("superseded_from_status"))
        if was_applied and proposal.get("proposal_id") not in applied:
            missing.append(f"{package.name} ({proposal.get('proposal_id')})")
    return missing


def _committed_prefix_error(root: Path, against: str) -> str | None:
    top = run_git(root, "rev-parse", "--show-toplevel")
    if top.returncode or run_git(root, "rev-parse", "--verify", "--quiet", f"{against}^{{commit}}").returncode:
        raise ValueError(f"git cannot resolve {against} from {root}")
    shown = Path(top.stdout.strip()).resolve()
    manifest_path = audit_manifest_path(root).resolve().relative_to(shown).as_posix()
    committed = run_git(shown, "show", f"{against}:{manifest_path}")
    if committed.returncode:
        return f"no audit manifest committed at {against}:{manifest_path}, so nothing anchors the log"
    try:
        manifest = json.loads(committed.stdout)
    except json.JSONDecodeError:
        return f"the audit manifest committed at {against} is not valid JSON"
    count, head = manifest.get("events"), manifest.get("head_sha256")
    events = read_events(root)
    if not isinstance(count, int) or count > len(events) or (count and events[count - 1].get("event_sha256") != head):
        return (
            f"the audit log does not extend the one committed at {against} ({count} events, head {head}); "
            "events were removed or rewritten since"
        )
    return None
