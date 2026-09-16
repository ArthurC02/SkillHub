from __future__ import annotations

import json
import hashlib
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


def canonical(value: dict[str, Any]) -> bytes:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode("utf-8")


def event_digest(value: dict[str, Any]) -> str:
    return "sha256:" + hashlib.sha256(canonical(value)).hexdigest()


def events_path(root: Path) -> Path:
    return root / "audit" / "events.jsonl"


def read_events(root: Path) -> list[dict[str, Any]]:
    path = events_path(root)
    if not path.is_file():
        return []
    events = []
    for index, line in enumerate(path.read_text(encoding="utf-8").splitlines(), start=1):
        try:
            value = json.loads(line)
        except json.JSONDecodeError as error:
            raise ValueError(f"audit event {index} is invalid JSON") from error
        if not isinstance(value, dict):
            raise ValueError(f"audit event {index} must be an object")
        events.append(value)
    return events


def append(root: Path, event: dict[str, Any]) -> None:
    directory = root / "audit"
    directory.mkdir(exist_ok=True)
    events = read_events(root)
    previous = events[-1].get("event_sha256") if events else None
    value = {"sequence": len(events) + 1, "previous_event_sha256": previous, "recorded_at": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"), **event}
    value["event_sha256"] = event_digest(value)
    with events_path(root).open("a", encoding="utf-8") as output:
        output.write(json.dumps(value, ensure_ascii=False, sort_keys=True) + "\n")


def verify(root: Path) -> dict[str, Any]:
    try:
        events = read_events(root)
    except ValueError as error:
        return {"status": "invalid", "reason": str(error)}
    previous = None
    for sequence, event in enumerate(events, start=1):
        stored_digest = event.get("event_sha256")
        unsigned = {key: value for key, value in event.items() if key != "event_sha256"}
        if event.get("sequence") != sequence:
            return {"status": "invalid", "reason": f"audit sequence is invalid at event {sequence}"}
        if event.get("previous_event_sha256") != previous:
            return {"status": "invalid", "reason": f"audit chain is broken at event {sequence}"}
        if stored_digest != event_digest(unsigned):
            return {"status": "invalid", "reason": f"audit digest is invalid at event {sequence}"}
        previous = stored_digest
    return {"status": "valid", "events": len(events), "head_sha256": previous}
