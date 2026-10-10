from __future__ import annotations

import hashlib
import json
import os
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from .common import MAX_JSON_BYTES, load_json, registry_dir, reject_duplicate_keys, writer_lock

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
    manifest_path = audit_manifest_path(root)
    if manifest_path.is_file():
        manifest = load_json(manifest_path)
        if (
            manifest.get("format") != MANIFEST_FORMAT
            or not isinstance(manifest.get("events"), int)
            or manifest["events"] < 0
        ):
            raise ValueError("audit manifest is invalid")
        sequence = manifest["events"] + 1
        previous = manifest.get("head_sha256")
    else:
        events = read_events(root)
        sequence = len(events) + 1
        previous = events[-1].get("event_sha256") if events else None
    value = {
        "sequence": sequence,
        "previous_event_sha256": previous,
        "recorded_at": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
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
    temporary = audit_manifest_path(root).with_suffix(".json.tmp")
    temporary.write_text(
        json.dumps(manifest, indent=2, ensure_ascii=False) + "\n", encoding="utf-8", newline="\n"
    )
    temporary.replace(audit_manifest_path(root))


def append(root: Path, event: dict[str, Any]) -> None:
    with writer_lock(root):
        append_locked(root, event)


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
    if not (registry_dir(root) / "manifest.json").is_file():
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
